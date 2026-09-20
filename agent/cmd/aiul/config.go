package main

import (
	"bufio"
	"os"
	"strings"
)

// Where the installed agent reads its settings from.
//
// launchd does not inherit the shell that installed the agent, and `installer`
// does not pass its environment to package scripts, so neither the endpoint nor
// the device token can be handed over on a command line. Both have to be written
// somewhere the agent itself reads — and a file beats baking them into the job
// definition, because changing the backend address then needs no reinstall.
//
// The format is deliberately dull: KEY=VALUE, one per line, # for comments.
//
//	# /etc/aiul/agent.conf
//	AIUL_ENDPOINT=https://aiul.example.com/api/aiul/events
//	AIUL_DEVICE_TOKEN=aiul_...
//
// An MDM writes this file when it deploys the package. It holds a credential, so
// `aiul install` checks its permissions and complains if anyone but root can read
// it.
const agentConfigPath = "/etc/aiul/agent.conf"

// readAgentConfig returns the settings in the config file, or an empty map when
// there is none. A missing file is the normal case when running by hand and is
// never an error.
func readAgentConfig(path string) map[string]string {
	out := map[string]string{}

	file, err := os.Open(path)
	if err != nil {
		return out
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// Tolerate quotes: someone copying a token out of a password manager will
		// paste them sooner or later.
		value = strings.Trim(value, `"'`)

		if key != "" && value != "" {
			out[key] = value
		}
	}

	return out
}

// firstSet returns the first value that is not empty.
func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}

	return ""
}
