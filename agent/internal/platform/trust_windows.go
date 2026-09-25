//go:build windows

package platform

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WindowsTrust puts our root in the machine's "Trusted Root Certification
// Authorities" store (LocalMachine\Root). Chrome, Edge, and Node with
// NODE_USE_SYSTEM_CA read it; Firefox keeps its own list and is not covered.
//
// It talks to the store through crypt32, Windows' certificate library, rather
// than certutil, so it can find our roots by name and remove older ones.
type WindowsTrust struct{}

func Trust() TrustInstaller { return WindowsTrust{} }

func (WindowsTrust) InstallCommands(certPath string) []string {
	return []string{fmt.Sprintf(`certutil -addstore Root "%s"`, certPath)}
}

func (WindowsTrust) UninstallCommands(string) []string {
	return []string{`certutil -delstore Root "AIUL Dev Root"   # every root of ours, by name`}
}

func (WindowsTrust) Install(certPath string) error {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("certificate not found: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("%s is not a PEM certificate", certPath)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	store, err := openMachineRoot()
	if err != nil {
		return err
	}
	defer windows.CertCloseStore(store, 0)

	// An older root of ours from a previous install: remove it, so exactly one
	// is trusted.
	removeRoots(store, func(c *x509.Certificate) bool {
		return hasOurPrefix(c) && !c.Equal(cert)
	})

	ctx, err := windows.CertCreateCertificateContext(
		windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING,
		&block.Bytes[0], uint32(len(block.Bytes)))
	if err != nil {
		return fmt.Errorf("read the CA: %w", err)
	}
	defer windows.CertFreeCertificateContext(ctx)
	if err := windows.CertAddCertificateContextToStore(store, ctx,
		windows.CERT_STORE_ADD_REPLACE_EXISTING, nil); err != nil {
		return fmt.Errorf("add the CA to LocalMachine\\Root: %w", err)
	}
	return nil
}

func (WindowsTrust) Uninstall(string) error {
	store, err := openMachineRoot()
	if err != nil {
		return err
	}
	defer windows.CertCloseStore(store, 0)
	removeRoots(store, hasOurPrefix)
	return nil
}

func (WindowsTrust) IsTrusted(commonName string) (bool, error) {
	store, err := openMachineRoot()
	if err != nil {
		return false, err
	}
	defer windows.CertCloseStore(store, 0)

	found := false
	eachCert(store, func(_ *windows.CertContext, c *x509.Certificate) {
		if strings.HasPrefix(c.Subject.CommonName, commonName) {
			found = true
		}
	})
	return found, nil
}

// ourRootPrefix matches ca.CommonNamePrefix; repeated because internal/ca
// cannot be imported from here without a cycle on other systems.
const ourRootPrefix = "AIUL Dev Root"

func hasOurPrefix(c *x509.Certificate) bool {
	return strings.HasPrefix(c.Subject.CommonName, ourRootPrefix)
}

func openMachineRoot() (windows.Handle, error) {
	name, _ := windows.UTF16PtrFromString("ROOT")
	store, err := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM, 0, 0,
		windows.CERT_SYSTEM_STORE_LOCAL_MACHINE, uintptr(unsafe.Pointer(name)))
	if err != nil {
		return 0, fmt.Errorf("open LocalMachine\\Root: %w", err)
	}
	return store, nil
}

// eachCert calls fn for every certificate in the store that parses.
func eachCert(store windows.Handle, fn func(*windows.CertContext, *x509.Certificate)) {
	var ctx *windows.CertContext
	for {
		next, err := windows.CertEnumCertificatesInStore(store, ctx)
		if err != nil || next == nil {
			return
		}
		ctx = next
		der := unsafe.Slice(ctx.EncodedCert, ctx.Length)
		if c, err := x509.ParseCertificate(der); err == nil {
			fn(ctx, c)
		}
	}
}

// removeRoots deletes every matching certificate. Deleting frees the context,
// which would end the walk, so matches are collected as copies first.
func removeRoots(store windows.Handle, match func(*x509.Certificate) bool) {
	var doomed []*windows.CertContext
	eachCert(store, func(ctx *windows.CertContext, c *x509.Certificate) {
		if match(c) {
			doomed = append(doomed, windows.CertDuplicateCertificateContext(ctx))
		}
	})
	for _, ctx := range doomed {
		_ = windows.CertDeleteCertificateFromStore(ctx)
	}
}
