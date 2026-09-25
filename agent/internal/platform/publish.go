package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// PublishCA copies the public half of the CA where every user can read it.
//
// Only certificates: the private key stays where it is, owned by the service
// account. A certificate is public by definition — it is what we ask the machine
// to trust — and the environment variables the agent writes are useless if the
// file they name cannot be opened.
func PublishCA(certPath, bundlePath string) error {
	if err := os.MkdirAll(PublicCADir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", PublicCADir, err)
	}

	for _, from := range []string{certPath, bundlePath} {
		if from == "" {
			continue
		}

		data, err := os.ReadFile(from)
		if err != nil {
			return fmt.Errorf("read %s: %w", from, err)
		}

		to := filepath.Join(PublicCADir, filepath.Base(from))
		if err := os.WriteFile(to, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", to, err)
		}
	}

	return nil
}
