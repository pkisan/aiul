package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pkisan/aiul/internal/ca"
	"github.com/pkisan/aiul/internal/platform"
)

func cmdCA(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, caUsage)
		return 2
	}
	switch args[0] {
	case "init":
		return caInit(args[1:])
	case "info":
		return caInfo()
	case "trust":
		return caTrust(args[1:])
	case "untrust":
		return caUntrust(args[1:])
	case "device":
		return cmdCADevice(args[1:])
	case "demo-server":
		return caDemoServer(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "aiul ca: unknown subcommand %q\n\n%s", args[0], caUsage)
		return 2
	}
}

const caUsage = `Usage:
  aiul ca init [--force]   create the development root CA on this machine
  aiul ca info             show where it lives, its name, fingerprint and expiry
  aiul ca trust [--yes]    add it to the macOS System keychain (asks first)
  aiul ca untrust [--yes]  remove it from the System keychain
  aiul ca device [--renew] show this device's own signing certificate (D3)
  aiul ca demo-server      serve https://localhost:8443 with a certificate we mint
`

func caInit(args []string) int {
	force := contains(args, "--force")

	root, err := ca.Init(force)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul ca init: %v\n", err)
		return 1
	}
	certPath, keyPath, _ := ca.Paths()

	fmt.Println("Created the development root CA.")
	fmt.Printf("  name         %s\n", root.Cert.Subject.CommonName)
	fmt.Printf("  certificate  %s\n", certPath)
	fmt.Printf("  private key  %s   (mode 0600, readable only by you)\n", keyPath)
	fmt.Printf("  valid until  %s\n", root.Cert.NotAfter.Format(time.RFC1123))
	fmt.Printf("  SHA-256      %s\n", ca.Fingerprint(root.Cert))
	fmt.Println()
	fmt.Println("Nothing on this Mac trusts it yet. 'aiul ca trust' is the next step,")
	fmt.Println("and it shows you the exact command before running anything.")
	return 0
}

func caInfo() int {
	root, err := ca.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul ca info: %v\n", err)
		return 1
	}
	certPath, keyPath, _ := ca.Paths()

	trusted, _ := platform.Trust().IsTrusted(ca.CommonNamePrefix)

	fmt.Printf("  name         %s\n", root.Cert.Subject.CommonName)
	fmt.Printf("  certificate  %s\n", certPath)
	fmt.Printf("  private key  %s\n", keyPath)
	fmt.Printf("  valid until  %s\n", root.Cert.NotAfter.Format(time.RFC1123))
	fmt.Printf("  SHA-256      %s\n", ca.Fingerprint(root.Cert))
	if trusted {
		fmt.Println("  keychain     TRUSTED in the System keychain")
		fmt.Println("\n  Undo with:   aiul ca untrust      (or sudo ./scripts/killswitch.sh)")
	} else {
		fmt.Println("  keychain     not trusted (nothing on this Mac accepts certificates we mint)")
	}
	return 0
}

func caTrust(args []string) int {
	root, err := ca.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul ca trust: %v\n", err)
		return 1
	}
	certPath, _, _ := ca.Paths()
	installer := platform.Trust()

	fmt.Println("This changes a system setting on your Mac.")
	fmt.Println()
	fmt.Println("What it does: it tells macOS to trust the certificate below as a")
	fmt.Println("certificate authority for TLS. After this, any certificate signed by")
	fmt.Println("that key is accepted by Safari, Chrome, curl and most macOS software.")
	fmt.Println("That is powerful, which is why the key is on your machine at 0600 and")
	fmt.Println("why one command undoes it.")
	fmt.Println()
	fmt.Printf("  certificate  %s\n", root.Cert.Subject.CommonName)
	fmt.Printf("  file         %s\n", certPath)
	fmt.Printf("  SHA-256      %s\n", ca.Fingerprint(root.Cert))
	fmt.Println()
	fmt.Println("Exact command that will run (sudo will ask for your password):")
	for _, c := range installer.InstallCommands(certPath) {
		fmt.Printf("  $ %s\n", c)
	}
	fmt.Println()
	fmt.Println("Undo at any time:")
	fmt.Println("  $ aiul ca untrust")
	fmt.Println("  $ sudo ./scripts/killswitch.sh      # undoes this and every other change")
	fmt.Println()

	if !confirm(args, "Add this certificate to the System keychain?") {
		fmt.Println("Cancelled. Nothing was changed.")
		return 1
	}

	if err := installer.Install(certPath); err != nil {
		fmt.Fprintf(os.Stderr, "aiul ca trust: %v\n", err)
		return 1
	}
	fmt.Println("\nDone. Verify with: aiul ca info")
	return 0
}

func caUntrust(args []string) int {
	certPath, _, err := ca.Paths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul ca untrust: %v\n", err)
		return 1
	}
	installer := platform.Trust()

	fmt.Println("Removing our development root CA from the System keychain.")
	fmt.Println("Exact commands (sudo will ask for your password):")
	for _, c := range installer.UninstallCommands(certPath) {
		fmt.Printf("  $ %s\n", c)
	}
	fmt.Println()

	if !confirm(args, "Remove it now?") {
		fmt.Println("Cancelled. Nothing was changed.")
		return 1
	}

	if err := installer.Uninstall(certPath); err != nil {
		fmt.Fprintf(os.Stderr, "aiul ca untrust: %v\n", err)
		return 1
	}
	fmt.Println("\nRemoved. The CA files are still on disk; delete them by hand if you want them gone.")
	return 0
}

// caDemoServer exists for the Phase 1 milestone: it proves in a browser that a
// certificate our own code minted is accepted once the root is trusted, and
// rejected once it is not.
func caDemoServer(args []string) int {
	root, err := ca.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul ca demo-server: %v\n", err)
		return 1
	}

	cert, err := root.MintLeaf(ca.LeafRequest{Hosts: []string{"localhost", "127.0.0.1"}})
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul ca demo-server: %v\n", err)
		return 1
	}

	addr := "127.0.0.1:8443" // loopback only: not reachable from your network
	srv := &http.Server{
		Addr:      addr,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{*cert}, MinVersion: tls.VersionTLS12},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<!doctype html><meta charset="utf-8">
<title>aiul certificate test</title>
<h1>It worked.</h1>
<p>This page was served over TLS using a certificate minted by <code>internal/ca</code>
and signed by <strong>%s</strong>.</p>
<p>If the browser showed no warning, the root is trusted. After
<code>aiul ca untrust</code> the same page must show a warning again.</p>
<p>Leaf valid until %s.</p>`, root.Cert.Subject.CommonName, cert.Leaf.NotAfter.Format(time.RFC1123))
		}),
	}

	fmt.Printf("Serving https://localhost:8443/ with a freshly minted certificate.\n")
	fmt.Printf("  signed by    %s\n", root.Cert.Subject.CommonName)
	fmt.Printf("  leaf names   %v\n", cert.Leaf.DNSNames)
	fmt.Println("  stop with    Ctrl-C")
	// Certificate and key come from TLSConfig, so both arguments are empty.
	if err := srv.ListenAndServeTLS("", ""); err != nil {
		fmt.Fprintf(os.Stderr, "aiul ca demo-server: %v\n", err)
		return 1
	}
	return 0
}

// confirm asks a yes/no question unless --yes was passed. Rule 1: no system change
// without an explicit yes.
func confirm(args []string, question string) bool {
	if contains(args, "--yes") || contains(args, "-y") {
		return true
	}
	fmt.Printf("%s [y/N]: ", question)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func contains(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}
