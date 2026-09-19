package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/pkisan/aiul/internal/ca"
	"github.com/pkisan/aiul/internal/forward"
	"github.com/pkisan/aiul/internal/helper"
	"github.com/pkisan/aiul/internal/paths"
	"github.com/pkisan/aiul/internal/platform"
)

// doctor explains, in plain English, what is and is not configured on this Mac.
// It is the command to run when something seems wrong, and it changes nothing.

type check struct {
	name   string
	ok     bool
	detail string
	fix    string
}

func cmdDoctor(args []string) int {
	checks := runChecks()

	fmt.Println("aiul doctor")
	fmt.Println("===========")
	fmt.Println()

	problems := 0
	for _, c := range checks {
		mark := "ok  "
		if !c.ok {
			mark = "NO  "
			problems++
		}
		fmt.Printf("%s %s\n", mark, c.name)
		if c.detail != "" {
			// Continuation lines line up under the first one.
			fmt.Printf("       %s\n", strings.ReplaceAll(strings.TrimSpace(c.detail), "\n", "\n       "))
		}
		if !c.ok && c.fix != "" {
			fmt.Printf("       fix: %s\n", c.fix)
		}
	}

	fmt.Println()
	if problems == 0 {
		fmt.Println("Everything checks out.")
	} else {
		fmt.Printf("%d thing(s) need attention. Nothing here changes your Mac;\n", problems)
		fmt.Println("run the suggested command yourself when you are ready.")
	}
	fmt.Println()
	fmt.Println("To undo everything at any time: sudo ./scripts/killswitch.sh")
	return 0
}

func runChecks() []check {
	var out []check

	// --- the CA -------------------------------------------------------------
	root, caErr := ca.Load()
	if caErr != nil {
		out = append(out, check{
			name:   "development CA exists",
			detail: caErr.Error(),
			fix:    "aiul ca init",
		})
	} else {
		expires := time.Until(root.Cert.NotAfter)
		out = append(out, check{
			name: "development CA exists",
			ok:   true,
			detail: fmt.Sprintf("%s, expires in %d days",
				root.Cert.Subject.CommonName, int(expires.Hours()/24)),
		})

		trusted, _ := platform.Trust().IsTrusted(ca.CommonNamePrefix)
		out = append(out, check{
			name:   "CA is trusted by macOS",
			ok:     trusted,
			detail: trustDetail(trusted),
			fix:    "aiul ca trust",
		})
	}

	// --- the proxy ----------------------------------------------------------
	listening := proxyIsListening()
	out = append(out, check{
		name:   "proxy is listening on " + proxyAddr,
		ok:     listening,
		detail: listeningDetail(listening),
		fix:    "aiul proxy   (or: sudo launchctl load -w /Library/LaunchDaemons/com.aiul.agent.plist)",
	})

	current, err := platform.Proxy().Current()
	if err == nil {
		var pointing, notPointing []string
		for service, value := range current {
			if value == proxyAddr {
				pointing = append(pointing, service)
			} else if value == "" {
				notPointing = append(notPointing, service)
			} else {
				notPointing = append(notPointing, fmt.Sprintf("%s (points at %s)", service, value))
			}
		}
		out = append(out, check{
			name:   "system proxy points at us",
			ok:     len(pointing) > 0,
			detail: proxyDetail(pointing, notPointing),
			fix:    "sudo aiul install --apply",
		})
	}

	// --- environment variables ----------------------------------------------
	vars, err := platform.Env().Current()
	if err == nil {
		out = append(out, check{
			name:   "environment variables are set for terminals",
			ok:     len(vars) > 0,
			detail: envDetail(vars),
			fix:    "sudo aiul install --apply",
		})
	}

	// --- the two background jobs ---------------------------------------------
	running, _ := platform.Service().Running()
	out = append(out, check{
		name:   "background jobs are loaded",
		ok:     running,
		detail: jobDetail(running),
		fix:    "sudo aiul install --apply",
	})

	// --- the privilege split --------------------------------------------------
	helperUp := helper.NewClient(helper.SocketPath).Available()
	uid, _ := platform.ServiceAccount()
	out = append(out, check{
		name:   "traffic is parsed WITHOUT root",
		ok:     helperUp && uid >= 0,
		detail: privilegeDetail(helperUp, uid),
		fix:    "sudo aiul install --apply   (running by hand is fine; this only applies to the installed agent)",
	})

	// --- MDM -----------------------------------------------------------------
	enrolled, detail, _ := platform.MDM().Enrolled()
	out = append(out, check{
		name:   "device is managed (MDM)",
		ok:     enrolled,
		detail: detail,
		fix:    "this agent only runs on a managed device; for development set " + platform.DevAllowUnmanagedVar + "=1",
	})

	// --- the spool -----------------------------------------------------------
	if dir, err := forward.DefaultDir(); err == nil {
		spool, err := forward.NewSpool(dir)
		if err == nil {
			pending, _ := spool.Pending()
			out = append(out, check{
				name:   "events are being captured",
				ok:     true,
				detail: fmt.Sprintf("%d event(s) waiting in %s", len(pending), dir),
			})
		}
	}

	// --- the backend ---------------------------------------------------------
	token, _ := platform.DeviceToken()
	out = append(out, check{
		name:   "device token for the backend",
		ok:     token != "",
		detail: tokenDetail(token),
		fix:    "not needed yet: the backend arrives in Phase 6. Events wait in the spool until then.",
	})

	// --- the tools -----------------------------------------------------------
	tools := platform.Tools().Detect()
	out = append(out, check{
		name:   "AI tools found on this Mac",
		ok:     len(tools) > 0,
		detail: toolsDetail(tools),
		fix:    "install one of: claude, codex, gemini, opencode, Cursor, VS Code",
	})

	return out
}

func proxyIsListening() bool {
	conn, err := net.DialTimeout("tcp", proxyAddr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func trustDetail(trusted bool) string {
	if trusted {
		return "in the System keychain; software on this Mac accepts certificates we mint"
	}
	return "not in the System keychain, so tools will reject our certificates and be tunneled instead"
}

func listeningDetail(listening bool) string {
	if listening {
		return "accepting connections"
	}
	return "nothing is listening, so no traffic can be captured"
}

func proxyDetail(pointing, notPointing []string) string {
	var b strings.Builder
	if len(pointing) > 0 {
		fmt.Fprintf(&b, "pointing at us: %s", strings.Join(pointing, ", "))
	}
	if len(notPointing) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "not pointing at us: %s", strings.Join(notPointing, ", "))
	}
	if b.Len() == 0 {
		return "no network services found"
	}
	return b.String()
}

func envDetail(vars platform.EnvVars) string {
	if len(vars) == 0 {
		return "no AIUL block in /etc/zshenv"
	}
	return fmt.Sprintf("%d variable(s) in the AIUL block in /etc/zshenv: %s",
		len(vars), strings.Join(sortedEnvKeys(vars), ", "))
}

func jobDetail(running bool) string {
	if running {
		return "com.aiul.helper (root) and com.aiul.agent (unprivileged) are both loaded"
	}
	return "not loaded; the proxy will not start at boot"
}

// privilegeDetail explains the split in plain English, because "is the helper
// running" is not a question anyone should have to translate.
func privilegeDetail(helperUp bool, uid int) string {
	switch {
	case helperUp && uid >= 0:
		return fmt.Sprintf("the worker runs as %s (uid %d) and asks a small root helper for the "+
			"three things that need privileges", platform.ServiceUserName, uid)
	case uid < 0:
		return "the " + platform.ServiceUserName + " service account does not exist, so nothing is installed yet"
	default:
		return "the root helper is not answering, so the installed worker cannot set the system proxy " +
			"or identify which process made a request"
	}
}

func tokenDetail(token string) string {
	if token == "" {
		return "none stored in the keychain, so events stay in the spool"
	}
	return fmt.Sprintf("stored in the keychain (%d characters)", len(token))
}

func toolsDetail(tools []platform.Tool) string {
	if len(tools) == 0 {
		return "none found"
	}
	var lines []string
	for _, t := range tools {
		lines = append(lines, fmt.Sprintf("%-12s %s", t.Name, t.Path))
	}
	return strings.Join(lines, "\n")
}

// cmdStatus is the short version of doctor: what is on, in a few lines.
func cmdStatus(args []string) int {
	certPath, _, _ := ca.Paths()
	trusted, _ := platform.Trust().IsTrusted(ca.CommonNamePrefix)
	running, _ := platform.Service().Running()
	current, _ := platform.Proxy().Current()
	vars, _ := platform.Env().Current()

	proxied := 0
	for _, v := range current {
		if v == proxyAddr {
			proxied++
		}
	}

	fmt.Printf("proxy listening   %s\n", yesNo(proxyIsListening()))
	fmt.Printf("background job    %s\n", yesNo(running))
	fmt.Printf("CA trusted        %s\n", yesNo(trusted))
	fmt.Printf("system proxy      %d of %d network services point at %s\n", proxied, len(current), proxyAddr)
	fmt.Printf("env vars written  %d\n", len(vars))
	fmt.Printf("CA file           %s\n", certPath)

	// When the agent is installed, the spool that matters is the worker's, not
	// this user's: the worker runs as the service account and writes under
	// /var/db/aiul. Reporting our own empty directory in that case says "nothing
	// captured" when the truth is "we are looking in the wrong place".
	dir, err := forward.DefaultDir()
	if running {
		dir, err = paths.SpoolDirIn(paths.SystemStateDir), nil
	}
	if err == nil {
		if spool, spoolErr := forward.NewSpool(dir); spoolErr == nil {
			pending, pendingErr := spool.Pending()
			if pendingErr != nil {
				// Owned by the service account and not readable by us. Say so,
				// rather than printing a zero that looks like an answer.
				fmt.Printf("events waiting    %s (run as root to count them)\n", dir)
			} else {
				fmt.Printf("events waiting    %d in %s\n", len(pending), dir)
			}
		} else {
			fmt.Printf("events waiting    %s (run as root to count them)\n", dir)
		}
	}

	if proxied > 0 || trusted || running || len(vars) > 0 {
		fmt.Println("\nundo everything   sudo ./scripts/killswitch.sh")
	} else {
		fmt.Println("\nThis Mac has no aiul settings applied.")
	}
	return 0
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
