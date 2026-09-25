// Command aiul is the single binary for the AI Usage Logger: it will hold both
// the TLS-inspecting proxy and the endpoint agent. Phase 0 ships only `version`.
package main

import (
	"fmt"
	"os"
	"runtime"
)

// version is the build version. A var, not a const, because scripts/build.sh
// stamps it with -ldflags "-X main.version=..." so a package can be identified
// from the binary it installed. A plain `go build` leaves the development value.
var version = "0.0.1-dev"

func main() {
	// Started by Windows as a service: the service wrapper runs the command.
	if runAsWindowsService() {
		return
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	os.Exit(dispatch(os.Args[1:]))
}

// dispatch runs one command and returns its exit code.
func dispatch(args []string) int {
	switch args[0] {
	case "version":
		fmt.Printf("aiul %s (%s/%s, %s)\n", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return 0
	case "ca":
		return cmdCA(args[1:])
	case "proxy":
		return cmdProxy(args[1:])
	case "run":
		return cmdRun(args[1:])
	case "helper":
		return cmdHelper(args[1:])
	case "install":
		return cmdInstall(args[1:])
	case "uninstall":
		return cmdUninstall(args[1:])
	case "status":
		return cmdStatus(args[1:])
	case "doctor":
		return cmdDoctor(args[1:])
	case "parsers":
		return cmdParsers(args[1:])
	case "login":
		return cmdLogin(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "aiul: unknown command %q\n\n", args[0])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `aiul - AI Usage Logger

Usage:
  aiul version    print the version and build platform
  aiul ca         manage the development certificate authority
                  (init, info, trust, untrust, demo-server)
  aiul parsers    what this build can decrypt, and what it can read
  aiul proxy      run the TLS-inspecting proxy on 127.0.0.1:8899
  aiul run        run the proxy and the agent loop together (used by launchd)
  aiul helper     the small root-only half: system proxy and process lookup

  aiul login      link this device to your account (prints a code to enter)

  aiul install    configure this Mac (DRY RUN unless you pass --apply)
  aiul uninstall  remove every change aiul made
  aiul status     a few lines: what is on right now
  aiul doctor     explain in plain English what is and is not configured
`)
}
