package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ---------- test scaffolding ----------

// cleanEnv pins every lookup input so tests are hermetic.
func cleanEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SYNCYARD_GK", "")
	t.Setenv("SYNCYARD_HOME", "")
	t.Setenv("SYNCYARD_PYTHON", "")
	t.Setenv("SYNCYARD_CLI", "")
	t.Setenv("SYNCYARD_STATE_HOME", "")
	t.Setenv("PATH", home) // empty of tools, on purpose
	return home
}

// fakeGK installs a dummy gatekeeper tree under a temp checkout
// and points the lookup at it. Returns (checkout, gkPath).
func fakeGK(t *testing.T) (string, string) {
	t.Helper()
	checkout := t.TempDir()
	gkDir := filepath.Join(checkout, "gatekeeper")
	if err := os.MkdirAll(gkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gk := filepath.Join(gkDir, "gk.py")
	if err := os.WriteFile(gk, []byte("# dummy gk.py for tests\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SYNCYARD_GK", gk)
	t.Setenv("SYNCYARD_PYTHON", "python-test-double")
	return checkout, gk
}

// fakeCLI installs a dummy syncyard-cli under the checkout's
// rust target dir and returns its path.
func fakeCLI(t *testing.T, checkout string) string {
	t.Helper()
	dir := filepath.Join(checkout, "rust", "target", "release")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cli := filepath.Join(dir, exeName("syncyard-cli"))
	if err := os.WriteFile(cli, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return cli
}

type childCall struct {
	argv []string
}

// captureChildren replaces the child-process transport with a
// recorder returning the given exit code.
func captureChildren(t *testing.T, code int) *[]childCall {
	t.Helper()
	calls := &[]childCall{}
	old := runChild
	runChild = func(argv []string, _ io.Reader, stdout, stderr io.Writer) int {
		*calls = append(*calls, childCall{append([]string{}, argv...)})
		return code
	}
	t.Cleanup(func() { runChild = old })
	return calls
}

// captureSocket replaces the daemon-socket transport.
func captureSocket(t *testing.T, reply string, err error) *[][]byte {
	t.Helper()
	payloads := &[][]byte{}
	old := socketCall
	socketCall = func(path string, payload []byte) (string, error) {
		*payloads = append(*payloads, append([]byte{}, payload...))
		return reply, err
	}
	t.Cleanup(func() { socketCall = old })
	return payloads
}

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runMain(args, &out, &errb)
	return code, out.String(), errb.String()
}

// ---------- native surface ----------

func TestVersionVerb(t *testing.T) {
	cleanEnv(t)
	code, out, _ := run(t, "version")
	if code != 0 || strings.TrimSpace(out) != "gh-syncyard "+version {
		t.Fatalf("version: code=%d out=%q", code, out)
	}
}

func TestNoArgsPrintsHelp(t *testing.T) {
	cleanEnv(t)
	code, out, _ := run(t)
	if code != 0 || !strings.Contains(out, "gh syncyard — Syncyard") {
		t.Fatalf("no-args help: code=%d", code)
	}
}

func TestHelpCoversEveryVerbAndGap(t *testing.T) {
	for v := range pythonVerbs {
		if !strings.Contains(helpText, v) {
			t.Errorf("help omits python verb %q", v)
		}
	}
	for v := range rustCLIVerbs {
		if !strings.Contains(helpText, v) {
			t.Errorf("help omits rust-cli verb %q", v)
		}
	}
	for v := range socketVerbs {
		if !strings.Contains(helpText, v) {
			t.Errorf("help omits socket verb %q", v)
		}
	}
	for v := range rustNotYetVerbs {
		if !strings.Contains(helpText, v) {
			t.Errorf("help omits named gap %q", v)
		}
	}
	for _, want := range []string{"--needs-gpu", "--venue", "agent_poll", "Known gaps", "desktop-access-migration", "/var/lib/syncyard"} {
		if !strings.Contains(helpText, want) {
			t.Errorf("help omits %q", want)
		}
	}
}

func TestUnknownVerbRefused(t *testing.T) {
	cleanEnv(t)
	code, _, errOut := run(t, "frobnicate")
	if code != 2 || !strings.Contains(errOut, "unknown verb") {
		t.Fatalf("unknown verb: code=%d err=%q", code, errOut)
	}
}

// ---------- drift / parity tests (spec 031: proven, not by eye) ----------

func readLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	sort.Strings(out)
	return out
}

func TestParityPythonVerbsMatchGKPy(t *testing.T) {
	// testdata/gk_verbs.txt is extracted mechanically from
	// gk.py's argparse setup (add_parser names) in the live
	// fleet gatekeeper. If gk.py gains or loses a verb, this
	// fails until the extension is refreshed: the spec 031
	// parity check.
	want := readLines(t, "testdata/gk_verbs.txt")
	got := sortedKeys(pythonVerbs)
	if strings.Join(want, ",") != strings.Join(got, ",") {
		t.Fatalf("python verb drift:\n gk.py: %v\n extension: %v", want, got)
	}
}

type cliSchema struct {
	ContractVersion int      `json:"contract_version"`
	Version         string   `json:"version"`
	Verbs           []string `json:"verbs"`
	NotYet          []struct {
		Verb string `json:"verb"`
		Note string `json:"note"`
	} `json:"not_yet_verbs"`
}

func loadSchema(t *testing.T) cliSchema {
	t.Helper()
	b, err := os.ReadFile("testdata/cli_schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s cliSchema
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestParityCLISchema(t *testing.T) {
	// testdata/cli_schema.json is captured from the built
	// syncyard-cli's own `schema` verb (the generation source
	// for this shim, spec 038 section 9).
	s := loadSchema(t)
	if s.Version != buildParity().EngineVersion {
		t.Errorf("schema version %q != parity engine version %q", s.Version, buildParity().EngineVersion)
	}
	if s.ContractVersion != buildParity().ContractVersion {
		t.Errorf("schema contract %d != parity contract %d", s.ContractVersion, buildParity().ContractVersion)
	}
	for _, v := range s.Verbs {
		switch {
		case cliSharedVerbs[v]:
			if !pythonVerbs[v] {
				t.Errorf("schema shared verb %q must route to python until cutover", v)
			}
		case cliOnlyVerbs[v]:
			if !rustCLIVerbs[v] && !nativeVerbs[v] {
				t.Errorf("schema rust-only verb %q is not wired (rustCLIVerbs/nativeVerbs)", v)
			}
		default:
			t.Errorf("schema verb %q unaccounted for in the extension tables", v)
		}
	}
	// Every rust-cli verb we advertise must be a real schema verb.
	for v := range rustCLIVerbs {
		found := false
		for _, sv := range s.Verbs {
			if sv == v {
				found = true
			}
		}
		if !found {
			t.Errorf("rustCLIVerbs %q not in CLI schema verbs", v)
		}
	}
	// not_yet parity, both directions, and all served by python.
	if len(s.NotYet) != len(rustNotYetVerbs) {
		t.Fatalf("not_yet count drift: schema %d, extension %d", len(s.NotYet), len(rustNotYetVerbs))
	}
	for _, ny := range s.NotYet {
		note, ok := rustNotYetVerbs[ny.Verb]
		if !ok {
			t.Errorf("schema not_yet verb %q missing from extension gap table", ny.Verb)
		} else if note != ny.Note {
			t.Errorf("not_yet note drift for %q: %q vs schema %q", ny.Verb, note, ny.Note)
		}
		if !pythonVerbs[ny.Verb] {
			t.Errorf("not_yet verb %q must still be served by the python engine", ny.Verb)
		}
	}
}

func TestSocketVerbsAreSpec037Surface(t *testing.T) {
	// The daemon's control surface (serve.rs): control_command /
	// control_outcomes / kill_switch. kill_switch routes through
	// the CLI's kill-switch verb; the two file/outcome verbs are
	// socket-only.
	if !socketVerbs["control-outcomes"] || !socketVerbs["control-command"] {
		t.Fatal("socket verbs must cover control-outcomes and control-command")
	}
	if !rustCLIVerbs["kill-switch"] {
		t.Fatal("kill-switch must route to the CLI")
	}
	for v := range daemonInternalVerbs {
		if pythonVerbs[v] || rustCLIVerbs[v] || socketVerbs[v] {
			t.Errorf("daemon-internal verb %q must not be a user verb", v)
		}
	}
}

// ---------- forwarding (mocked transport) ----------

func TestPythonForwardVerbatim(t *testing.T) {
	cleanEnv(t)
	_, gk := fakeGK(t)
	calls := captureChildren(t, 7)
	code, _, _ := run(t, "status", "--repo")
	if code != 7 {
		t.Fatalf("exit code must propagate: got %d", code)
	}
	if len(*calls) != 1 {
		t.Fatalf("expected 1 child call, got %d", len(*calls))
	}
	want := []string{"python-test-double", gk, "status", "--repo"}
	if strings.Join((*calls)[0].argv, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v, want %v", (*calls)[0].argv, want)
	}
}

func TestSubmitSpec016RoutingArgsForwardVerbatim(t *testing.T) {
	cleanEnv(t)
	_, gk := fakeGK(t)
	calls := captureChildren(t, 0)
	args := []string{"submit", "--program", "p", "--workflow", "run.yml",
		"--lane", "auto", "--needs-cpu", "4", "--needs-ram-mb", "8192",
		"--needs-gpu", "12000", "--venue", "docker", "--image", "img",
		"--key", "k", "--script", "s.ps1", "--cwd", "C:\\x"}
	code, _, _ := run(t, args...)
	if code != 0 || len(*calls) != 1 {
		t.Fatalf("submit: code=%d calls=%d", code, len(*calls))
	}
	got := (*calls)[0].argv
	want := append([]string{"python-test-double", gk}, args...)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("submit argv drift:\n got %v\nwant %v", got, want)
	}
}

func TestEveryPythonVerbForwards(t *testing.T) {
	cleanEnv(t)
	fakeGK(t)
	calls := captureChildren(t, 0)
	for v := range pythonVerbs {
		before := len(*calls)
		if code, _, _ := run(t, v); code != 0 {
			t.Errorf("verb %q: code %d", v, code)
		}
		if len(*calls) != before+1 {
			t.Errorf("verb %q did not forward", v)
			continue
		}
		if (*calls)[before].argv[2] != v {
			t.Errorf("verb %q forwarded as %q", v, (*calls)[before].argv[2])
		}
	}
}

func TestPythonForwardWithoutGatekeeper(t *testing.T) {
	cleanEnv(t) // HOME is an empty temp dir; no fleet paths exist under it
	code, _, errOut := run(t, "status")
	if code != 2 || !strings.Contains(errOut, "no Syncyard gatekeeper found") {
		t.Fatalf("missing gatekeeper: code=%d err=%q", code, errOut)
	}
}

func TestRustCLIForwardWithExplicitHome(t *testing.T) {
	cleanEnv(t)
	checkout, _ := fakeGK(t)
	cli := fakeCLI(t, checkout)
	stateHome := t.TempDir()
	t.Setenv("SYNCYARD_STATE_HOME", stateHome)
	calls := captureChildren(t, 3)
	code, _, _ := run(t, "progress", "job-123")
	if code != 3 {
		t.Fatalf("exit code must propagate: got %d", code)
	}
	want := []string{cli, "--home", stateHome, "progress", "job-123"}
	if len(*calls) != 1 || strings.Join((*calls)[0].argv, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v, want %v", *calls, want)
	}
}

func TestEveryRustCLIVerbForwards(t *testing.T) {
	cleanEnv(t)
	checkout, _ := fakeGK(t)
	fakeCLI(t, checkout)
	t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
	calls := captureChildren(t, 0)
	for v := range rustCLIVerbs {
		before := len(*calls)
		if code, _, _ := run(t, v); code != 0 {
			t.Errorf("verb %q: code %d", v, code)
		}
		if len(*calls) != before+1 || (*calls)[before].argv[3] != v {
			t.Errorf("verb %q did not forward to the CLI", v)
		}
	}
}

func TestRustVerbWithoutCLIFailsHonestly(t *testing.T) {
	cleanEnv(t)
	fakeGK(t) // gatekeeper exists, but no syncyard-cli anywhere
	t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
	code, _, errOut := run(t, "progress", "job-1")
	if code != 2 || !strings.Contains(errOut, "syncyard-cli not found") {
		t.Fatalf("missing cli: code=%d err=%q", code, errOut)
	}
}

func TestKillSwitchForwards(t *testing.T) {
	cleanEnv(t)
	checkout, _ := fakeGK(t)
	fakeCLI(t, checkout)
	t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
	calls := captureChildren(t, 0)
	if code, _, _ := run(t, "kill-switch", "on"); code != 0 {
		t.Fatalf("kill-switch: code %d", code)
	}
	got := (*calls)[0].argv
	if got[3] != "kill-switch" || got[4] != "on" {
		t.Fatalf("kill-switch argv = %v", got)
	}
}

// ---------- daemon socket verbs (mocked transport) ----------

func TestControlOutcomesViaSocket(t *testing.T) {
	cleanEnv(t)
	t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
	payloads := captureSocket(t, `{"ok":true,"outcomes":[]}`, nil)
	code, out, _ := run(t, "control-outcomes")
	if code != 0 || !strings.Contains(out, "outcomes") {
		t.Fatalf("control-outcomes: code=%d out=%q", code, out)
	}
	if len(*payloads) != 1 || string((*payloads)[0]) != `{"verb":"control_outcomes"}` {
		t.Fatalf("payload = %v", *payloads)
	}
}

func TestControlCommandViaSocket(t *testing.T) {
	cleanEnv(t)
	t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
	cmdFile := filepath.Join(t.TempDir(), "cmd.json")
	cmdJSON := `{"command_id":"c1","action":"cancel","target":"job-9"}`
	if err := os.WriteFile(cmdFile, []byte(cmdJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	payloads := captureSocket(t, `{"ok":true,"outcome":{"state":"executed"}}`, nil)
	code, _, _ := run(t, "control-command", cmdFile)
	if code != 0 {
		t.Fatalf("control-command: code=%d", code)
	}
	var sent struct {
		Verb    string          `json:"verb"`
		Command json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal((*payloads)[0], &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Verb != "control_command" || string(sent.Command) != cmdJSON {
		t.Fatalf("payload = %s", (*payloads)[0])
	}
}

func TestControlCommandDaemonRefusalExits1(t *testing.T) {
	cleanEnv(t)
	t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
	captureSocket(t, `{"ok":false,"error":"bad signature"}`, nil)
	code, _, _ := run(t, "control-outcomes")
	if code != 1 {
		t.Fatalf("ok:false must exit 1, got %d", code)
	}
}

func TestControlCommandBadArgs(t *testing.T) {
	cleanEnv(t)
	t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
	if code, _, _ := run(t, "control-command"); code != 2 {
		t.Fatalf("missing file: code=%d", code)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte("not json"), 0o600)
	if code, _, _ := run(t, "control-command", bad); code != 2 {
		t.Fatalf("invalid json: code=%d", code)
	}
}

func TestSocketVerbWithoutDaemon(t *testing.T) {
	cleanEnv(t)
	t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
	captureSocket(t, "", io.ErrUnexpectedEOF)
	code, _, errOut := run(t, "control-outcomes")
	if code != 2 || !strings.Contains(errOut, "daemon socket") {
		t.Fatalf("socket failure: code=%d err=%q", code, errOut)
	}
}

// ---------- doctor / parity / schema ----------

func TestDoctorResolvesFleet(t *testing.T) {
	cleanEnv(t)
	checkout, gk := fakeGK(t)
	fakeCLI(t, checkout)
	code, out, _ := run(t, "doctor")
	if code != 0 {
		t.Fatalf("doctor: code=%d out=%q", code, out)
	}
	for _, want := range []string{gk, "syncyard-cli (Rust)", "rust state home"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestParityAndSchemaCommands(t *testing.T) {
	cleanEnv(t)
	code, out, _ := run(t, "parity")
	if code != 0 || !strings.Contains(out, "verb parity") {
		t.Fatalf("parity: code=%d", code)
	}
	code, out, _ = run(t, "schema")
	if code != 0 {
		t.Fatalf("schema: code=%d", code)
	}
	var d parityData
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	wantRows := len(pythonVerbs) + len(rustCLIVerbs) + len(socketVerbs) + 5
	if len(d.Verbs) != wantRows {
		t.Fatalf("schema rows = %d, want %d", len(d.Verbs), wantRows)
	}
	if d.ExtensionVersion != version {
		t.Fatalf("schema extension version = %q, want %q", d.ExtensionVersion, version)
	}
	code, out, _ = run(t, "parity", "--json")
	if code != 0 || !strings.Contains(out, `"extension_version"`) {
		t.Fatalf("parity --json: code=%d", code)
	}
}

// ---------- install-path lookup ----------

func TestGatekeeperCandidatesCoverFleetPaths(t *testing.T) {
	cleanEnv(t)
	var hows []string
	for _, c := range gatekeeperCandidates() {
		hows = append(hows, c.how)
	}
	joined := strings.Join(hows, " | ")
	for _, want := range []string{"desktop-access-migration", "/var/lib/syncyard", "~/syncyard", "~/.syncyard"} {
		if !strings.Contains(joined, want) {
			t.Errorf("candidates missing fleet path %q: %s", want, joined)
		}
	}
}

func TestResolveGatekeeperHonestFailure(t *testing.T) {
	home := cleanEnv(t)
	// Ensure the ./gatekeeper candidate cannot hit: tests run in
	// the repo root, which has no gatekeeper/ dir.
	if _, err := os.Stat("gatekeeper"); err == nil {
		t.Skip("repo root unexpectedly contains gatekeeper/")
	}
	if _, err := os.Stat("/var/lib/syncyard/gatekeeper/gk.py"); err == nil {
		t.Skip("/var/lib/syncyard exists on this host")
	}
	_ = home
	if gk, _ := resolveGatekeeper(); gk != "" {
		t.Fatalf("expected honest failure, got %q", gk)
	}
}
