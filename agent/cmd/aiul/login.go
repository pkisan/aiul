package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pkisan/aiul/internal/platform"
)

const loginUsage = `Usage:
  aiul login

Links this device to your account, so what is captured here is attributed to
you. It prints a short code; sign in to the web app, accept the notice on your
first sign-in, open "Devices" and type the code. This command waits until you
have, then says who the device now belongs to.

Until a device is linked to someone who accepted the notice, the backend
discards everything it sends.

Reads the backend address and device token the same way 'aiul run' does, so on
an installed Mac it usually needs sudo to read /etc/aiul/agent.conf.
`

// pairStatus is the backend's answer to "who does this device belong to?".
type pairStatus struct {
	Paired    bool   `json:"paired"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Consented bool   `json:"consented"`
}

// pairURL turns the events endpoint (".../api/aiul/events") into the pairing
// one (".../api/aiul/pair"). Both live under the same prefix on the backend.
func pairURL(endpoint string) (string, error) {
	base, found := strings.CutSuffix(strings.TrimRight(endpoint, "/"), "/events")
	if !found {
		return "", fmt.Errorf("the endpoint %q does not end in /events", endpoint)
	}
	return base + "/pair", nil
}

func cmdLogin(args []string) int {
	if contains(args, "-h") || contains(args, "--help") {
		fmt.Print(loginUsage)
		return 0
	}

	// Same precedence as `aiul run`, minus the flags: environment, config file,
	// keychain.
	config := readAgentConfig(agentConfigPath)
	endpoint := firstSet(os.Getenv("AIUL_ENDPOINT"), config["AIUL_ENDPOINT"])
	token := firstSet(os.Getenv("AIUL_DEVICE_TOKEN"), config["AIUL_DEVICE_TOKEN"])
	if token == "" {
		token, _ = platform.DeviceToken()
	}
	if endpoint == "" || token == "" {
		fmt.Fprintf(os.Stderr, "aiul login: no backend address or device token.\n"+
			"Set AIUL_ENDPOINT and AIUL_DEVICE_TOKEN, or run with sudo so %s can be read.\n", agentConfigPath)
		return 1
	}

	url, err := pairURL(endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aiul login: %v\n", err)
		return 1
	}

	client := &http.Client{Timeout: 15 * time.Second}

	var start struct {
		Code      string `json:"code"`
		URL       string `json:"url"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := callJSON(client, http.MethodPost, url, token, &start); err != nil {
		fmt.Fprintf(os.Stderr, "aiul login: %v\n", err)
		return 1
	}

	fmt.Println("To link this device to your account:")
	fmt.Println()
	fmt.Printf("  1. Open   %s\n", start.URL)
	fmt.Println("  2. Sign in (and accept the notice, the first time)")
	fmt.Printf("  3. Enter  %s\n", start.Code)
	fmt.Println()
	fmt.Println("Waiting... (the code is valid for 10 minutes; Ctrl-C to stop)")

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)

		var status pairStatus
		if err := callJSON(client, http.MethodGet, url, token, &status); err != nil {
			// A blip while polling is not worth giving up over.
			continue
		}
		if !status.Paired {
			continue
		}

		fmt.Printf("\nThis device is linked to %s <%s>.\n", status.Name, status.Email)
		if !status.Consented {
			fmt.Println("They have not accepted the current notice yet, so nothing is recorded until they do.")
		}
		return 0
	}

	fmt.Fprintln(os.Stderr, "\naiul login: the code expired. Run aiul login again for a new one.")
	return 1
}

// callJSON makes one authenticated request to the backend and decodes the reply.
func callJSON(client *http.Client, method, url, token string, out any) error {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return errors.New("the backend does not know this device's token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the backend answered %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
