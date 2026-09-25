//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"github.com/pkisan/aiul/internal/platform"
)

// On Windows the two background processes are Windows services, started by the
// Service Control Manager (SCM) rather than launchd or systemd. A service must
// answer the SCM: say when it is running, accept "stop", and report when it has
// stopped. This file is that conversation; the work itself is the same
// `aiul helper` and `aiul run --manage-proxy` as on the other systems.

// runAsWindowsService runs the command line under the SCM and returns true when
// this process was started as a service. Run by hand, it returns false.
func runAsWindowsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService || len(os.Args) < 2 {
		return false
	}

	name, logName := platform.WorkerServiceName, "agent.err.log"
	if os.Args[1] == "helper" {
		name, logName = platform.HelperServiceName, "helper.err.log"
	}

	// A service has no console, so whatever it writes to stderr would be lost.
	// Point stderr at the log file, at the Windows handle level too, so even a
	// crash's stack trace lands there.
	if f, err := os.OpenFile(filepath.Join(platform.LogDir, logName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
		os.Stderr = f
	}

	if err := svc.Run(name, service{}); err != nil {
		fmt.Fprintf(os.Stderr, "aiul: could not run as the %s service: %v\n", name, err)
	}
	return true
}

type service struct{}

// Execute is called by the SCM. It runs the command in the background and
// relays stop requests to it as if it had received SIGTERM, so the command
// takes the same fail-open path as everywhere else: the system proxy goes first.
func (service) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	done := make(chan int, 1)
	exitProcess = func(code int) {
		done <- code
		select {} // Execute returns and the process ends; this goroutine never resumes
	}
	go func() { done <- dispatch(os.Args[1:]) }()

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case code := <-done:
			// A non-zero code counts as a failure, which the service's recovery
			// settings answer by restarting it.
			return false, uint32(code)
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				raiseStop()
			}
		}
	}
}
