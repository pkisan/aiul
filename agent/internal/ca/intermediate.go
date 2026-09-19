package ca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkisan/aiul/internal/paths"
)

// The device intermediate, and why the product needs one (D3).
//
// A single self-signed root whose private key sits on the laptop is fine for one
// developer and unacceptable for a fleet: steal the laptop, take the key, and you
// can mint a certificate for any website in the world that the device trusts.
//
// The production chain puts a long-lived root out of reach — in KMS, one per
// tenant — and gives each device a SHORT-LIVED intermediate whose key is generated
// on that device and never leaves it. Two things then limit the damage a stolen
// laptop can do:
//
//   - the intermediate expires within days, so revocation is mostly just waiting
//   - the intermediate carries NAME CONSTRAINTS listing only the AI hostnames we
//     are allowed to decrypt. A correctly implemented client REJECTS any
//     certificate from that intermediate for any other name. That is enforced by
//     the verifier, not by our code, so a compromised device cannot impersonate a
//     bank even with the key in hand.
//
// The root's signature is the only part that still has to come from somewhere
// trusted. Today that is the local dev root; in production it is a KMS call.
// `Root.SignIntermediate` is the seam, and it takes a PUBLIC key — there is no
// code path anywhere that moves a device's private key.
const (
	// IntermediateValidity is deliberately short. A certificate nobody can revoke
	// promptly is a certificate you want to expire on its own.
	IntermediateValidity = 7 * 24 * time.Hour

	// RenewBefore is how much life must remain before the agent renews. Renewing
	// with two days left means a device that is asleep over a weekend still comes
	// back with a valid chain.
	RenewBefore = 2 * 24 * time.Hour
)

// Intermediate is a device's own signing certificate and key.
type Intermediate struct {
	Cert *x509.Certificate
	Key  *ecdsa.PrivateKey

	// RootDER is the issuing root, kept so the chain served to clients is
	// complete: leaf, then intermediate, then root.
	RootDER []byte
}

// IntermediateRequest is what the device asks the root to sign.
type IntermediateRequest struct {
	// PublicKey is the device key's public half. The private half stays on the
	// device; nothing here can carry it.
	PublicKey *ecdsa.PublicKey

	// DeviceName identifies the device in the certificate subject, so a
	// certificate found in the wild can be traced to one machine.
	DeviceName string

	// PermittedDNSDomains is the whole point: the names this intermediate may sign
	// for. Passed in rather than read from the allow-list directly, because the
	// allow-list lives in internal/proxy and these two packages must not depend on
	// each other.
	PermittedDNSDomains []string

	// NotBefore and NotAfter default to now-1h and NotBefore+IntermediateValidity.
	// They exist so tests can build an expired or nearly-expired intermediate.
	NotBefore time.Time
	NotAfter  time.Time
}

// SignIntermediate issues a device intermediate.
//
// In production this method is what moves to KMS: the template stays identical,
// and only the final signing call changes from "sign with the root key we hold" to
// "ask KMS to sign". Nothing else in the package knows the difference.
func (r *Root) SignIntermediate(req IntermediateRequest) (*x509.Certificate, error) {
	if req.PublicKey == nil {
		return nil, fmt.Errorf("sign intermediate: no public key given")
	}
	if len(req.PermittedDNSDomains) == 0 {
		// Refusing here is deliberate. An intermediate with no name constraints is
		// exactly the unrestricted signing certificate this design exists to
		// prevent, and it would be easy to produce by passing an empty slice.
		return nil, fmt.Errorf("sign intermediate: refusing to issue without name constraints")
	}

	// A root created before 2026-09-19 carries MaxPathLen 0, meaning "may sign
	// leaves, may not sign another CA". An intermediate under it would be rejected
	// by every verifier, so say so here rather than issuing a chain that cannot
	// work.
	if !r.CanIssueIntermediate() {
		return nil, fmt.Errorf("this root cannot issue an intermediate: it was created with a path length of 0. " +
			"Run 'aiul ca untrust' then 'aiul ca init --force' to create one that can")
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, fmt.Errorf("sign intermediate: %w", err)
	}

	notBefore := req.NotBefore
	if notBefore.IsZero() {
		notBefore = time.Now().Add(-1 * time.Hour)
	}
	notAfter := req.NotAfter
	if notAfter.IsZero() {
		notAfter = notBefore.Add(IntermediateValidity + time.Hour)
	}

	name := req.DeviceName
	if name == "" {
		name = "unknown device"
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("%s Device - %s", CommonNamePrefix, name),
			Organization: []string{organization},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		// A CA that may sign leaves and nothing else: MaxPathLen 0 with
		// MaxPathLenZero set means "no further CAs beneath this one".
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,

		// The constraint that makes a stolen device harmless for anything but AI
		// traffic. Critical, so a client that does not understand the extension
		// must reject the certificate rather than ignore the limit.
		PermittedDNSDomains:         req.PermittedDNSDomains,
		PermittedDNSDomainsCritical: true,
	}

	skid, err := subjectKeyID(req.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("sign intermediate: %w", err)
	}
	tmpl.SubjectKeyId = skid

	der, err := x509.CreateCertificate(rand.Reader, tmpl, r.Cert, req.PublicKey, r.Key)
	if err != nil {
		return nil, fmt.Errorf("sign intermediate: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("sign intermediate: parse: %w", err)
	}

	return cert, nil
}

// CanIssueIntermediate reports whether this root may sign a CA beneath it.
//
// A root written before the device chain existed carries a path length of 0, which
// means "leaves only". Certificates issued under such a root are rejected by every
// verifier, so the agent has to know the difference before it tries.
func (r *Root) CanIssueIntermediate() bool {
	return !(r.Cert.MaxPathLen == 0 && r.Cert.MaxPathLenZero)
}

// NewDeviceKey generates the key that stays on this device.
func NewDeviceKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// NeedsRenewal reports whether the intermediate is missing, expired, or close
// enough to expiry that it should be replaced now.
func (i *Intermediate) NeedsRenewal() bool {
	if i == nil || i.Cert == nil {
		return true
	}

	return time.Until(i.Cert.NotAfter) < RenewBefore
}

// PermittedDomainsFrom turns allow-list entries into name constraints.
//
// A "*.claude.ai" entry becomes "claude.ai", because an RFC 5280 DNS name
// constraint already covers a name and everything beneath it. Duplicates are
// dropped so the extension stays small.
func PermittedDomainsFrom(allowList []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(allowList))

	for _, entry := range allowList {
		domain := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(entry), "*."))
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}

	return out
}

// --- where the device's own files live ------------------------------------

// DeviceDir is where the intermediate and its key are kept: beside the CA, in
// whichever state directory this process uses.
func DeviceDir() (string, error) {
	base, err := paths.State()
	if err != nil {
		return "", err
	}

	return filepath.Join(base, "device"), nil
}

// DevicePaths returns the certificate and key paths.
func DevicePaths() (certPath, keyPath string, err error) {
	dir, err := DeviceDir()
	if err != nil {
		return "", "", err
	}

	return filepath.Join(dir, "intermediate.crt"), filepath.Join(dir, "intermediate.key"), nil
}

// Save writes the intermediate and its key, the key readable only by its owner.
func (i *Intermediate) Save() error {
	dir, err := DeviceDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	certPath, keyPath, err := DevicePaths()
	if err != nil {
		return err
	}

	if err := writeCertPEM(certPath, i.Cert.Raw); err != nil {
		return err
	}

	return writeKeyPEM(keyPath, i.Key)
}

// LoadIntermediate reads this device's intermediate, if it has one.
func LoadIntermediate(root *Root) (*Intermediate, error) {
	certPath, keyPath, err := DevicePaths()
	if err != nil {
		return nil, err
	}

	cert, err := readCertPEM(certPath)
	if err != nil {
		return nil, err
	}
	key, err := readKeyPEM(keyPath)
	if err != nil {
		return nil, err
	}

	inter := &Intermediate{Cert: cert, Key: key}
	if root != nil {
		inter.RootDER = root.Cert.Raw
	}

	return inter, nil
}

// ProvisionDevice returns a usable intermediate, creating or renewing one if
// needed. It reports whether it had to issue a new certificate, so the caller can
// say so in its log rather than silently rotating a key.
func ProvisionDevice(root *Root, deviceName string, permitted []string) (*Intermediate, bool, error) {
	existing, err := LoadIntermediate(root)
	if err == nil && !existing.NeedsRenewal() && constraintsMatch(existing.Cert, permitted) {
		return existing, false, nil
	}

	key, err := NewDeviceKey()
	if err != nil {
		return nil, false, fmt.Errorf("provision device: generate key: %w", err)
	}

	cert, err := root.SignIntermediate(IntermediateRequest{
		PublicKey:           &key.PublicKey,
		DeviceName:          deviceName,
		PermittedDNSDomains: permitted,
	})
	if err != nil {
		return nil, false, err
	}

	inter := &Intermediate{Cert: cert, Key: key, RootDER: root.Cert.Raw}
	if err := inter.Save(); err != nil {
		return nil, false, err
	}

	return inter, true, nil
}

// constraintsMatch reports whether an existing intermediate already permits
// exactly the domains asked for. When the allow-list changes, the intermediate has
// to be reissued or the new hosts cannot be captured at all — the client would
// reject our certificate for them, correctly.
func constraintsMatch(cert *x509.Certificate, permitted []string) bool {
	if len(cert.PermittedDNSDomains) != len(permitted) {
		return false
	}

	have := map[string]bool{}
	for _, d := range cert.PermittedDNSDomains {
		have[strings.ToLower(d)] = true
	}
	for _, d := range permitted {
		if !have[strings.ToLower(d)] {
			return false
		}
	}

	return true
}
