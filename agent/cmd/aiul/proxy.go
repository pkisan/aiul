package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkisan/aiul/internal/ca"
	"github.com/pkisan/aiul/internal/forward"
	"github.com/pkisan/aiul/internal/proxy"
)

const proxyUsage = `Usage:
  aiul proxy [--addr 127.0.0.1:8899] [--spool DIR] [--debug]

Runs the TLS-inspecting proxy. It changes NOTHING on your machine: point one
command at it with environment variables to try it, for example

  HTTPS_PROXY=http://127.0.0.1:8899 \
  NODE_EXTRA_CA_CERTS="$HOME/Library/Application Support/AIUL/dev-ca/root.crt" \
  gemini -p "hello"
`

func cmdProxy(args []string) int {
	addr := "127.0.0.1:8899"
	spoolDir := ""
	level := slog.LevelInfo

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--addr":
			if i+1 >= len(args) {
				fmt.Fprint(os.Stderr, proxyUsage)
				return 2
			}
			i++
			addr = args[i]
		case "--spool":
			if i+1 >= len(args) {
				fmt.Fprint(os.Stderr, proxyUsage)
				return 2
			}
			i++
			spoolDir = args[i]
		case "--debug":
			level = slog.LevelDebug
		case "-h", "--help":
			fmt.Print(proxyUsage)
			return 0
		default:
			fmt.Fprintf(os.Stderr, "aiul proxy: unknown flag %q\n\n%s", args[i], proxyUsage)
			return 2
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	root, err := ca.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul proxy: %v\n", err)
		return 1
	}

	spool, err := forward.NewSpool(spoolDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul proxy: %v\n", err)
		return 1
	}

	p, err := proxy.New(proxy.Config{
		Addr:   addr,
		Root:   root,
		Logger: logger,
		Sink:   sinkFunc(spool.Record),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul proxy: %v\n", err)
		return 1
	}

	fmt.Printf("aiul proxy on %s\n", addr)
	fmt.Printf("  CA certificate  %s\n", mustCertPath())
	fmt.Printf("  events          %s\n", spool.Dir())
	fmt.Printf("  allow-list      version %d — only AI hosts are decrypted, everything else passes through sealed\n", proxy.AllowListVersion)
	fmt.Println("  stop with       Ctrl-C")
	fmt.Println()

	// Ctrl-C should stop the proxy cleanly rather than cutting connections dead.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		fmt.Println("\nstopping")
		os.Exit(0)
	}()

	if err := p.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "aiul proxy: %v\n", err)
		return 1
	}
	return 0
}

// sinkFunc adapts a plain function to the proxy's Sink interface.
type sinkFunc func(any)

func (f sinkFunc) Record(e proxy.Event) { f(e) }

func mustCertPath() string {
	certPath, _, err := ca.Paths()
	if err != nil {
		return "(unknown)"
	}
	return certPath
}
