package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func dirOf(p string) string { return filepath.Dir(p) }

type candidate struct {
	path string
	how  string
}

// gatekeeperCandidates is the install-path lookup, in order. It
// covers this fleet's real locations (spec 031: the lookup must
// know the fleet, and fail honestly when none is found).
func gatekeeperCandidates() []candidate {
	var out []candidate
	add := func(path, how string) { out = append(out, candidate{path, how}) }

	if p := os.Getenv("SYNCYARD_GK"); p != "" {
		add(p, "$SYNCYARD_GK")
	}
	if home := os.Getenv("SYNCYARD_HOME"); home != "" {
		add(filepath.Join(home, "gatekeeper", "gk.py"), "$SYNCYARD_HOME/gatekeeper/gk.py")
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, "syncyard", "gatekeeper", "gk.py"), "~/syncyard/gatekeeper/gk.py")
		add(filepath.Join(home, ".syncyard", "gatekeeper", "gk.py"), "~/.syncyard/gatekeeper/gk.py")
		// This fleet: the VM-local Desktop Access gatekeeper.
		add(filepath.Join(home, "workspace", "desktop-access-migration", "gatekeeper", "gk.py"),
			"~/workspace/desktop-access-migration/gatekeeper/gk.py (fleet VM gatekeeper)")
	}
	// This fleet: the mini-server system install.
	add("/var/lib/syncyard/gatekeeper/gk.py", "/var/lib/syncyard/gatekeeper/gk.py (mini-server install)")
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		add(filepath.Join(dir, "gatekeeper", "gk.py"), "beside the extension binary")
		add(filepath.Join(dir, "..", "gatekeeper", "gk.py"), "one level up from the extension binary")
	}
	if cwd, err := os.Getwd(); err == nil {
		add(filepath.Join(cwd, "gatekeeper", "gk.py"), "./gatekeeper/gk.py")
	}
	return out
}

// resolveGatekeeper returns the path to gk.py and how it was found.
func resolveGatekeeper() (string, string) {
	for _, c := range gatekeeperCandidates() {
		if fileExists(c.path) {
			return c.path, c.how
		}
	}
	return "", ""
}

// resolvePython returns the interpreter command and any leading args.
func resolvePython() (string, []string) {
	if p := os.Getenv("SYNCYARD_PYTHON"); p != "" {
		return p, nil
	}
	for _, name := range []string{"python3", "python", "py"} {
		if path, err := exec.LookPath(name); err == nil {
			if name == "py" {
				return path, []string{"-3"}
			}
			return path, nil
		}
	}
	return "", nil
}

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// resolveSyncyardCLI locates the Rust CLI binary and how.
func resolveSyncyardCLI() (string, string) {
	var out []candidate
	add := func(path, how string) { out = append(out, candidate{path, how}) }

	if p := os.Getenv("SYNCYARD_CLI"); p != "" {
		add(p, "$SYNCYARD_CLI")
	}
	// Beside the resolved gatekeeper's checkout:
	// <checkout>/rust/target/{release,debug}/syncyard-cli.
	var roots []string
	if gk, _ := resolveGatekeeper(); gk != "" {
		roots = append(roots, filepath.Dir(filepath.Dir(gk)))
	}
	if home := os.Getenv("SYNCYARD_HOME"); home != "" {
		roots = append(roots, home)
	}
	for _, root := range roots {
		for _, profile := range []string{"release", "debug"} {
			add(filepath.Join(root, "rust", "target", profile, exeName("syncyard-cli")),
				"rust/target/"+profile+" under the Syncyard checkout")
		}
	}
	if exe, err := os.Executable(); err == nil {
		add(filepath.Join(filepath.Dir(exe), exeName("syncyard-cli")), "beside the extension binary")
	}
	for _, c := range out {
		if fileExists(c.path) {
			return c.path, c.how
		}
	}
	if p, err := exec.LookPath("syncyard-cli"); err == nil {
		return p, "PATH"
	}
	return "", ""
}

func printSetupHelp(w io.Writer) {
	fmt.Fprintf(w, `gh-syncyard: no Syncyard gatekeeper found.

The extension dispatches to the gatekeeper from a local
AXIOVEX/syncyard install. To set it up:

  git clone https://github.com/AXIOVEX/syncyard ~/syncyard

or point the extension at an existing checkout:

  export SYNCYARD_HOME=/path/to/syncyard-checkout
  # or, for the gatekeeper file itself:
  export SYNCYARD_GK=/path/to/gatekeeper/gk.py

Fleet locations already in the lookup: ~/syncyard, ~/.syncyard,
~/workspace/desktop-access-migration/gatekeeper (this fleet's VM
gatekeeper) and /var/lib/syncyard/gatekeeper (mini-server).

Then re-run:  gh syncyard doctor
`)
}

// doctor reports exactly how each engine surface was resolved.
func doctor(w io.Writer) int {
	code := 0
	fmt.Fprintf(w, "gh-syncyard %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)

	gk, how := resolveGatekeeper()
	if gk == "" {
		fmt.Fprintln(w, "gatekeeper (gk.py): NOT FOUND")
		printSetupHelp(w)
		code = 2
	} else {
		fmt.Fprintf(w, "gatekeeper (gk.py): %s (via %s)\n", gk, how)
	}
	py, pre := resolvePython()
	if py == "" {
		fmt.Fprintln(w, "python: NOT FOUND on PATH (set $SYNCYARD_PYTHON)")
		code = 2
	} else {
		fmt.Fprintf(w, "python: %s %s\n", py, strings.Join(pre, " "))
	}

	cli, cliHow := resolveSyncyardCLI()
	if cli == "" {
		fmt.Fprintln(w, "syncyard-cli (Rust): NOT FOUND (Rust-only verbs unavailable; set $SYNCYARD_CLI or build the Rust workspace)")
	} else {
		fmt.Fprintf(w, "syncyard-cli (Rust): %s (via %s)\n", cli, cliHow)
	}
	home := rustStateHome()
	if home == "" {
		fmt.Fprintln(w, "rust state home: NOT RESOLVED (set $SYNCYARD_STATE_HOME)")
	} else {
		sock := filepath.Join(home, "gk.sock")
		state := "no live socket (daemon not running)"
		if _, err := os.Stat(sock); err == nil {
			state = "socket present"
		}
		fmt.Fprintf(w, "rust state home: %s (daemon socket %s: %s)\n", home, sock, state)
	}
	return code
}
