package main

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/pkisan/aiul/internal/ca"
	"github.com/pkisan/aiul/internal/proxy"
)

// This device's signing certificate (D3).
//
// The root signs one short-lived, name-constrained intermediate per device, and
// every certificate the proxy mints is signed by that intermediate rather than by
// the root. Two consequences, and they are the reason the chain exists:
//
//   - the intermediate expires within days, so a device that leaves the fleet stops
//     being able to mint anything without anyone having to revoke it
//   - it may only sign for hosts on the allow-list. A client REJECTS a certificate
//     from it for any other name, so even with the device's key in hand nobody can
//     impersonate a bank
//
// deviceIssuer keeps the current intermediate behind a lock, because the proxy
// mints from it on many connections at once while the health loop may replace it.
type deviceIssuer struct {
	mu    sync.RWMutex
	inter *ca.Intermediate
}

func (d *deviceIssuer) MintLeaf(req ca.LeafRequest) (*tls.Certificate, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.inter.MintLeaf(req)
}

// set replaces the intermediate in use. Certificates already minted stay valid
// until they expire; they were signed by a certificate that is still within its
// own validity, and the chain they carry includes it.
func (d *deviceIssuer) set(inter *ca.Intermediate) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inter = inter
}

func (d *deviceIssuer) current() *ca.Intermediate {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.inter
}

// provisionIssuer returns the issuer the proxy should mint from, creating or
// renewing this device's intermediate as needed.
func provisionIssuer(log *slog.Logger, root *ca.Root) (*deviceIssuer, error) {
	issuer := &deviceIssuer{}
	if err := renewIfNeeded(log, root, issuer); err != nil {
		return nil, err
	}

	return issuer, nil
}

// renewIfNeeded provisions the intermediate when it is missing, close to expiry, or
// constrained to a different set of hosts than the allow-list now holds.
func renewIfNeeded(log *slog.Logger, root *ca.Root, issuer *deviceIssuer) error {
	hostname, _ := os.Hostname()
	permitted := ca.PermittedDomainsFrom(proxy.AllowListEntries())

	inter, issued, err := ca.ProvisionDevice(root, hostname, permitted)
	if err != nil {
		return err
	}
	issuer.set(inter)

	if issued {
		log.Info("issued a signing certificate for this device",
			"expires", inter.Cert.NotAfter.Format(time.RFC3339),
			"permitted_hosts", len(permitted),
			"note", "this certificate cannot sign for any host outside the allow-list")
	}

	return nil
}

const deviceUsage = `Usage:
  aiul ca device [--renew]

Shows this device's signing certificate: what signed it, when it expires, and
which hostnames it is allowed to sign for. --renew replaces it now rather than
waiting for the agent to do so.

The agent provisions and renews this by itself; this command is for looking.
`

func cmdCADevice(args []string) int {
	if contains(args, "-h") || contains(args, "--help") {
		fmt.Print(deviceUsage)
		return 0
	}

	root, err := ca.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul ca device: %v\n", err)
		return 1
	}

	permitted := ca.PermittedDomainsFrom(proxy.AllowListEntries())

	if contains(args, "--renew") {
		hostname, _ := os.Hostname()
		// Provision refuses to reissue a certificate that is still good, so remove
		// the old one first when the owner explicitly asked for a new one.
		certPath, keyPath, err := ca.DevicePaths()
		if err != nil {
			fmt.Fprintf(os.Stderr, "aiul ca device: %v\n", err)
			return 1
		}
		_ = os.Remove(certPath)
		_ = os.Remove(keyPath)

		if _, _, err := ca.ProvisionDevice(root, hostname, permitted); err != nil {
			fmt.Fprintf(os.Stderr, "aiul ca device: %v\n", err)
			return 1
		}
		fmt.Println("Issued a new signing certificate for this device.")
		fmt.Println()
	}

	inter, err := ca.LoadIntermediate(root)
	if err != nil {
		fmt.Println("This device has no signing certificate yet.")
		fmt.Println("The agent provisions one when it starts, or run: aiul ca device --renew")
		return 0
	}

	certPath, keyPath, _ := ca.DevicePaths()

	fmt.Println("This device's signing certificate")
	fmt.Println("================================")
	fmt.Println()
	fmt.Printf("  name           %s\n", inter.Cert.Subject.CommonName)
	fmt.Printf("  issued by      %s\n", inter.Cert.Issuer.CommonName)
	fmt.Printf("  certificate    %s\n", certPath)
	fmt.Printf("  private key    %s   (mode 0600, and it never leaves this device)\n", keyPath)
	fmt.Printf("  valid until    %s\n", inter.Cert.NotAfter.Format(time.RFC1123))

	remaining := time.Until(inter.Cert.NotAfter)
	switch {
	case remaining <= 0:
		fmt.Printf("  expired        %s ago — the agent will replace it on its next check\n", (-remaining).Round(time.Hour))
	case inter.NeedsRenewal():
		fmt.Printf("  renews         now (%v left, threshold %v)\n", remaining.Round(time.Hour), ca.RenewBefore)
	default:
		fmt.Printf("  renews         in %v\n", (remaining - ca.RenewBefore).Round(time.Hour))
	}

	fmt.Println()
	fmt.Printf("  It may sign certificates for these %d domains and their subdomains,\n", len(inter.Cert.PermittedDNSDomains))
	fmt.Println("  and a client will REFUSE a certificate from it for anything else:")
	for _, domain := range inter.Cert.PermittedDNSDomains {
		fmt.Printf("    %s\n", domain)
	}

	if !matchesAllowList(inter, permitted) {
		fmt.Println()
		fmt.Println("  NOTE: the allow-list has changed since this certificate was issued.")
		fmt.Println("  The agent will replace it on its next check, or run: aiul ca device --renew")
	}

	return 0
}

func matchesAllowList(inter *ca.Intermediate, permitted []string) bool {
	if len(inter.Cert.PermittedDNSDomains) != len(permitted) {
		return false
	}
	have := map[string]bool{}
	for _, d := range inter.Cert.PermittedDNSDomains {
		have[d] = true
	}
	for _, d := range permitted {
		if !have[d] {
			return false
		}
	}

	return true
}
