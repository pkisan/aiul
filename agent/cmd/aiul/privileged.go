package main

import (
	"log/slog"

	"github.com/pkisan/aiul/internal/helper"
	"github.com/pkisan/aiul/internal/platform"
	"github.com/pkisan/aiul/internal/tasks"
)

// The worker needs three privileged things. Where they come from depends on how
// it was started:
//
//   - installed: a root helper is listening, and the worker itself is
//     unprivileged. It asks the helper.
//   - run by hand in development: no helper. It does what it can directly, which
//     is everything except changing the system proxy — and that is fine, because
//     running by hand does not touch system settings anyway.
//
// This file is the one place that decides between the two.

// privilegedSource is what the worker uses for the three operations.
type privilegedSource interface {
	ProxyOn() error
	ProxyOff() error

	// ProcessOnPort also answers what is checked out where that process is
	// working: the worker's account cannot read a person's .git/HEAD, so the
	// privileged side reads it in the same reply.
	ProcessOnPort(port int) (pid int, name, workingDir, repo, branch, executable string, err error)
}

// chooseSource prefers the helper and says plainly which one it picked, because
// "task tagging is not working" usually means "there is no helper and the worker
// cannot see other users' processes".
func chooseSource(log *slog.Logger) privilegedSource {
	client := helper.NewClient(helper.SocketPath)

	if client.Available() {
		log.Info("using the privileged helper", "socket", helper.SocketPath)

		return client
	}

	log.Info("no privileged helper; doing what this process can itself",
		"socket", helper.SocketPath,
		"note", "expected when running by hand; task tagging sees only this user's processes")

	return directOps{}
}

// directOps performs the operations in this process. Only used when no helper is
// running, which is the by-hand development case.
type directOps struct{}

func (directOps) ProxyOn() error  { return platform.Proxy().Set(proxyAddr) }
func (directOps) ProxyOff() error { return platform.Proxy().Unset() }

func (directOps) ProcessOnPort(port int) (int, string, string, string, string, string, error) {
	process, err := platform.Processes().ByLocalPort(port)
	if err != nil {
		return 0, "", "", "", "", "", err
	}

	dir, err := platform.Processes().WorkingDir(process.PID)
	if err != nil {
		return process.PID, process.Name, "", "", "", process.Path, nil
	}

	// Running by hand, this process is the person using the machine, so it can
	// read the checkout itself.
	repo, branch := tasks.CheckoutAt(dir)

	return process.PID, process.Name, dir, repo, branch, process.Path, nil
}

// sourceFinder adapts a privilegedSource to the interface the proxy wants for
// task tagging.
type sourceFinder struct {
	source privilegedSource
}

func (f sourceFinder) ByLocalPort(port int) (platform.Process, error) {
	pid, name, dir, repo, branch, exe, err := f.source.ProcessOnPort(port)
	if err != nil {
		return platform.Process{}, err
	}

	// The working directory and its checkout came back in the same answer, so
	// remember them rather than asking again.
	f.remember(pid, dir, repo, branch)

	return platform.Process{PID: pid, Name: name, Path: exe}, nil
}

func (f sourceFinder) WorkingDir(pid int) (string, error) {
	return lookupRememberedDir(pid)
}
