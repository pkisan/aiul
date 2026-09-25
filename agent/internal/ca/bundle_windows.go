//go:build windows

package ca

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows keeps its trusted roots in a certificate store, not a file, so the
// bundle is exported from LocalMachine\Root: every certificate there, as PEM.
// Our own roots are skipped; WriteBundle appends the current one.
func init() {
	systemRoots = func() ([]byte, string, error) {
		const source = `the LocalMachine\Root certificate store`
		name, _ := windows.UTF16PtrFromString("ROOT")
		store, err := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM, 0, 0,
			windows.CERT_SYSTEM_STORE_LOCAL_MACHINE|windows.CERT_STORE_READONLY_FLAG,
			uintptr(unsafe.Pointer(name)))
		if err != nil {
			return nil, "", fmt.Errorf("open %s: %w", source, err)
		}
		defer windows.CertCloseStore(store, 0)

		var out bytes.Buffer
		var ctx *windows.CertContext
		for {
			ctx, err = windows.CertEnumCertificatesInStore(store, ctx)
			if err != nil || ctx == nil {
				break
			}
			der := bytes.Clone(unsafe.Slice(ctx.EncodedCert, ctx.Length))
			if c, err := x509.ParseCertificate(der); err != nil || strings.HasPrefix(c.Subject.CommonName, CommonNamePrefix) {
				continue
			}
			_ = pem.Encode(&out, &pem.Block{Type: "CERTIFICATE", Bytes: der})
		}
		if out.Len() == 0 {
			return nil, "", fmt.Errorf("no certificates in %s", source)
		}
		return out.Bytes(), source, nil
	}
}
