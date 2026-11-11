package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- fix 6: doctor is a functional check ----------

func TestDoctorGatekeeperProbeFailureIsFAIL(t *testing.T) {
	cleanEnv(t)
	checkout, _ := fakeGK(t)
	fakeCLI(t, checkout)
	captureChildren(t, 9) // every probe exits 9
	code, out, _ := run(t, "doctor")
	if code == 0 {
		t.Fatalf("doctor with a failing gatekeeper probe: exit 0, want nonzero\n%s", out)
	}
	if !strings.Contains(out, "FAIL") {
		t.Fatalf("doctor output must report FAIL for a component that cannot execute:\n%s", out)
	}
}

func TestDoctorDaemonPingLive(t *testing.T) {
	cleanEnv(t)
	checkout, _ := fakeGK(t)
	fakeCLI(t, checkout)
	captureChildren(t, 0)
	state := t.TempDir()
	t.Setenv("SYNCYARD_STATE_HOME", state)
	if err := os.WriteFile(filepath.Join(state, "gk.sock"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	payloads := captureSocket(t, `{"ok":true}`, nil)
	code, out, _ := run(t, "doctor")
	if code != 0 {
		t.Fatalf("doctor: code=%d\n%s", code, out)
	}
	if !strings.Contains(out, "live") {
		t.Fatalf("doctor must report the daemon live after a successful ping:\n%s", out)
	}
	if len(*payloads) != 1 || string((*payloads)[0]) != `{"verb":"ping"}` {
		t.Fatalf("doctor ping payload = %v, want a single ping request", *payloads)
	}
}

func TestDoctorDaemonPingStale(t *testing.T) {
	cleanEnv(t)
	checkout, _ := fakeGK(t)
	fakeCLI(t, checkout)
	captureChildren(t, 0)
	state := t.TempDir()
	t.Setenv("SYNCYARD_STATE_HOME", state)
	// A socket path exists (stale file from a dead daemon)
	// but the ping is refused.
	if err := os.WriteFile(filepath.Join(state, "gk.sock"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	captureSocket(t, "", errors.New("connect: connection refused"))
	code, out, _ := run(t, "doctor")
	if code == 0 {
		t.Fatalf("doctor with a stale socket: exit 0, want nonzero\n%s", out)
	}
	if !strings.Contains(out, "stale") || !strings.Contains(out, "FAIL") {
		t.Fatalf("doctor must report a stale socket as FAIL, not 'socket present':\n%s", out)
	}
}
