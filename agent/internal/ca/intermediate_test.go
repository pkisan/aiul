package ca

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"os"
	"strings"
	"testing"
	"time"
)

// The allow-list shape these tests use. Kept short and local: the real list lives
// in internal/proxy, and these tests are about the constraint mechanism, not about
// which hosts are on it.
var testPermitted = []string{"api.openai.com", "api.anthropic.com", "claude.ai"}

func testIntermediate(t *testing.T, root *Root, permitted []string) *Intermediate {
	t.Helper()

	key, err := NewDeviceKey()
	if err != nil {
		t.Fatalf("device key: %v", err)
	}
	cert, err := root.SignIntermediate(IntermediateRequest{
		PublicKey:           &key.PublicKey,
		DeviceName:          "test-mac",
		PermittedDNSDomains: permitted,
	})
	if err != nil {
		t.Fatalf("sign intermediate: %v", err)
	}

	return &Intermediate{Cert: cert, Key: key, RootDER: root.Cert.Raw}
}

// verify checks a minted certificate the way a client would: against the root
// alone, with the served chain supplying anything in between.
func verify(t *testing.T, root *Root, served *tls.Certificate, host string) error {
	t.Helper()

	roots := x509.NewCertPool()
	roots.AddCert(root.Cert)

	intermediates := x509.NewCertPool()
	for _, der := range served.Certificate[1:] {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatalf("parse chain certificate: %v", err)
		}
		// The root is in the chain too; adding it as an intermediate is harmless.
		intermediates.AddCert(cert)
	}

	_, err := served.Leaf.Verify(x509.VerifyOptions{
		DNSName:       host,
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})

	return err
}

func TestALeafFromTheIntermediateVerifiesForAnAllowedHost(t *testing.T) {
	root := testRoot(t)
	inter := testIntermediate(t, root, testPermitted)

	served, err := inter.MintLeaf(LeafRequest{Hosts: []string{"api.openai.com"}})
	if err != nil {
		t.Fatalf("mint leaf: %v", err)
	}

	// leaf, intermediate, root — a client holding only the root can build a path.
	if len(served.Certificate) != 3 {
		t.Errorf("served chain has %d certificates, want 3 (leaf, intermediate, root)", len(served.Certificate))
	}

	if err := verify(t, root, served, "api.openai.com"); err != nil {
		t.Errorf("an allowed host must verify: %v", err)
	}
}

// THE test. This is the property that makes a stolen laptop harmless: the device's
// key can sign whatever it likes, and a client still refuses it for any name
// outside the allow-list. Enforced by the verifier, not by our code.
func TestTheIntermediateCannotBeUsedForAnythingButTheAllowList(t *testing.T) {
	root := testRoot(t)
	inter := testIntermediate(t, root, testPermitted)

	for _, host := range []string{
		"bank.example.com",
		"login.microsoftonline.com",
		// The near misses rule 3 cares about: a constraint must anchor to whole
		// labels, never match as a substring.
		"notopenai.com",
		"api.openai.com.evil.net",
	} {
		served, err := inter.MintLeaf(LeafRequest{Hosts: []string{host}})
		if err != nil {
			t.Fatalf("mint leaf for %s: %v", host, err)
		}

		err = verify(t, root, served, host)
		if err == nil {
			t.Errorf("SECURITY: a certificate for %s was accepted; the name constraints are not being enforced", host)

			continue
		}
		if !strings.Contains(err.Error(), "not permitted") && !strings.Contains(err.Error(), "authorized") {
			t.Errorf("%s was rejected, but for the wrong reason: %v", host, err)
		}
	}
}

// A subdomain of a permitted domain is allowed: "claude.ai" in the constraints
// covers "www.claude.ai", which is what the "*.claude.ai" allow-list entry means.
func TestASubdomainOfAPermittedDomainIsAllowed(t *testing.T) {
	root := testRoot(t)
	inter := testIntermediate(t, root, testPermitted)

	served, err := inter.MintLeaf(LeafRequest{Hosts: []string{"www.claude.ai"}})
	if err != nil {
		t.Fatalf("mint leaf: %v", err)
	}

	if err := verify(t, root, served, "www.claude.ai"); err != nil {
		t.Errorf("a subdomain of a permitted domain must verify: %v", err)
	}
}

func TestTheIntermediateCannotSignAnotherCA(t *testing.T) {
	root := testRoot(t)
	inter := testIntermediate(t, root, testPermitted)

	if !inter.Cert.MaxPathLenZero {
		t.Error("the intermediate must not be able to issue further CAs")
	}
	if !inter.Cert.IsCA {
		t.Error("the intermediate must be a CA; it has to sign leaves")
	}
	if !inter.Cert.PermittedDNSDomainsCritical {
		t.Error("the name constraints must be critical, so a client that cannot read them refuses the certificate")
	}
}

// An intermediate with no constraints is the unrestricted signing certificate this
// whole design exists to prevent, and an empty slice is an easy way to produce one.
func TestIssuingWithoutNameConstraintsIsRefused(t *testing.T) {
	root := testRoot(t)
	key, err := NewDeviceKey()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := root.SignIntermediate(IntermediateRequest{PublicKey: &key.PublicKey}); err == nil {
		t.Error("issuing an intermediate with no name constraints must be refused")
	}
}

func TestSigningNeedsOnlyThePublicKey(t *testing.T) {
	root := testRoot(t)
	key, err := NewDeviceKey()
	if err != nil {
		t.Fatal(err)
	}

	cert, err := root.SignIntermediate(IntermediateRequest{
		PublicKey:           &key.PublicKey,
		DeviceName:          "test-mac",
		PermittedDNSDomains: testPermitted,
	})
	if err != nil {
		t.Fatalf("sign intermediate: %v", err)
	}

	// The certificate carries the device's public key, and the request had no
	// field that could have carried the private one.
	issued, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("the issued certificate carries a %T, not an ECDSA key", cert.PublicKey)
	}
	if !issued.Equal(&key.PublicKey) {
		t.Error("the issued certificate does not carry the device's public key")
	}
}

func TestRenewalTiming(t *testing.T) {
	root := testRoot(t)

	fresh := testIntermediate(t, root, testPermitted)
	if fresh.NeedsRenewal() {
		t.Error("a certificate issued moments ago should not need renewal")
	}

	var missing *Intermediate
	if !missing.NeedsRenewal() {
		t.Error("no intermediate at all must count as needing renewal")
	}

	key, _ := NewDeviceKey()
	nearlyDone, err := root.SignIntermediate(IntermediateRequest{
		PublicKey:           &key.PublicKey,
		PermittedDNSDomains: testPermitted,
		NotBefore:           time.Now().Add(-6 * 24 * time.Hour),
		NotAfter:            time.Now().Add(24 * time.Hour), // inside RenewBefore
	})
	if err != nil {
		t.Fatal(err)
	}
	if !(&Intermediate{Cert: nearlyDone}).NeedsRenewal() {
		t.Errorf("a certificate with %v left must be renewed (threshold %v)",
			time.Until(nearlyDone.NotAfter).Round(time.Hour), RenewBefore)
	}
}

func TestPermittedDomainsFromStripsWildcardsAndDuplicates(t *testing.T) {
	got := PermittedDomainsFrom([]string{"claude.ai", "*.claude.ai", "API.OpenAI.com", " *.chatgpt.com "})

	want := []string{"claude.ai", "api.openai.com", "chatgpt.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)

			break
		}
	}
}

// A root that predates D3 has a path length of 0 and cannot issue an intermediate.
// Saying so is better than issuing a chain no client will accept.
func TestARootThatCannotIssueAnIntermediateSaysSo(t *testing.T) {
	root := testRoot(t)
	root.Cert.MaxPathLen = 0
	root.Cert.MaxPathLenZero = true

	key, _ := NewDeviceKey()
	_, err := root.SignIntermediate(IntermediateRequest{
		PublicKey:           &key.PublicKey,
		PermittedDNSDomains: testPermitted,
	})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "ca init --force") {
		t.Errorf("the error should say how to fix it, got: %v", err)
	}
}

// Provisioning writes the key where only its owner can read it, and reuses what is
// already there rather than rotating on every start.
func TestProvisionDeviceSavesAndReuses(t *testing.T) {
	t.Setenv("AIUL_STATE_DIR", t.TempDir())
	root := testRoot(t)

	first, issued, err := ProvisionDevice(root, "test-mac", testPermitted)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if !issued {
		t.Error("the first call must issue a certificate")
	}

	_, keyPath, _ := DevicePaths()
	if _, err := readKeyPEM(keyPath); err != nil {
		t.Errorf("the saved key is not readable back, or its permissions are wrong: %v", err)
	}

	second, issued, err := ProvisionDevice(root, "test-mac", testPermitted)
	if err != nil {
		t.Fatalf("provision again: %v", err)
	}
	if issued {
		t.Error("a valid intermediate must be reused, not reissued")
	}
	if !second.Cert.Equal(first.Cert) {
		t.Error("the second call returned a different certificate")
	}
}

// When the allow-list changes, the old intermediate permits the wrong set of names
// and every new host would be rejected by the client. It has to be reissued.
func TestProvisionDeviceReissuesWhenTheAllowListChanges(t *testing.T) {
	t.Setenv("AIUL_STATE_DIR", t.TempDir())
	root := testRoot(t)

	first, _, err := ProvisionDevice(root, "test-mac", testPermitted)
	if err != nil {
		t.Fatal(err)
	}

	grown := append([]string{"api.newprovider.ai"}, testPermitted...)
	second, issued, err := ProvisionDevice(root, "test-mac", grown)
	if err != nil {
		t.Fatal(err)
	}
	if !issued {
		t.Fatal("a changed allow-list must produce a new intermediate")
	}
	if second.Cert.Equal(first.Cert) {
		t.Error("the certificate did not change")
	}

	served, err := second.MintLeaf(LeafRequest{Hosts: []string{"api.newprovider.ai"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := verify(t, root, served, "api.newprovider.ai"); err != nil {
		t.Errorf("the newly allowed host must verify: %v", err)
	}
}

// An install replaced the root but kept the intermediate the OLD root signed. Only
// clients that still trusted the old root could verify what we served.
func TestProvisionDeviceReissuesWhenTheRootChanges(t *testing.T) {
	t.Setenv("AIUL_STATE_DIR", t.TempDir())

	if _, _, err := ProvisionDevice(testRoot(t), "test-mac", testPermitted); err != nil {
		t.Fatal(err)
	}

	newRoot := testRoot(t)
	inter, issued, err := ProvisionDevice(newRoot, "test-mac", testPermitted)
	if err != nil {
		t.Fatal(err)
	}
	if !issued {
		t.Fatal("an intermediate from another root must be reissued")
	}

	served, err := inter.MintLeaf(LeafRequest{Hosts: []string{"api.openai.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := verify(t, newRoot, served, "api.openai.com"); err != nil {
		t.Errorf("the chain must verify against the current root: %v", err)
	}
}

// The installer asked "is a CA present?" when the question it needed was "is a CA
// present that works?". A root from before the device chain has a path length of 0,
// so the worker cannot start, and an install fails with a good-looking CA in place.
func TestCanIssueIntermediate(t *testing.T) {
	root := testRoot(t)
	if !root.CanIssueIntermediate() {
		t.Error("a freshly created root must be able to issue this device's certificate")
	}

	root.Cert.MaxPathLen = 0
	root.Cert.MaxPathLenZero = true
	if root.CanIssueIntermediate() {
		t.Error("a root with a path length of 0 must report that it cannot issue one")
	}
}

// The certificate says which hosts this device may sign for, which anyone on the
// machine is entitled to check. The key beside it is nobody else's business.
func TestTheCertificateIsReadableAndTheKeyIsNot(t *testing.T) {
	t.Setenv("AIUL_STATE_DIR", t.TempDir())
	root := testRoot(t)

	if _, _, err := ProvisionDevice(root, "test-mac", testPermitted); err != nil {
		t.Fatal(err)
	}

	dir, _ := DeviceDir()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o055 == 0 {
		t.Errorf("the device directory is %#o; the certificate in it must be readable", info.Mode().Perm())
	}

	certPath, keyPath, _ := DevicePaths()
	if _, err := ReadIntermediateCert(certPath); err != nil {
		t.Errorf("the certificate must be readable: %v", err)
	}

	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if keyInfo.Mode().Perm()&0o077 != 0 {
		t.Errorf("the private key is %#o, must be 0600", keyInfo.Mode().Perm())
	}
}
