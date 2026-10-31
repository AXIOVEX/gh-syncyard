// Command gh-syncyard is the Syncyard extension for the GitHub CLI.
//
// It is a thin dispatcher: the Syncyard gatekeeper (gatekeeper/gk.py,
// Python, in the AXIOVEX/syncyard project) stays the engine. This
// binary only locates the locally installed gatekeeper and forwards
// every `gh syncyard ...` invocation to it, streaming stdin, stdout,
// stderr and the exit code unchanged.
//
// Gatekeeper resolution order:
//  1. $SYNCYARD_GK            — explicit path to gk.py
//  2. $SYNCYARD_HOME/gatekeeper/gk.py
//  3. ~/syncyard/gatekeeper/gk.py
//  4. ~/.syncyard/gatekeeper/gk.py
//  5. gatekeeper/gk.py next to, or one level up from, this binary
//  6. ./gatekeeper/gk.py relative to the working directory
//
// Python resolution: $SYNCYARD_PYTHON, else the first of
// python3 / python / py found on PATH (py is invoked as `py -3`).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const version = "0.1.0"

const helpText = `gh syncyard — Syncyard for the GitHub CLI

Syncyard dispatches jobs to your own machines through self-hosted
runner lanes, supervised by a local gatekeeper. This extension is a
thin dispatcher: it forwards everything to the gatekeeper
(gatekeeper/gk.py) from your AXIOVEX/syncyard install.

Usage:
  gh syncyard <gatekeeper command> [args...]

Gatekeeper commands include:
  submit   queue a job script for dispatch to a runner lane
  status   queue, lane and run state
  sweep    one dispatch + watch pass
  outbox   push notifications owed to registered chats
  cancel   cancel a queued (not yet dispatched) job
  pause    stop dispatching (watching/harvesting continues)
  resume   resume dispatching

Extension-native commands:
  gh syncyard version   print the extension version
  gh syncyard doctor    show how the gatekeeper was located
  gh syncyard help      this text

Gatekeeper lookup: $SYNCYARD_GK, then $SYNCYARD_HOME/gatekeeper/gk.py,
then ~/syncyard, ~/.syncyard, beside this binary, or ./gatekeeper.
Python: $SYNCYARD_PYTHON, else python3 / python / py on PATH.
`

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// resolveGatekeeper returns the path to gk.py and how it was found.
func resolveGatekeeper() (string, string) {
	if p := os.Getenv("SYNCYARD_GK"); p != "" {
		return p, "$SYNCYARD_GK"
	}
	var candidates []struct {
		path string
		how  string
	}
	if home := os.Getenv("SYNCYARD_HOME"); home != "" {
		candidates = append(candidates, struct {
			path string
			how  string
		}{filepath.Join(home, "gatekeeper", "gk.py"), "$SYNCYARD_HOME/gatekeeper/gk.py"})
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			struct {
				path string
				how  string
			}{filepath.Join(home, "syncyard", "gatekeeper", "gk.py"), "~/syncyard/gatekeeper/gk.py"},
			struct {
				path string
				how  string
			}{filepath.Join(home, ".syncyard", "gatekeeper", "gk.py"), "~/.syncyard/gatekeeper/gk.py"},
		)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			struct {
				path string
				how  string
			}{filepath.Join(dir, "gatekeeper", "gk.py"), "beside the extension binary"},
			struct {
				path string
				how  string
			}{filepath.Join(dir, "..", "gatekeeper", "gk.py"), "one level up from the extension binary"},
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, struct {
			path string
			how  string
		}{filepath.Join(cwd, "gatekeeper", "gk.py"), "./gatekeeper/gk.py"})
	}
	for _, c := range candidates {
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

func printSetupHelp() {
	fmt.Fprintf(os.Stderr, `gh-syncyard: no Syncyard gatekeeper found.

The extension dispatches to the gatekeeper from a local
AXIOVEX/syncyard install. To set it up:

  git clone https://github.com/AXIOVEX/syncyard ~/syncyard

or point the extension at an existing checkout:

  export SYNCYARD_HOME=/path/to/syncyard-checkout
  # or, for the gatekeeper file itself:
  export SYNCYARD_GK=/path/to/gatekeeper/gk.py

Then re-run:  gh syncyard doctor
`)
}

func doctor() int {
	fmt.Printf("gh-syncyard %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	gk, how := resolveGatekeeper()
	if gk == "" {
		fmt.Println("gatekeeper: NOT FOUND")
		printSetupHelp()
		return 2
	}
	fmt.Printf("gatekeeper: %s (via %s)\n", gk, how)
	py, pre := resolvePython()
	if py == "" {
		fmt.Println("python: NOT FOUND on PATH (set $SYNCYARD_PYTHON)")
		return 2
	}
	fmt.Printf("python: %s %s\n", py, strings.Join(pre, " "))
	return 0
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(helpText)
		os.Exit(0)
	}
	switch args[0] {
	case "version", "--version", "-version":
		fmt.Printf("gh-syncyard %s\n", version)
		os.Exit(0)
	case "help", "--help", "-h":
		fmt.Print(helpText)
		os.Exit(0)
	case "doctor":
		os.Exit(doctor())
	}

	gk, _ := resolveGatekeeper()
	if gk == "" {
		printSetupHelp()
		os.Exit(2)
	}
	py, pre := resolvePython()
	if py == "" {
		fmt.Fprintln(os.Stderr, "gh-syncyard: no Python interpreter found on PATH (looked for python3, python, py). Set $SYNCYARD_PYTHON.")
		os.Exit(2)
	}
	full := append(append([]string{}, pre...), gk)
	full = append(full, args...)
	cmd := exec.Command(py, full...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "gh-syncyard: failed to launch gatekeeper: %v\n", err)
		os.Exit(1)
	}
}
