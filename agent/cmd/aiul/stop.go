package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// How a long-running command learns it should stop, and how it exits.
//
// On macOS and Linux a signal does both jobs. A Windows service is told to stop
// by the Service Control Manager instead, and must report "stopped" back to it
// rather than simply vanish, or Windows records a crash and restarts it. So the
// signal handling and the final exit go through these two hooks, which the
// Windows service wrapper (service_windows.go) plugs into.

var (
	stopMu        sync.Mutex
	stopListeners []chan<- os.Signal
)

// notifyStop delivers Ctrl-C, SIGTERM and a service stop request to ch.
func notifyStop(ch chan os.Signal) {
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	stopMu.Lock()
	defer stopMu.Unlock()
	stopListeners = append(stopListeners, ch)
}

// raiseStop tells every listener to stop, as if it had received SIGTERM.
func raiseStop() {
	stopMu.Lock()
	defer stopMu.Unlock()
	for _, ch := range stopListeners {
		select {
		case ch <- syscall.SIGTERM:
		default: // already told
		}
	}
}

// exitProcess ends the process. The Windows service wrapper replaces it so the
// exit code reaches the Service Control Manager first.
var exitProcess = os.Exit
