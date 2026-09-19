package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/pkisan/aiul/internal/ca"
	"github.com/pkisan/aiul/internal/platform"
)

// The proxy address the agent configures everything to use.
const proxyAddr = "127.0.0.1:8899"

func proxyURL() string { return "http://" + proxyAddr }

const installUsage = `Usage:
  aiul install [--apply] [--yes]

Without --apply this is a DRY RUN: it prints every command it would run and
changes nothing. That is the default on purpose.

With --apply it will, as root:
  - create the _aiul service account, which the worker runs as
  - install the binary and TWO launchd jobs: a tiny root helper, and the worker
    (proxy, parsing, redaction, forwarding) running unprivileged as _aiul
  - trust the development CA in the System keychain
  - point every network service's HTTPS proxy at ` + proxyAddr + `
  - write environment variables to /etc/zshenv and a login LaunchAgent

Undo all of it with 'sudo aiul uninstall' or 'sudo ./scripts/killswitch.sh'.
`

func cmdInstall(args []string) int {
	apply := contains(args, "--apply")

	root, err := ca.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul install: %v\n", err)
		return 1
	}
	certPath, _, _ := ca.Paths()

	// Write the combined bundle (the system roots plus ours) before planning, so
	// the variables that REPLACE the trust store have something correct to point
	// at. Writing it changes no setting; it is a file in our own directory.
	bundlePath, bundleErr := root.WriteBundle()
	if bundleErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write the combined CA bundle: %v\n", bundleErr)
		fmt.Fprintln(os.Stderr, "SSL_CERT_FILE would then point at our root alone, which breaks ordinary HTTPS.")
		return 1
	}
	vars := platform.DefaultEnvVars(proxyURL(), certPath, bundlePath)

	// The MDM gate comes first: on an unmanaged machine we stop here, before
	// printing anything that looks like a plan.
	enrolled, detail, _ := platform.MDM().Enrolled()
	if !enrolled {
		fmt.Fprintln(os.Stderr, "This device is not enrolled in an MDM, so the agent will not install.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Intercepting traffic is appropriate on a company-owned, managed device and")
		fmt.Fprintln(os.Stderr, "not on a personal one, so this check is deliberate.")
		fmt.Fprintf(os.Stderr, "\n  profiles status -type enrollment said:\n  %s\n", indent(detail))
		fmt.Fprintf(os.Stderr, "\nFor development on your own machine, set %s=1.\n", platform.DevAllowUnmanagedVar)
		return 1
	}
	if strings.HasPrefix(detail, "DEVELOPER OVERRIDE") {
		fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		fmt.Println("!! " + platform.DevAllowUnmanagedVar + "=1 — the MDM check was SKIPPED.")
		fmt.Println("!! This is for development only and must never be set on a real device.")
		fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		fmt.Println()
	}

	self, _ := os.Executable()

	fmt.Println("What 'aiul install --apply' will do to this Mac")
	fmt.Println("===============================================")
	fmt.Println()
	fmt.Println("1. Create the service account the worker runs as")
	fmt.Println("   The code that parses network traffic does NOT run as root: it runs as")
	fmt.Printf("   %s, a hidden account with no login shell and no home directory.\n", platform.ServiceUserName)
	printCommands(platform.CreateServiceAccountCommands())
	fmt.Println("2. Run the agent in the background, as two processes")
	printCommands(platform.Service().InstallCommands())
	fmt.Println("3. Trust our development CA, so software accepts the certificates we mint")
	fmt.Printf("   certificate: %s\n", root.Cert.Subject.CommonName)
	fmt.Printf("   SHA-256:     %s\n", ca.Fingerprint(root.Cert))
	printCommands(platform.Trust().InstallCommands(certPath))
	fmt.Printf("4. Send HTTPS traffic through %s\n", proxyAddr)
	printCommands(platform.Proxy().SetCommands(proxyAddr))
	fmt.Println("5. Set environment variables so CLI runtimes trust our CA")
	for _, k := range sortedEnvKeys(vars) {
		fmt.Printf("   %-22s %s\n", k, vars[k])
	}
	printCommands(platform.Env().WriteCommands(vars))

	fmt.Println("To undo everything, at any time:")
	fmt.Println("   $ sudo aiul uninstall")
	fmt.Println("   $ sudo ./scripts/killswitch.sh")
	fmt.Println()

	if !apply {
		fmt.Println("This was a DRY RUN. Nothing was changed.")
		fmt.Println("Re-run with --apply to make these changes.")
		return 0
	}

	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "aiul install --apply must run as root: sudo aiul install --apply")
		return 1
	}
	if !confirm(args, "Apply all of the above to this Mac?") {
		fmt.Println("Cancelled. Nothing was changed.")
		return 1
	}

	// Order matters: trust and the background job first, then the proxy setting.
	// If the machine is left half-configured, it must be left in the state where
	// traffic still flows normally.
	// Anything the installed jobs need must be written into the job definition:
	// launchd does not inherit the shell that ran install. The MDM override is the
	// one that matters, because without it the worker refuses to start on a
	// development machine.
	daemonEnv := map[string]string{}
	if os.Getenv(platform.DevAllowUnmanagedVar) != "" {
		daemonEnv[platform.DevAllowUnmanagedVar] = os.Getenv(platform.DevAllowUnmanagedVar)
	}

	steps := []struct {
		name string
		do   func() error
	}{
		{"create the service account and the background jobs", func() error { return platform.Service().Install(self, daemonEnv) }},
		// Nothing points traffic at the proxy until the proxy answers. This check
		// is the difference between a failed install and a Mac with no working
		// HTTPS, because the proxy setting outlives the process that set it.
		{"wait for the worker to start listening", waitForProxy},
		{"trust the CA", func() error { return platform.Trust().Install(certPath) }},
		{"write environment variables", func() error { return platform.Env().Write(vars) }},
		{"set the system proxy", func() error { return platform.Proxy().Set(proxyAddr) }},
	}
	for _, step := range steps {
		fmt.Printf("… %s\n", step.name)
		if err := step.do(); err != nil {
			fmt.Fprintf(os.Stderr, "\nFailed to %s: %v\n", step.name, err)
			fmt.Fprintln(os.Stderr, "Rolling back so this Mac is left working.")
			// In this order, so traffic is flowing normally before anything else is
			// touched. A half-finished install must never leave a proxy setting
			// behind: the setting outlives the process that wrote it.
			_ = platform.Proxy().Unset()
			_ = platform.Env().Remove()
			_ = platform.Service().Uninstall()
			fmt.Fprintf(os.Stderr, "Rolled back. The worker's log is %s.\n", platform.WorkerLogPath)
			return 1
		}
	}

	fmt.Println("\nDone. Check it with: aiul status")
	fmt.Println("Open a NEW terminal window for the environment variables to apply,")
	fmt.Println("and log out and back in for GUI applications to see them.")
	return 0
}

const uninstallUsage = `Usage:
  aiul uninstall [--yes]

Removes everything aiul changed: the proxy setting on every network service, the
environment variables, the /etc/zshenv block, both launchd jobs, the installed
binary, the CA trust and the stored device token. Safe to run twice.
`

func cmdUninstall(args []string) int {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "aiul uninstall must run as root: sudo aiul uninstall")
		return 1
	}

	certPath, _, _ := ca.Paths()

	fmt.Println("This will remove every change aiul made to this Mac:")
	printCommands(platform.Proxy().UnsetCommands())
	printCommands(platform.Env().RemoveCommands())
	printCommands(platform.Service().UninstallCommands())
	printCommands(platform.Trust().UninstallCommands(certPath))
	fmt.Println()

	if !confirm(args, "Remove all of it?") {
		fmt.Println("Cancelled. Nothing was changed.")
		return 1
	}

	// Remove the proxy setting FIRST, so the machine's traffic is flowing normally
	// before anything else is touched. Rule 7.
	steps := []struct {
		name string
		do   func() error
	}{
		{"remove the system proxy", platform.Proxy().Unset},
		{"remove the environment variables", platform.Env().Remove},
		{"remove the background jobs", platform.Service().Uninstall},
		{"remove the CA trust", func() error { return platform.Trust().Uninstall(certPath) }},
		{"remove the device token", platform.DeleteDeviceToken},
	}

	failed := false
	for _, step := range steps {
		fmt.Printf("… %s\n", step.name)
		if err := step.do(); err != nil {
			// Keep going: a half-finished uninstall is worse than a noisy one.
			fmt.Fprintf(os.Stderr, "   could not %s: %v\n", step.name, err)
			failed = true
		}
	}

	fmt.Println("\nThe CA files under ~/Library/Application Support/AIUL/ were left in place.")
	fmt.Println("Delete them by hand if you want them gone, along with the spooled events.")
	if failed {
		fmt.Fprintln(os.Stderr, "\nSome steps failed. Run 'sudo ./scripts/killswitch.sh' and then 'aiul status'.")
		return 1
	}
	fmt.Println("Done. Verify with: aiul status")
	return 0
}

// waitForProxy blocks until the worker accepts a connection on the proxy port, or
// gives up. launchd starts the job in the background, so "the job loaded" is not
// the same as "the proxy works" — and the difference is a Mac whose HTTPS traffic
// is pointed at nothing.
func waitForProxy() error {
	deadline := time.Now().Add(20 * time.Second)

	for {
		conn, err := net.DialTimeout("tcp", proxyAddr, 2*time.Second)
		if err == nil {
			_ = conn.Close()

			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("nothing is listening on %s after 20s: %w\n"+
				"  The worker did not start. Its log says why: %s",
				proxyAddr, err, platform.WorkerLogPath)
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func printCommands(cmds []string) {
	for _, c := range cmds {
		fmt.Printf("   $ %s\n", c)
	}
	fmt.Println()
}

func sortedEnvKeys(vars platform.EnvVars) []string {
	out := make([]string, 0, len(vars))
	for k := range vars {
		out = append(out, k)
	}
	// A plain sort keeps the display stable between runs.
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func indent(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n  ")
}
