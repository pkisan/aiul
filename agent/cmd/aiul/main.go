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
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("aiul %s (%s/%s, %s)\n", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
	case "ca":
		os.Exit(cmdCA(os.Args[2:]))
	case "proxy":
		os.Exit(cmdProxy(os.Args[2:]))
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
	case "helper":
		os.Exit(cmdHelper(os.Args[2:]))
	case "install":
		os.Exit(cmdInstall(os.Args[2:]))
	case "uninstall":
		os.Exit(cmdUninstall(os.Args[2:]))
	case "status":
		os.Exit(cmdStatus(os.Args[2:]))
	case "doctor":
		os.Exit(cmdDoctor(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "aiul: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `aiul - AI Usage Logger

Usage:
  aiul version    print the version and build platform
  aiul ca         manage the development certificate authority
                  (init, info, trust, untrust, demo-server)
  aiul proxy      run the TLS-inspecting proxy on 127.0.0.1:8899
  aiul run        run the proxy and the agent loop together (used by launchd)
  aiul helper     the small root-only half: system proxy and process lookup

  aiul install    configure this Mac (DRY RUN unless you pass --apply)
  aiul uninstall  remove every change aiul made
  aiul status     a few lines: what is on right now
  aiul doctor     explain in plain English what is and is not configured
`)
}
