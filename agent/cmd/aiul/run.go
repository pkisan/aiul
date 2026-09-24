package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pkisan/aiul/internal/ca"
	"github.com/pkisan/aiul/internal/forward"
	"github.com/pkisan/aiul/internal/platform"
	"github.com/pkisan/aiul/internal/proxy"
	"github.com/pkisan/aiul/internal/tasks"
)

const runUsage = `Usage:
  aiul run [--endpoint URL] [--manage-proxy] [--token TOKEN] [--debug]

Runs the proxy and the agent loop in one process. This is what the LaunchDaemon
starts, with --manage-proxy.

WITHOUT --manage-proxy (the default) this changes NOTHING on your machine: it
serves the proxy and forwards events, and you point one command at it yourself.

WITH --manage-proxy the agent owns the system proxy setting: it re-applies the
setting if something removes it, and REMOVES it if the proxy stops answering, so
traffic is never blocked by a broken proxy. Only 'aiul install --apply' turns this
on, because it is the only command that asked you first.

The device token normally comes from the keychain. --token, or AIUL_DEVICE_TOKEN,
overrides it for development so nothing has to be stored to try the backend.

AIUL_ENDPOINT does the same for --endpoint. The INSTALLED agent reads both from
/etc/aiul/agent.conf instead, because launchd inherits no shell — see that file's
format in docs/TESTING.md.
`

// healthInterval is how often the agent loop checks itself. Short enough that a
// broken proxy does not block traffic for long, long enough not to be noise.
const healthInterval = 30 * time.Second

func cmdRun(args []string) int {
	endpoint := ""
	token := ""
	manageProxy := false

	// Settings the installed agent cannot be given on a command line, because
	// launchd inherits no shell. Read before the flags are parsed so a flag still
	// wins.
	config := readAgentConfig(agentConfigPath)

	// AIUL_DEBUG=1 does the same as --debug, from the environment when running by
	// hand or from the config file when installed. The debug lines are the ones
	// that say why a request was not recorded, so reaching them must not require a
	// reinstall.
	level := slog.LevelInfo
	if firstSet(os.Getenv("AIUL_DEBUG"), config["AIUL_DEBUG"]) != "" {
		level = slog.LevelDebug
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--endpoint":
			if i+1 >= len(args) {
				fmt.Fprint(os.Stderr, runUsage)
				return 2
			}
			i++
			endpoint = args[i]
		case "--token":
			if i+1 >= len(args) {
				fmt.Fprint(os.Stderr, runUsage)
				return 2
			}
			i++
			token = args[i]
		case "--manage-proxy":
			manageProxy = true
		case "--debug":
			level = slog.LevelDebug
		case "-h", "--help":
			fmt.Print(runUsage)
			return 0
		default:
			fmt.Fprintf(os.Stderr, "aiul run: unknown flag %q\n\n%s", args[i], runUsage)
			return 2
		}
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	// The MDM gate again: the daemon must refuse to start on an unmanaged device,
	// not only refuse to install.
	enrolled, detail, _ := platform.MDM().Enrolled()
	if !enrolled {
		log.Error("refusing to run: this device is not enrolled in an MDM",
			"detail", detail,
			"override", platform.DevAllowUnmanagedVar+"=1 for development only")
		return 1
	}

	root, err := ca.Load()
	if err != nil {
		log.Error("cannot load the CA", "err", err)
		return 1
	}

	// This device's own signing certificate (D3). Leaves are minted from this, not
	// from the root, so what signs our certificates is name-constrained to the
	// allow-list and expires within days. Renewed here on every start, and again by
	// the health loop while the agent runs.
	issuer, err := provisionIssuer(log, root)
	if err != nil {
		log.Error("cannot provision this device's signing certificate", "err", err)
		return 1
	}

	spool, err := forward.NewSpool("")
	if err != nil {
		log.Error("cannot open the spool", "err", err)
		return 1
	}

	// Where to forward, and what to authenticate with.
	//
	// Precedence: the flag, then the environment, then the config file, then the
	// keychain. The flag and the environment are for running by hand; the config
	// file is how the INSTALLED agent is told, because launchd does not inherit the
	// shell that installed it and `installer` does not pass its environment to
	// package scripts. Without the file the packaged agent captures and spools
	// perfectly and sends nothing, which looks like a broken backend rather than a
	// missing setting.
	if endpoint == "" {
		endpoint = firstSet(os.Getenv("AIUL_ENDPOINT"), config["AIUL_ENDPOINT"])
	}
	if token == "" {
		token = firstSet(os.Getenv("AIUL_DEVICE_TOKEN"), config["AIUL_DEVICE_TOKEN"])
	}
	if token == "" {
		token, _ = platform.DeviceToken()
	}
	forwarder := forward.NewForwarder(spool, forward.Config{
		Endpoint:    endpoint,
		DeviceToken: token,
		Logger:      log,
	})

	// Where the three privileged operations come from: the root helper when one is
	// listening, this process otherwise.
	privileged := chooseSource(log)

	p, err := proxy.New(proxy.Config{
		Addr:   proxyAddr,
		Issuer: issuer,
		Logger: log,
		Sink:   sinkFunc(spool.Record),
		// Task tagging: the source port identifies the client process, its working
		// directory gives the checkout, and the branch there gives the task ID.
		Tasks:     tasks.NewResolver(),
		Processes: sourceFinder{source: privileged},
		// The checkout comes back with the process lookup, because the worker's own
		// account cannot read anyone's .git/HEAD.
		Checkout: rememberedCheckout,
		// Research mode: off unless AIUL_RESEARCH_DUMP names a directory.
		ResearchDir: firstSet(os.Getenv("AIUL_RESEARCH_DUMP"), config["AIUL_RESEARCH_DUMP"]),
	})
	if err != nil {
		log.Error("cannot start the proxy", "err", err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// On any exit, remove the system proxy. If this process is not running, no
	// traffic must depend on it. Rule 7.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		// Only undo what we were asked to manage. Removing a proxy setting we
		// never set would be just as rude as setting one we were not asked to.
		if manageProxy {
			log.Info("stopping; removing the system proxy so traffic keeps flowing")
			_ = privileged.ProxyOff()
		}
		cancel()
		os.Exit(0)
	}()

	go agentLoop(ctx, log, forwarder, manageProxy, privileged, root, issuer)

	log.Info("agent running",
		"endpoint", endpoint,
		"proxy", proxyAddr,
		"spool", spool.Dir(),
		"forwarding", forwarder.Enabled(),
		"manages_system_proxy", manageProxy)
	if err := p.ListenAndServe(); err != nil {
		log.Error("the proxy stopped", "err", err)
		if manageProxy {
			// The proxy is gone, so nothing must be pointed at it any more.
			_ = privileged.ProxyOff()
		}
		return 1
	}
	return 0
}

// agentLoop is the housekeeping that runs alongside the proxy.
func agentLoop(ctx context.Context, log *slog.Logger, forwarder *forward.Forwarder, manageProxy bool, privileged privilegedSource, root *ca.Root, issuer *deviceIssuer) {
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if manageProxy {
				healthCheck(log, privileged)
			}

			// The device's signing certificate is short-lived on purpose, so
			// something has to renew it while the agent runs. A long-running daemon
			// that let it expire would stop being able to mint anything, and every
			// AI tool on the machine would fall back to a tunnel.
			if issuer.current().NeedsRenewal() {
				if err := renewIfNeeded(log, root, issuer); err != nil {
					log.Error("could not renew this device's signing certificate", "err", err)
				}
			}

			drainSpool(ctx, log, forwarder)
		}
	}
}

// healthCheck makes sure the proxy is answering, and if it is not, takes the
// system proxy setting away so traffic flows directly instead of failing.
//
// Only ever called with --manage-proxy. Without it this process must not touch a
// system setting: "the proxy setting is not pointing at us" is the normal state on
// a machine where nobody asked us to configure anything.
func healthCheck(log *slog.Logger, privileged privilegedSource) {
	conn, err := net.DialTimeout("tcp", proxyAddr, 5*time.Second)
	if err != nil {
		log.Error("the proxy is not answering; removing the system proxy so traffic is not blocked",
			"addr", proxyAddr, "err", err)
		if err := privileged.ProxyOff(); err != nil {
			log.Error("could not remove the system proxy", "err", err)
		}
		return
	}
	conn.Close()

	// The proxy is healthy. Re-apply the setting if something removed it — a
	// network change, a VPN connecting, or a new network service appearing.
	current, err := platform.Proxy().Current()
	if errors.Is(err, platform.ErrProxyStateHidden) {
		// This account cannot see the setting (Linux: it is each person's, and
		// only root reads it), so ask the helper to apply it again. The helper
		// changes only what differs.
		if err := privileged.ProxyOn(); err != nil {
			log.Error("could not re-apply the proxy setting", "err", err)
		}
		return
	}
	if err != nil {
		return
	}
	for service, value := range current {
		if value != proxyAddr {
			log.Info("the proxy setting drifted; re-applying", "service", service, "was", value)
			if err := privileged.ProxyOn(); err != nil {
				log.Error("could not re-apply the proxy setting", "err", err)
			}
			return
		}
	}
}

func drainSpool(ctx context.Context, log *slog.Logger, forwarder *forward.Forwarder) {
	if !forwarder.Enabled() {
		return
	}
	sent, err := forwarder.DrainOnce(ctx)
	if err != nil {
		log.Warn("could not forward events; they stay in the spool", "err", err, "retry_in", forwarder.Backoff())
		return
	}
	if sent > 0 {
		log.Info("forwarded events", "count", sent)
	}
}
