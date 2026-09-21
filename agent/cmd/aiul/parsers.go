package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkisan/aiul/internal/parsers"
	"github.com/pkisan/aiul/internal/proxy"
)

const parsersUsage = `Usage:
  aiul parsers

Lists what this build can decrypt and what it can read.

Two different things, and the difference matters:
  DECRYPTED  the host is on the allow-list, so we see inside the connection
  PARSED     an endpoint on that host is understood, so a conversation is stored

A host that is decrypted but not parsed is recorded as metadata only: when, how
big, how long — never what was said.
`

// cmdParsers answers "is this tool covered?" from the binary rather than from
// somebody's memory of a documentation table.
func cmdParsers(args []string) int {
	if contains(args, "-h") || contains(args, "--help") {
		fmt.Print(parsersUsage)
		return 0
	}

	fmt.Printf("aiul %s — allow-list version %d\n\n", version, proxy.AllowListVersion)

	fmt.Println("Parsers in this build")
	fmt.Println("---------------------")
	for _, name := range parsers.Names() {
		fmt.Printf("  %s\n", name)
	}

	fmt.Println()
	fmt.Println("Hosts decrypted, and whether a conversation endpoint is known")
	fmt.Println("-------------------------------------------------------------")

	// A path that is representative of a conversation on each host, so the table
	// can say more than "the host is listed".
	probes := map[string]string{
		"api.openai.com":                    "/v1/chat/completions",
		"chatgpt.com":                       "/backend-api/f/conversation",
		"chat.openai.com":                   "/backend-api/f/conversation",
		"api.anthropic.com":                 "/v1/messages",
		"claude.ai":                         "/api/organizations/o/chat_conversations/c/completion",
		"generativelanguage.googleapis.com": "/v1beta/models/gemini-2.5-pro:streamGenerateContent",
		"api.githubcopilot.com":             "/chat/completions",
		"api.individual.githubcopilot.com":  "/chat/completions",
		"api.business.githubcopilot.com":    "/chat/completions",
		"api.enterprise.githubcopilot.com":  "/chat/completions",
		"api.mistral.ai":                    "/v1/chat/completions",
		"api.groq.com":                      "/v1/chat/completions",
		"api.deepseek.com":                  "/v1/chat/completions",
		"api.x.ai":                          "/v1/chat/completions",
		"api.cohere.com":                    "/v1/chat",
		"api.perplexity.ai":                 "/chat/completions",
		"openrouter.ai":                     "/api/v1/chat/completions",
		"api.together.xyz":                  "/v1/chat/completions",
		"api2.cursor.sh":                    "/aiserver.v1.ChatService/StreamUnifiedChat",
		"api2direct.cursor.sh":              "/aiserver.v1.ChatService/StreamUnifiedChat",
		"api3.cursor.sh":                    "/tev1/v1/rgstr",
		"api.origin.cursor.com":             "/aiserver.v1.ChatService/StreamUnifiedChat",
	}

	seen := map[string]bool{}
	for _, host := range proxy.AllowListEntries() {
		// "claude.ai" and "*.claude.ai" are one host as far as this table is
		// concerned.
		clean := strings.TrimPrefix(host, "*.")
		if seen[clean] {
			continue
		}
		seen[clean] = true

		name := "— metadata only"
		if probe, ok := probes[clean]; ok {
			if p := parsers.For(clean, probe); p != nil {
				name = p.Name()
			}
		}

		fmt.Printf("  %-36s %s\n", clean, name)
	}

	fmt.Println()
	fmt.Println("Known limits:")
	fmt.Println("  Cursor pins its certificate. api2.cursor.sh rejects ours and is tunneled,")
	fmt.Println("  so its conversations are countable and never readable. See docs/ROADMAP.md.")

	if len(args) > 0 && args[0] != "" {
		fmt.Fprintf(os.Stderr, "\naiul parsers takes no arguments\n")
		return 2
	}

	return 0
}
