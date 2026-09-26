package main

import "runtime"

// Words that differ per system in what the commands print, so a Windows PC is
// not told about "this Mac" and "sudo".
var (
	machine    = "Mac"
	trustStore = "the System keychain"
	killSwitch = "sudo ./scripts/killswitch.sh"

	// restartWorker makes the installed worker re-read agent.conf.
	restartWorker = "sudo launchctl kickstart -k system/com.aiul.agent"
)

func init() {
	switch runtime.GOOS {
	case "windows":
		machine = "PC"
		trustStore = `LocalMachine\Root`
		killSwitch = `powershell -ExecutionPolicy Bypass -File scripts\killswitch.ps1   (Administrator)`
		restartWorker = "Restart-Service aiul   (Administrator)"
	case "linux":
		machine = "computer"
		trustStore = "the system CA store"
		restartWorker = "sudo systemctl restart aiul"
	}
}

// asAdmin renders an aiul command that needs administrator rights.
func asAdmin(cmd string) string {
	if runtime.GOOS == "windows" {
		return cmd + "   (from an Administrator PowerShell)"
	}
	return "sudo " + cmd
}
