package main

import (
	"context"
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
)

const runUsage = `Usage:
  aiul run [--endpoint URL] [--debug]

Runs the proxy and the agent loop in one process. This is what the LaunchDaemon
starts. The agent loop:
  - checks the proxy is healthy, and REMOVES the system proxy setting if it is not
  - re-applies settings that have drifted
  - forwards spooled events to the backend when one is configured
`

// healthInterval is how often the agent loop checks itself. Short enough that a
// broken proxy does not block traffic for long, long enough not to be noise.
const healthInterval = 30 * time.Second

func cmdRun(args []string) int {
	endpoint := ""
	level := slog.LevelInfo

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--endpoint":
			if i+1 >= len(args) {
				fmt.Fprint(os.Stderr, runUsage)
				return 2
			}
			i++
			endpoint = args[i]
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

	spool, err := forward.NewSpool("")
	if err != nil {
		log.Error("cannot open the spool", "err", err)
		return 1
	}

	token, _ := platform.DeviceToken()
	forwarder := forward.NewForwarder(spool, forward.Config{
		Endpoint:    endpoint,
		DeviceToken: token,
		Logger:      log,
	})

	p, err := proxy.New(proxy.Config{
		Addr:   proxyAddr,
		Root:   root,
		Logger: log,
		Sink:   sinkFunc(spool.Record),
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
		log.Info("stopping; removing the system proxy so traffic keeps flowing")
		_ = platform.Proxy().Unset()
		cancel()
		os.Exit(0)
	}()

	go agentLoop(ctx, log, forwarder)

	log.Info("agent running", "proxy", proxyAddr, "spool", spool.Dir(), "forwarding", forwarder.Enabled())
	if err := p.ListenAndServe(); err != nil {
		log.Error("the proxy stopped", "err", err)
		// The proxy is gone, so nothing must be pointed at it any more.
		_ = platform.Proxy().Unset()
		return 1
	}
	return 0
}

// agentLoop is the housekeeping that runs alongside the proxy.
func agentLoop(ctx context.Context, log *slog.Logger, forwarder *forward.Forwarder) {
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			healthCheck(log)
			drainSpool(ctx, log, forwarder)
		}
	}
}

// healthCheck makes sure the proxy is answering, and if it is not, takes the
// system proxy setting away so traffic flows directly instead of failing.
func healthCheck(log *slog.Logger) {
	conn, err := net.DialTimeout("tcp", proxyAddr, 5*time.Second)
	if err != nil {
		log.Error("the proxy is not answering; removing the system proxy so traffic is not blocked",
			"addr", proxyAddr, "err", err)
		if err := platform.Proxy().Unset(); err != nil {
			log.Error("could not remove the system proxy", "err", err)
		}
		return
	}
	conn.Close()

	// The proxy is healthy. Re-apply the setting if something removed it — a
	// network change, a VPN connecting, or a new network service appearing.
	current, err := platform.Proxy().Current()
	if err != nil {
		return
	}
	for service, value := range current {
		if value != proxyAddr {
			log.Info("the proxy setting drifted; re-applying", "service", service, "was", value)
			if err := platform.Proxy().Set(proxyAddr); err != nil {
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
