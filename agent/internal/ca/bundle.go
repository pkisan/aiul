package ca

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// Some tools read a CA BUNDLE rather than a list of extra certificates, and for
// those the variable REPLACES the trust store instead of adding to it:
//
//	NODE_EXTRA_CA_CERTS   adds to Node's built-in roots        -> our root alone is right
//	SSL_CERT_FILE         REPLACES the roots for OpenSSL/curl  -> must be a full bundle
//	REQUESTS_CA_BUNDLE    REPLACES the roots for Python        -> must be a full bundle
//	CODEX_CA_CERTIFICATE  Rust/rustls                          -> use the full bundle to be safe
//
// Pointing those at our root alone would mean curl could no longer verify any
// ordinary website — every connection we deliberately pass through sealed would
// fail. So we write a bundle: the system roots, plus ours.

// systemBundles are the files macOS and Homebrew ship, in order of preference.
var systemBundles = []string{
	"/etc/ssl/cert.pem",
	"/opt/homebrew/etc/ca-certificates/cert.pem",
	"/usr/local/etc/ca-certificates/cert.pem",
}

// BundlePath is where the combined bundle is written.
func BundlePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ca-bundle.pem"), nil
}

// WriteBundle writes "the normal system roots, plus our root" to BundlePath and
// returns that path.
func (r *Root) WriteBundle() (string, error) {
	path, err := BundlePath()
	if err != nil {
		return "", err
	}

	system, source, err := readSystemBundle()
	if err != nil {
		return "", err
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "# Written by aiul. The system roots from %s, plus our development root.\n", source)
	fmt.Fprintf(&b, "# Do not edit; 'aiul install' rewrites it. Delete it with the CA directory.\n\n")
	b.Write(system)
	if !bytes.HasSuffix(system, []byte("\n")) {
		b.WriteByte('\n')
	}
	b.WriteString("\n# AIUL development root\n")
	b.Write(r.CertPEM)

	// World-readable on purpose: every tool on the machine has to read it, and it
	// contains only public certificates.
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func readSystemBundle() ([]byte, string, error) {
	for _, candidate := range systemBundles {
		data, err := os.ReadFile(candidate)
		if err == nil && bytes.Contains(data, []byte("BEGIN CERTIFICATE")) {
			return data, candidate, nil
		}
	}
	return nil, "", fmt.Errorf("no system CA bundle found in %v; without one, setting SSL_CERT_FILE would break ordinary HTTPS", systemBundles)
}
