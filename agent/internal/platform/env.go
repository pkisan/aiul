package platform

import "strings"

// Markers delimiting the block this agent owns in a shell startup file. Only the
// text between them is ever written or removed, so anything else in the file is
// left exactly as it was.
const (
	blockBegin = "# >>> AIUL BEGIN >>>"
	blockEnd   = "# <<< AIUL END <<<"
)

// DefaultEnvVars is the set of variables the agent writes. The sources are the
// capture matrix in AI-Usage-Logger-Build-Map.md and what each runtime actually
// reads.
//
// caCertPath is the PEM file of our root CA; proxyURL is "http://127.0.0.1:8899".
func DefaultEnvVars(proxyURL, caCertPath string) EnvVars {
	return EnvVars{
		// Route through the proxy. Both spellings: tools disagree about case.
		"HTTPS_PROXY": proxyURL,
		"https_proxy": proxyURL,

		// Never proxy local traffic. A broken proxy must not take local
		// development down with it.
		"NO_PROXY": strings.Join(NoProxyList, ","),
		"no_proxy": strings.Join(NoProxyList, ","),

		// Node and Bun runtimes do not read the macOS keychain, which is the whole
		// reason macOS needs these variables at all.
		"NODE_EXTRA_CA_CERTS": caCertPath,
		"NODE_USE_SYSTEM_CA":  "1",

		// Rust (Codex) and most Python tooling.
		"SSL_CERT_FILE":        caCertPath,
		"REQUESTS_CA_BUNDLE":   caCertPath,
		"CODEX_CA_CERTIFICATE": caCertPath,
	}
}

// NoProxyList is everything that must reach the network directly.
var NoProxyList = []string{
	"localhost", "127.0.0.1", "::1", "0.0.0.0",
	"*.local", "*.test",
	"169.254.0.0/16",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
}

// parseBlockVars reads the "export K=\"V\"" lines inside our marked block.
func parseBlockVars(content string) EnvVars {
	out := EnvVars{}
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.Contains(line, blockBegin):
			inBlock = true
		case strings.Contains(line, blockEnd):
			inBlock = false
		case inBlock:
			if after, ok := strings.CutPrefix(strings.TrimSpace(line), "export "); ok {
				k, v, found := strings.Cut(after, "=")
				if found {
					out[k] = strings.Trim(v, `"`)
				}
			}
		}
	}
	return out
}
