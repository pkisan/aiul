package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkisan/aiul/internal/helper"
	"github.com/pkisan/aiul/internal/platform"
)

const helperUsage = `Usage:
  aiul helper [--socket PATH] [--group GID] [--debug]

The privileged half of the agent. It runs as root, listens on a unix socket, and
answers exactly four things:

  PING           are you alive
  PROXY-ON       point the system proxy at our listener
  PROXY-OFF      remove the system proxy
  PROCESS <port> which process owns a connection from this local port

It never opens a network socket and never parses anything that came from the
network. That work happens in 'aiul run', which does not run as root.
`

func cmdHelper(args []string) int {
	socketPath := helper.SocketPath
	gid := -1
	level := slog.LevelInfo

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--socket":
			if i+1 >= len(args) {
				fmt.Fprint(os.Stderr, helperUsage)

				return 2
			}
			i++
			socketPath = args[i]
		case "--group":
			if i+1 >= len(args) {
				fmt.Fprint(os.Stderr, helperUsage)

				return 2
			}
			i++
			if _, err := fmt.Sscanf(args[i], "%d", &gid); err != nil {
				fmt.Fprintf(os.Stderr, "aiul helper: --group must be a numeric group id\n")

				return 2
			}
		case "--debug":
			level = slog.LevelDebug
		case "-h", "--help":
			fmt.Print(helperUsage)

			return 0
		default:
			fmt.Fprintf(os.Stderr, "aiul helper: unknown flag %q\n\n%s", args[i], helperUsage)

			return 2
		}
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if os.Geteuid() != 0 {
		log.Warn("not running as root; the privileged operations will fail")
	}

	server := helper.NewServer(privilegedOps{}, log)

	ln, err := server.Listen(socketPath, gid)
	if err != nil {
		log.Error("cannot listen", "err", err)

		return 1
	}
	defer ln.Close()

	// On the way out, take the system proxy with us: if the helper is gone, the
	// worker cannot ask for it to be removed later. Rule 7.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop
		log.Info("stopping; removing the system proxy so traffic keeps flowing")
		_ = platform.Proxy().Unset()
		ln.Close()
		os.Exit(0)
	}()

	log.Info("helper listening", "socket", socketPath, "group", gid)

	if err := server.Serve(ln); err != nil {
		log.Error("stopped serving", "err", err)

		return 1
	}

	return 0
}

// privilegedOps is the only code in the root process that does anything, and it
// is four lines of delegation to the platform package.
type privilegedOps struct{}

func (privilegedOps) ProxyOn() error  { return platform.Proxy().Set(proxyAddr) }
func (privilegedOps) ProxyOff() error { return platform.Proxy().Unset() }

func (privilegedOps) ProcessOnPort(port int) (int, string, string, error) {
	process, err := platform.Processes().ByLocalPort(port)
	if err != nil {
		return 0, "", "", err
	}

	dir, err := platform.Processes().WorkingDir(process.PID)
	if err != nil {
		// Knowing the process but not its directory is still useful: the event
		// records the tool even when the task cannot be worked out.
		return process.PID, process.Name, "", nil
	}

	return process.PID, process.Name, dir, nil
}
