package ca

import (
	"crypto/x509"
	"os"
	"strings"
	"testing"
)

// The bundle must contain the system roots AND ours. A bundle with only our root
// would stop curl verifying any ordinary website.
func TestBundleContainsSystemRootsAndOurs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := Init(false)
	if err != nil {
		t.Fatal(err)
	}

	path, err := root.WriteBundle()
	if err != nil {
		t.Skipf("no system CA bundle on this machine: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		t.Fatal("the bundle holds no usable certificates")
	}

	// Our root must be in there.
	if !strings.Contains(string(data), string(root.CertPEM)) {
		t.Error("our root is missing from the bundle")
	}

	// And so must a great many others — the system store has over a hundred.
	if got := strings.Count(string(data), "BEGIN CERTIFICATE"); got < 50 {
		t.Errorf("the bundle holds %d certificates; it should hold the whole system store plus ours", got)
	}
}

func TestBundleIsReadableByEveryone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, _ := Init(false)
	path, err := root.WriteBundle()
	if err != nil {
		t.Skip("no system CA bundle on this machine")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Every tool on the machine reads this, and it is all public certificates.
	if info.Mode().Perm()&0o044 == 0 {
		t.Errorf("mode %#o; the bundle must be readable by other users", info.Mode().Perm())
	}
}
