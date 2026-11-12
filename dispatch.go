package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Transport seams. The defaults run real child processes and real
// sockets; tests substitute fakes, so the whole dispatch surface
// is exercised without a gatekeeper, a daemon, or a network.
var (
	// runChild runs argv with the given streams and returns the
	// child's exit code.
	runChild = func(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			fmt.Fprintf(stderr, "gh-syncyard: failed to launch %s: %v\n", argv[0], err)
			return 1
		}
		return 0
	}
	// socketCall sends one newline-JSON request to the daemon
	// socket and returns the raw reply line.
	socketCall = socketCallUnix
	// stdin is what forwarded children inherit.
	stdin io.Reader = os.Stdin
)

// runMain is the whole CLI, returning the process exit code.
func runMain(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, helpText)
		return 0
	}
	verb, rest := args[0], args[1:]

	switch {
	case nativeVerbs[verb]:
		return runNative(verb, rest, stdout, stderr)
	case rustCLIVerbs[verb]:
		return forwardToRustCLI(verb, rest, stdout, stderr)
	case socketVerbs[verb]:
		return runSocketVerb(verb, rest, stdout, stderr)
	case pythonVerbs[verb]:
		return forwardToGK(verb, rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gh-syncyard: unknown verb %q.\n\n%s", verb, helpText)
		return 2
	}
}

// usageError reports a native-verb argument error. Native
// verbs take a fixed argument shape; anything unexpected is a
// usage error (exit 2), never silently ignored.
func usageError(stderr io.Writer, format string, args ...any) int {
	fmt.Fprintf(stderr, "gh-syncyard: usage: "+format+"\n", args...)
	return 2
}

// runNative handles the extension's own verbs.
func runNative(verb string, rest []string, stdout, stderr io.Writer) int {
	switch verb {
	case "version", "--version", "-version":
		if len(rest) > 0 {
			return usageError(stderr, "gh syncyard version (takes no arguments)")
		}
		fmt.Fprintf(stdout, "gh-syncyard %s\n", version)
		return 0
	case "help", "--help", "-h":
		if len(rest) > 0 {
			return usageError(stderr, "gh syncyard help (takes no arguments)")
		}
		fmt.Fprint(stdout, helpText)
		return 0
	case "doctor":
		if len(rest) > 0 {
			return usageError(stderr, "gh syncyard doctor (takes no arguments)")
		}
		return doctor(stdout)
	case "parity":
		return parityCmd(rest, stdout, stderr)
	case "schema":
		if len(rest) > 0 {
			return usageError(stderr, "gh syncyard schema (takes no arguments)")
		}
		return schemaCmd(stdout)
	case "install":
		return installOrUpgrade(false, rest, stdout, stderr)
	case "upgrade":
		return installOrUpgrade(true, rest, stdout, stderr)
	}
	fmt.Fprintf(stderr, "gh-syncyard: unhandled native verb %q\n", verb)
	return 2
}

// forwardToGK is the 0.1.0 path: locate gk.py, locate Python,
// forward the invocation verbatim.
func forwardToGK(verb string, rest []string, stdout, stderr io.Writer) int {
	gk, _ := resolveGatekeeper()
	if gk == "" {
		printSetupHelp(stderr)
		return 2
	}
	py, pre := resolvePython()
	if py == "" {
		fmt.Fprintln(stderr, "gh-syncyard: no Python interpreter found on PATH (looked for python3, python, py). Set $SYNCYARD_PYTHON.")
		return 2
	}
	argv := append([]string{py}, pre...)
	argv = append(argv, gk, verb)
	argv = append(argv, rest...)
	return runChild(argv, stdin, stdout, stderr)
}

// rustStateHome resolves the Rust engine's state home
// explicitly: $SYNCYARD_STATE_HOME when set, else the
// directory holding the resolved gk.py. $SYNCYARD_HOME is
// never used here: it names a checkout root for this
// extension but a state home for the Rust tools, so treating
// it as either one in this function would import the other
// meaning. "" when neither source is available.
func rustStateHome() string {
	if h := os.Getenv("SYNCYARD_STATE_HOME"); h != "" {
		return h
	}
	if gk, _ := resolveGatekeeper(); gk != "" {
		return dirOf(gk)
	}
	return ""
}

// forwardToRustCLI delegates a Rust-only verb to syncyard-cli,
// always with an explicit --home (FR-038-11: no default exists).
func forwardToRustCLI(verb string, rest []string, stdout, stderr io.Writer) int {
	home := rustStateHome()
	if home == "" {
		fmt.Fprintln(stderr, "gh-syncyard: cannot locate the Rust state home for verb "+verb+": set $SYNCYARD_STATE_HOME, or install the gatekeeper so its directory can be used (see: gh syncyard doctor).")
		return 2
	}
	cli, _ := resolveSyncyardCLI()
	if cli == "" {
		fmt.Fprintln(stderr, "gh-syncyard: syncyard-cli not found for verb "+verb+": set $SYNCYARD_CLI or build the Rust workspace (rust/target/release/syncyard-cli) in your Syncyard checkout.")
		return 2
	}
	argv := []string{cli, "--home", home, verb}
	argv = append(argv, rest...)
	// The Rust tools read $SYNCYARD_HOME as the state home,
	// while this extension reads it as a checkout root. For
	// the duration of the child call, set BOTH variables to
	// the resolved state home so the child sees the state
	// home explicitly in its environment and the ambient
	// checkout meaning cannot leak through; the parent's
	// environment is restored on return.
	oldHome, hadHome := os.LookupEnv("SYNCYARD_HOME")
	oldState, hadState := os.LookupEnv("SYNCYARD_STATE_HOME")
	_ = os.Setenv("SYNCYARD_HOME", home)
	_ = os.Setenv("SYNCYARD_STATE_HOME", home)
	defer restoreEnv("SYNCYARD_HOME", oldHome, hadHome)
	defer restoreEnv("SYNCYARD_STATE_HOME", oldState, hadState)
	return runChild(argv, stdin, stdout, stderr)
}

func restoreEnv(name, value string, had bool) {
	if had {
		_ = os.Setenv(name, value)
	} else {
		_ = os.Unsetenv(name)
	}
}

// runSocketVerb speaks the daemon's newline-JSON socket protocol
// for the control surface.
func runSocketVerb(verb string, rest []string, stdout, stderr io.Writer) int {
	home := rustStateHome()
	if home == "" {
		fmt.Fprintln(stderr, "gh-syncyard: cannot locate the Rust state home for verb "+verb+": set $SYNCYARD_STATE_HOME, or install the gatekeeper so its directory can be used (see: gh syncyard doctor).")
		return 2
	}
	sock := home + string(os.PathSeparator) + "gk.sock"

	var payload []byte
	var err error
	switch verb {
	case "control-outcomes":
		if len(rest) > 0 {
			return usageError(stderr, "gh syncyard control-outcomes (takes no arguments)")
		}
		payload = []byte(`{"verb":"control_outcomes"}`)
	case "control-command":
		payload, err = controlCommandPayload(rest)
		if err != nil {
			fmt.Fprintf(stderr, "gh-syncyard: %v\n", err)
			return 2
		}
	}

	reply, err := socketCall(sock, payload)
	if err != nil {
		fmt.Fprintf(stderr, "gh-syncyard: daemon socket %s: %v (is syncyard-daemon running for state home %s?)\n", sock, err, home)
		return 2
	}
	// A daemon reply is only a reply when it is well-formed
	// JSON carrying an explicit boolean "ok". A malformed
	// reply, a missing ok, or a non-object reply is an error
	// with a nonzero exit — never a silent exit 0.
	ok, valid := validateDaemonReply(reply)
	if !valid {
		fmt.Fprintf(stderr, "gh-syncyard: invalid daemon reply from %s: want a JSON object with an \"ok\" field, got %q\n", sock, strings.TrimSpace(reply))
		return 1
	}
	fmt.Fprintln(stdout, strings.TrimSpace(reply))
	if !ok {
		return 1
	}
	return 0
}

// validateDaemonReply parses a daemon socket reply and
// reports its ok flag. valid is false when the reply is not a
// JSON object or its "ok" field is missing or not a boolean.
func validateDaemonReply(reply string) (ok bool, valid bool) {
	trimmed := strings.TrimSpace(reply)
	if trimmed == "" {
		return false, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return false, false
	}
	raw, present := obj["ok"]
	if !present {
		return false, false
	}
	if err := json.Unmarshal(raw, &ok); err != nil {
		return false, false
	}
	return ok, true
}

// controlCommandPayload builds the control_command request from
// the signed command file the tray wrote: `control-command
// <file>` or `control-command --file <file>`. The file's JSON is
// embedded verbatim; verification happens daemon-side.
func controlCommandPayload(rest []string) ([]byte, error) {
	var path string
	for i := 0; i < len(rest); i++ {
		if rest[i] == "--file" && i+1 < len(rest) {
			path = rest[i+1]
			i++
		} else if !strings.HasPrefix(rest[i], "--") && path == "" {
			path = rest[i]
		} else {
			return nil, fmt.Errorf("control-command: unexpected argument %q (usage: gh syncyard control-command <command-file>)", rest[i])
		}
	}
	if path == "" {
		return nil, fmt.Errorf("control-command requires a signed command file (usage: gh syncyard control-command <command-file>)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("control-command: cannot read %s: %w", path, err)
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("control-command: %s is not valid JSON", path)
	}
	out, err := json.Marshal(struct {
		Verb    string          `json:"verb"`
		Command json.RawMessage `json:"command"`
	}{"control_command", raw})
	if err != nil {
		return nil, err
	}
	return out, nil
}
