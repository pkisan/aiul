package ca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"time"
)

// LeafValidity is how long a minted server certificate is valid. Short on purpose:
// a leaf that leaks is useless within a day, and nothing we mint ever needs to
// outlive the proxy process that made it.
const LeafValidity = 24 * time.Hour

// LeafRequest describes the certificate to mint.
type LeafRequest struct {
	// Hosts are the names this certificate is valid for: DNS names, IP addresses,
	// or both. In the proxy these are copied from the real provider's certificate
	// so our copy claims exactly the same names and nothing more.
	Hosts []string

	// NotBefore defaults to one hour ago, NotAfter to NotBefore + LeafValidity.
	// Both exist so tests can mint an already-expired certificate.
	NotBefore time.Time
	NotAfter  time.Time
}

// MintLeaf signs a server certificate with the root itself.
//
// This is the development path — `aiul ca demo-server`, and running by hand before
// a device has an intermediate. The installed agent mints from its device
// intermediate instead (D3), so that what signs its certificates is name
// constrained and expires within days.
func (r *Root) MintLeaf(req LeafRequest) (*tls.Certificate, error) {
	return mintLeaf(req, r.Cert, r.Key, [][]byte{r.Cert.Raw})
}

// MintLeaf signs a server certificate with this device's intermediate.
//
// The chain served is leaf, intermediate, root: a client holding only the root can
// still build a path, and the intermediate's name constraints travel with it, so
// the client itself enforces which hosts we are allowed to impersonate.
func (i *Intermediate) MintLeaf(req LeafRequest) (*tls.Certificate, error) {
	chain := [][]byte{i.Cert.Raw}
	if len(i.RootDER) > 0 {
		chain = append(chain, i.RootDER)
	}

	return mintLeaf(req, i.Cert, i.Key, chain)
}

// mintLeaf is the shared body. A "leaf" is an ordinary end-entity certificate: it
// may identify a server, but it may not sign anything else. Each call generates a
// fresh key pair, so the private key of one minted certificate is useless against
// another.
func mintLeaf(req LeafRequest, issuerCert *x509.Certificate, issuerKey *ecdsa.PrivateKey, issuerChain [][]byte) (*tls.Certificate, error) {
	if len(req.Hosts) == 0 {
		return nil, fmt.Errorf("mint leaf: no hosts given")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("mint leaf: generate key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, fmt.Errorf("mint leaf: %w", err)
	}

	notBefore := req.NotBefore
	if notBefore.IsZero() {
		notBefore = time.Now().Add(-1 * time.Hour) // tolerate small clock differences
	}
	notAfter := req.NotAfter
	if notAfter.IsZero() {
		notAfter = notBefore.Add(LeafValidity + time.Hour)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: req.Hosts[0], Organization: []string{organization}},
		NotBefore:    notBefore,
		NotAfter:     notAfter,

		// Modern clients ignore the CommonName entirely and read only these Subject
		// Alternative Name fields, so every host must appear here.
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},

		// Explicitly not a CA: this certificate cannot sign others.
		IsCA:                  false,
		BasicConstraintsValid: true,
	}

	for _, h := range req.Hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}

	skid, err := subjectKeyID(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("mint leaf: %w", err)
	}
	tmpl.SubjectKeyId = skid

	// Template, issuer certificate, the leaf's public key, the issuer's private
	// key. The issuer is the root in development and the device intermediate once
	// one exists.
	der, err := x509.CreateCertificate(rand.Reader, tmpl, issuerCert, &key.PublicKey, issuerKey)
	if err != nil {
		return nil, fmt.Errorf("mint leaf: sign: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("mint leaf: parse: %w", err)
	}

	return &tls.Certificate{
		// The chain we serve: our leaf first, then whatever signed it, so a client
		// that has the root but has not been handed the rest in this connection can
		// still build a path to it.
		Certificate: append([][]byte{der}, issuerChain...),
		PrivateKey:  key,
		Leaf:        leaf,
	}, nil
}
