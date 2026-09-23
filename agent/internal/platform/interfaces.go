package platform

// The interfaces in this file are the complete list of things the agent does to a
// machine. Everything OS-specific lives behind one of them, so porting to Linux or
// Windows later means writing new files in this package and nothing else.
//
// Every one of them can say what it WOULD do before doing it: each has a
// *Commands method returning the exact shell commands, so the user sees them
// first. Rule 1.

// EnvVars are the environment variables that make a tool's runtime trust our CA
// and route through our proxy. Which ones matter per tool is in the capture matrix
// in AI-Usage-Logger-Build-Map.md.
type EnvVars map[string]string

// ProxyConfigurator points the machine's network settings at our proxy.
type ProxyConfigurator interface {
	// SetCommands and UnsetCommands return the exact commands, for showing first.
	SetCommands(hostport string) []string
	UnsetCommands() []string

	// Set points every active network service at hostport.
	Set(hostport string) error

	// Unset removes the proxy from every network service. This is the fail-open
	// path (rule 7) and must work even when everything else is broken.
	Unset() error

	// Current reports where each network service's proxy points, for `aiul status`
	// and for noticing drift.
	Current() (map[string]string, error)
}

// EnvWriter sets machine-level environment variables, so tools started later
// inherit them. On macOS this needs two mechanisms: one for terminals, one for
// GUI applications.
type EnvWriter interface {
	WriteCommands(vars EnvVars) []string
	RemoveCommands() []string

	Write(vars EnvVars) error
	Remove() error

	// Current reports the variables we have written, as they are on disk now.
	Current() (EnvVars, error)
}

// MDMChecker answers whether this device is managed by the company. The agent
// refuses to decrypt anything on an unmanaged device: a personal laptop that
// mistakenly receives the agent must not start reading traffic.
type MDMChecker interface {
	// Enrolled reports whether the device is enrolled, and a human-readable
	// explanation of how that was determined.
	Enrolled() (bool, string, error)
}

// ToolDetector finds the AI tools installed on this machine, so `aiul doctor` can
// say which ones are covered and which need attention.
type ToolDetector interface {
	Detect() []Tool
}

// Tool is one AI tool found on the machine.
type Tool struct {
	Name string // "claude-code", "cursor", ...
	Kind string // "cli" or "ide"
	Path string // where it was found

	// TrustVars are the environment variables this tool's runtime needs before it
	// will accept our CA. Empty means the OS trust store is enough.
	TrustVars []string
}

// ServiceManager installs and removes the background jobs that keep the agent
// running.
type ServiceManager interface {
	InstallCommands() []string
	UninstallCommands() []string

	// Install writes and loads the jobs. extraEnv is written into the job
	// definition: a variable set in the installing shell does not reach a service,
	// so anything the installed processes need must be passed here.
	Install(binaryPath string, extraEnv map[string]string) error
	Uninstall() error

	// Running reports whether our jobs are loaded.
	Running() (bool, error)
}

// ProcessFinder answers "which program opened this connection, and where is it
// working?". That is how an event gets a task ID with no action from the user: the
// process's working directory is a checkout, the checkout has a branch, and the
// branch name carries the ticket.
type ProcessFinder interface {
	// ByLocalPort finds the process that owns a TCP connection from this local
	// port on the loopback interface. A port is reused quickly, so the answer is
	// only meaningful while the connection is open.
	ByLocalPort(port int) (Process, error)

	// WorkingDir returns a process's current working directory.
	WorkingDir(pid int) (string, error)
}

// Process is a program that opened a connection to our proxy.
type Process struct {
	PID  int
	Name string // "claude", "node", "Cursor Helper"

	// Path is the executable, when we could read it:
	// "/Users/x/Library/Application Support/Claude/claude-code/2.1.275/claude.app/Contents/MacOS/claude".
	//
	// Name alone is NOT an identity. The Claude desktop app bundles its own copy
	// of Claude Code, and the process name of both that copy and the terminal CLI
	// is "claude". Anything that has to tell two programs apart — the proxy's
	// tunnel list, for one — must use this.
	Path string

	// Account is the AI account the program is signed in with, when the machine
	// says so locally (Claude Code's ~/.claude.json). Empty otherwise.
	Account string
}

// Identity returns the most specific stable name we have for a program: its
// executable path, or its process name when the path could not be read.
func (p Process) Identity() string {
	if p.Path != "" {
		return p.Path
	}
	return p.Name
}
