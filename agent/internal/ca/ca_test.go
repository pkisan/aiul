package ca

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testRoot makes a root in memory. None of these tests touch the real CA on disk.
func testRoot(t *testing.T) *Root {
	t.Helper()
	root, err := generateRoot()
	if err != nil {
		t.Fatalf("generateRoot: %v", err)
	}
	return root
}

func TestRootIsAUsableCA(t *testing.T) {
	root := testRoot(t)

	if !root.Cert.IsCA {
		t.Error("root certificate is not marked as a CA")
	}
	if root.Cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("root certificate may not sign certificates")
	}
	if !strings.HasPrefix(root.Cert.Subject.CommonName, CommonNamePrefix) {
		t.Errorf("common name %q does not start with %q; killswitch.sh finds the certificate by this prefix",
			root.Cert.Subject.CommonName, CommonNamePrefix)
	}
	// Exactly one CA level below the root: the device intermediate of D3, which
	// may then sign only leaves. This was MaxPathLenZero until the intermediate
	// existed; a root with a path length of 0 cannot issue one at all, and every
	// verifier rejects the chain.
	if root.Cert.MaxPathLen != 1 || root.Cert.MaxPathLenZero {
		t.Errorf("root path length is %d (zero=%v), want 1: one intermediate below the root and no more",
			root.Cert.MaxPathLen, root.Cert.MaxPathLenZero)
	}
	if got := root.Cert.NotAfter.Sub(root.Cert.NotBefore); got < 300*24*time.Hour {
		t.Errorf("root validity %v is suspiciously short", got)
	}
	if !strings.Contains(string(root.CertPEM), "BEGIN CERTIFICATE") {
		t.Error("CertPEM is not PEM encoded")
	}
}

func TestMintLeafProducesAVerifiableCertificate(t *testing.T) {
	root := testRoot(t)

	cert, err := root.MintLeaf(LeafRequest{Hosts: []string{"api.openai.com", "openai.com", "127.0.0.1"}})
	if err != nil {
		t.Fatalf("MintLeaf: %v", err)
	}

	leaf := cert.Leaf
	if leaf.IsCA {
		t.Error("leaf must not be a CA")
	}
	if len(leaf.DNSNames) != 2 || leaf.DNSNames[0] != "api.openai.com" {
		t.Errorf("unexpected DNS names: %v", leaf.DNSNames)
	}
	if len(leaf.IPAddresses) != 1 || !leaf.IPAddresses[0].Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("unexpected IP addresses: %v", leaf.IPAddresses)
	}
	if len(cert.Certificate) != 2 {
		t.Errorf("served chain should be leaf + root, got %d certificates", len(cert.Certificate))
	}

	// The real check: a verifier that trusts our root accepts this leaf for that
	// hostname — and rejects it for a hostname it was not issued for.
	pool := x509.NewCertPool()
	pool.AddCert(root.Cert)

	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: "api.openai.com", Roots: pool}); err != nil {
		t.Errorf("leaf should verify for api.openai.com: %v", err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: "example.com", Roots: pool}); err == nil {
		t.Error("leaf must NOT verify for a hostname it was not minted for")
	}

	// And an untrusted verifier must reject it, or our CA would be meaningless.
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: "api.openai.com", Roots: x509.NewCertPool()}); err == nil {
		t.Error("leaf must not verify against an empty root pool")
	}
}

func TestMintLeafValidity(t *testing.T) {
	root := testRoot(t)
	cert, err := root.MintLeaf(LeafRequest{Hosts: []string{"localhost"}})
	if err != nil {
		t.Fatalf("MintLeaf: %v", err)
	}
	if got := time.Until(cert.Leaf.NotAfter); got > 48*time.Hour {
		t.Errorf("leaf lifetime %v is longer than intended; leaves must be short-lived", got)
	}
	if !cert.Leaf.NotBefore.Before(time.Now()) {
		t.Error("leaf should already be valid (backdated for clock skew)")
	}
}

func TestMintLeafRejectsEmptyHosts(t *testing.T) {
	root := testRoot(t)
	if _, err := root.MintLeaf(LeafRequest{}); err == nil {
		t.Error("minting with no hosts must fail")
	}
}

func TestEachLeafGetsItsOwnKeyAndSerial(t *testing.T) {
	root := testRoot(t)
	a, err := root.MintLeaf(LeafRequest{Hosts: []string{"a.example"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := root.MintLeaf(LeafRequest{Hosts: []string{"b.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if a.Leaf.SerialNumber.Cmp(b.Leaf.SerialNumber) == 0 {
		t.Error("two leaves share a serial number")
	}
	if string(a.Leaf.SubjectKeyId) == string(b.Leaf.SubjectKeyId) {
		t.Error("two leaves share a public key")
	}
}

// TestRealTLSHandshake is the end-to-end proof: a Go HTTPS server using a leaf we
// minted, and a Go client that trusts only our root, complete a real handshake.
func TestRealTLSHandshake(t *testing.T) {
	root := testRoot(t)
	cert, err := root.MintLeaf(LeafRequest{Hosts: []string{"localhost", "127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello from a certificate we minted"))
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}}
	srv.StartTLS()
	defer srv.Close()

	pool := x509.NewCertPool()
	pool.AddCert(root.Cert)
	// Note: no InsecureSkipVerify anywhere. Rule 5.
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("handshake with our own leaf failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status %d", resp.StatusCode)
	}

	// A client that does not trust our root must refuse the same server.
	strict := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: x509.NewCertPool()}}}
	if _, err := strict.Get(srv.URL); err == nil {
		t.Error("a client with no trusted roots must reject our certificate")
	}
}

func TestFingerprintFormat(t *testing.T) {
	root := testRoot(t)
	fp := Fingerprint(root.Cert)
	if len(fp) != 32*3-1 { // 32 bytes as "AA BB CC ..."
		t.Errorf("unexpected fingerprint format: %q", fp)
	}
}
