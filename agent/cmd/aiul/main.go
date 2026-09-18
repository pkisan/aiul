// Command aiul is the single binary for the AI Usage Logger: it will hold both
// the TLS-inspecting proxy and the endpoint agent. Phase 0 ships only `version`.
package main

import (
	"fmt"
	"os"
	"runtime"
)

// version is the build version. Later phases will stamp this at build time with
// -ldflags; a constant is enough for now.
const version = "0.0.1-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("aiul %s (%s/%s, %s)\n", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
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
`)
}
