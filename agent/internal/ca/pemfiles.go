package ca

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"runtime"
)

// Reading and writing the PEM files this package keeps: the root, and each
// device's intermediate. One place, because a private key written with the wrong
// permissions is a real problem and it should only be possible to get that wrong
// once.

// writeCertPEM writes a certificate. 0644: a certificate is public by design, and
// other tools have to be able to read it.
func writeCertPEM(path string, der []byte) error {
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// writeKeyPEM writes a private key at 0600 — readable only by its owner. Rule 9.
func writeKeyPEM(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("encode private key: %w", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

func readCertPEM(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s does not contain a PEM certificate", path)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return cert, nil
}

// readKeyPEM reads a private key, refusing one that other users on this machine
// can read. A key with loose permissions is treated as compromised, not as a
// warning to print.
func readKeyPEM(path string) (*ecdsa.PrivateKey, error) {
	// Windows has no mode bits: Go reports 0666 for any writable file, and access
	// is decided by the ACL instead. The key lives under %LOCALAPPDATA%, which only
	// its owner (and administrators) can read by default.
	// ponytail: no ACL check on Windows yet; add one with x/sys/windows in W4,
	// when the key moves to a machine-wide directory.
	if info, err := os.Stat(path); err == nil && runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode&0o077 != 0 {
			return nil, fmt.Errorf("%s has permissions %#o; it must be 0600. Fix with: chmod 600 %s", path, mode, path)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s does not contain a PEM private key", path)
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return key, nil
}
