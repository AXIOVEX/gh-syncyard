//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Doctor probes with REAL fake executables on PATH (no
// transport fakes): a python3 script and a syncyard-cli
// script, executed by the default child runner.

func writeExe(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func doctorWithFakeTools(t *testing.T, cliBody string) (int, string) {
	t.Helper()
	home := cleanEnv(t)
	bin := filepath.Join(home, "bin")
	writeExe(t, bin, "python3", "#!/bin/sh\nexit 0\n")
	writeExe(t, bin, "syncyard-cli", cliBody)
	// A gatekeeper file for the resolver to find (executed
	// only through the fake python3 above).
	gkDir := filepath.Join(home, "syncyard", "gatekeeper")
	if err := os.MkdirAll(gkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gk := filepath.Join(gkDir, "gk.py")
	if err := os.WriteFile(gk, []byte("# fake gk.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SYNCYARD_GK", gk)
	t.Setenv("PATH", bin)
	t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
	code, out, _ := run(t, "doctor")
	return code, out
}

func TestDoctorExecutesFakeExecutablesOnPath(t *testing.T) {
	cli := "#!/bin/sh\ncase \"$1\" in\nversion) echo '{\"ok\":true,\"version\":\"0.4.0\"}';;\nschema) echo '{\"contract_version\":1,\"version\":\"0.4.0\",\"verbs\":[]}';;\n*) exit 2;;\nesac\n"
	code, out := doctorWithFakeTools(t, cli)
	if code != 0 {
		t.Fatalf("doctor with working fake tools: code=%d\n%s", code, out)
	}
	if !strings.Contains(out, "probe: OK") {
		t.Fatalf("doctor must report probe results from actually executing the tools:\n%s", out)
	}
}

func TestDoctorFakeCLIProbeFailureIsFAIL(t *testing.T) {
	cli := "#!/bin/sh\ncase \"$1\" in\nversion) echo '{\"ok\":true,\"version\":\"0.4.0\"}';;\nschema) exit 9;;\n*) exit 2;;\nesac\n"
	code, out := doctorWithFakeTools(t, cli)
	if code == 0 {
		t.Fatalf("doctor with a failing syncyard-cli schema probe: exit 0, want nonzero\n%s", out)
	}
	if !strings.Contains(out, "FAIL") {
		t.Fatalf("doctor output must report FAIL:\n%s", out)
	}
}
