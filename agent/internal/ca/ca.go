// Package ca creates and uses the development certificate authority.
//
// Background in plain terms. A certificate authority (CA) is a key pair whose
// certificate other software has been told to trust. Anything that key signs is
// believed. Our proxy needs to present a certificate for, say, api.openai.com that
// the client accepts; it can only do that by signing one with a CA the machine
// trusts. So we create our own CA ("the root"), trust it on this Mac, and use it
// to sign short-lived "leaf" certificates for the hostnames we intercept.
//
// This is the DEVELOPMENT CA only: one self-signed root, its key in a file on this
// laptop. The production chain (per-tenant root in a KMS/HSM signing a short-lived,
// name-constrained intermediate per device) is designed in docs/DECISIONS.md and
// deliberately not built here.
package ca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"os/user"
	"path/filepath"

	"github.com/pkisan/aiul/internal/paths"
	"time"
)

const (
	// rootValidity is how long the dev root is good for. Long enough not to be a
	// nuisance, short enough that a forgotten dev CA eventually stops working.
	rootValidity = 3 * 365 * 24 * time.Hour

	// CommonNamePrefix must stay in sync with scripts/killswitch.sh, which finds
	// the certificate in the keychain by this name.
	CommonNamePrefix = "AIUL Dev Root"

	organization = "AI Usage Logger (development)"
)

// Dir returns the directory holding the dev CA files. Where that is depends on
// how the agent is running — see internal/paths.
func Dir() (string, error) {
	return paths.CADir()
}

// Paths of the two files that make up the CA.
func Paths() (certPath, keyPath string, err error) {
	dir, err := Dir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "root.crt"), filepath.Join(dir, "root.key"), nil
}

// Root is a loaded certificate authority: the certificate everyone sees and the
// private key that must never leave this machine.
type Root struct {
	Cert *x509.Certificate
	Key  *ecdsa.PrivateKey

	// CertPEM is the certificate in PEM form — the text format starting with
	// "-----BEGIN CERTIFICATE-----" that tools such as NODE_EXTRA_CA_CERTS expect.
	CertPEM []byte
}

// Exists reports whether a dev CA has already been created.
func Exists() bool {
	certPath, keyPath, err := Paths()
	if err != nil {
		return false
	}
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	return certErr == nil && keyErr == nil
}

// Init creates a new dev root CA and writes it to disk. It refuses to overwrite an
// existing CA unless force is true, because overwriting silently would leave a
// trusted-but-useless certificate in the keychain.
func Init(force bool) (*Root, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	certPath, keyPath, err := Paths()
	if err != nil {
		return nil, err
	}

	if Exists() && !force {
		return nil, fmt.Errorf("a dev CA already exists at %s (use --force to replace it, and run 'aiul ca untrust' first)", dir)
	}

	// 0700: only the owner may even list this directory.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}

	root, err := generateRoot()
	if err != nil {
		return nil, err
	}

	keyDER, err := x509.MarshalECPrivateKey(root.Key)
	if err != nil {
		return nil, fmt.Errorf("encode private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	// The private key is written 0600 — readable only by its owner. Rule 9.
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", keyPath, err)
	}
	// The certificate is public by design, so 0644 is fine and other tools can read it.
	if err := os.WriteFile(certPath, root.CertPEM, 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", certPath, err)
	}
	return root, nil
}

// generateRoot builds the self-signed root certificate in memory.
func generateRoot() (*Root, error) {
	// ECDSA P-256: a modern key type every current client accepts, and much faster
	// to generate and sign with than RSA — which matters because the proxy mints a
	// leaf certificate per hostname on demand.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown-host"
	}
	who := "unknown-user"
	if u, err := user.Current(); err == nil && u.Username != "" {
		who = u.Username
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			// A clear, obviously-not-a-real-CA name, so this is recognisable in
			// Keychain Access and in any browser's certificate viewer.
			CommonName:         fmt.Sprintf("%s - %s", CommonNamePrefix, hostname),
			Organization:       []string{organization},
			OrganizationalUnit: []string{"user:" + who},
		},
		// Backdate by an hour so a slightly slow clock on another machine does not
		// reject a certificate that was only just created.
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(rootValidity),

		// IsCA plus BasicConstraintsValid is what marks this as a certificate
		// authority rather than an ordinary server certificate.
		IsCA:                  true,
		BasicConstraintsValid: true,
		// MaxPathLenZero: this root may sign leaf certificates but no further CAs.
		MaxPathLen:     0,
		MaxPathLenZero: true,

		// A CA key is only ever used to sign certificates and revocation lists.
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	// SubjectKeyId is the fingerprint of the public key. Verifiers use it to match
	// a leaf certificate to the CA that issued it.
	skid, err := subjectKeyID(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	tmpl.SubjectKeyId = skid

	// Self-signed: the template is both the certificate being created and its
	// issuer, signed with its own key.
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create root certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse created certificate: %w", err)
	}

	return &Root{
		Cert:    cert,
		Key:     key,
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}, nil
}

// Load reads the dev CA from disk.
func Load() (*Root, error) {
	certPath, keyPath, err := Paths()
	if err != nil {
		return nil, err
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w (run 'aiul ca init' first)", certPath, err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w (run 'aiul ca init' first)", keyPath, err)
	}

	// Refuse to use a private key that other users on this Mac can read.
	if info, err := os.Stat(keyPath); err == nil {
		if mode := info.Mode().Perm(); mode&0o077 != 0 {
			return nil, fmt.Errorf("%s has permissions %#o; it must be 0600. Fix with: chmod 600 %s", keyPath, mode, keyPath)
		}
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s does not contain a PEM certificate", certPath)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", certPath, err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("%s does not contain a PEM private key", keyPath)
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", keyPath, err)
	}

	if !cert.IsCA {
		return nil, fmt.Errorf("%s is not a CA certificate", certPath)
	}
	return &Root{Cert: cert, Key: key, CertPEM: certPEM}, nil
}

// Fingerprint returns the SHA-256 fingerprint of a certificate, formatted the way
// Keychain Access and browsers show it. Used to prove on screen that the
// certificate we trusted is the one we created.
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	out := make([]byte, 0, len(sum)*3)
	const hexDigits = "0123456789ABCDEF"
	for i, b := range sum {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, hexDigits[b>>4], hexDigits[b&0x0f])
	}
	return string(out)
}

// randomSerial produces a 128-bit random serial number. Serial numbers must be
// unique per issuer and unpredictable.
func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}
	// Serial numbers must be positive; avoid an accidental zero.
	return serial.Add(serial, big.NewInt(1)), nil
}

// subjectKeyID is the SHA-1 hash of the public key, as RFC 5280 recommends. SHA-1
// is used here only as an identifier, never as a signature, so it is not a
// security weakness.
func subjectKeyID(pub *ecdsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}
	// A public key in DER form is a SubjectPublicKeyInfo structure: the algorithm,
	// then the key bits. RFC 5280 hashes only the key bits, so unwrap it first.
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	if _, err := asn1.Unmarshal(der, &spki); err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	sum := sha256.Sum256(spki.PublicKey.Bytes)
	return sum[:20], nil
}
