package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------- fix 5: SYNCYARD_HOME collision ----------

func TestRustStateHomeResolutionOrder(t *testing.T) {
	cleanEnv(t)
	_, gk := fakeGK(t)
	// No SYNCYARD_STATE_HOME: the gatekeeper's directory.
	if got := rustStateHome(); got != filepath.Dir(gk) {
		t.Fatalf("state home = %q, want gatekeeper dir %q", got, filepath.Dir(gk))
	}
	// SYNCYARD_STATE_HOME wins over the gatekeeper directory.
	custom := t.TempDir()
	t.Setenv("SYNCYARD_STATE_HOME", custom)
	if got := rustStateHome(); got != custom {
		t.Fatalf("state home = %q, want SYNCYARD_STATE_HOME %q", got, custom)
	}
	// SYNCYARD_HOME is a checkout root for this extension and
	// a state home for the Rust tools; it must never be used
	// as the state home here.
	t.Setenv("SYNCYARD_STATE_HOME", "")
	t.Setenv("SYNCYARD_GK", "")
	t.Setenv("SYNCYARD_HOME", filepath.Join(t.TempDir(), "state-only"))
	if got := rustStateHome(); got != "" {
		t.Fatalf("state home = %q, want \"\" (SYNCYARD_HOME is not a state home)", got)
	}
}

func TestRustForwardPassesStateHomeExplicitlyInEnv(t *testing.T) {
	cleanEnv(t)
	checkout, _ := fakeGK(t)
	fakeCLI(t, checkout)
	stateHome := t.TempDir()
	t.Setenv("SYNCYARD_STATE_HOME", stateHome)
	// Ambient SYNCYARD_HOME carries the extension's checkout
	// meaning in the parent process.
	t.Setenv("SYNCYARD_HOME", checkout)
	calls := captureChildren(t, 0)
	code, _, _ := run(t, "progress", "job-1")
	if code != 0 || len(*calls) != 1 {
		t.Fatalf("progress: code=%d calls=%d", code, len(*calls))
	}
	call := (*calls)[0]
	// The child must see the state home explicitly in its
	// environment under BOTH names the Rust tools read, so
	// the ambiguous ambient SYNCYARD_HOME cannot leak through.
	if call.envState != stateHome {
		t.Errorf("child SYNCYARD_STATE_HOME = %q, want %q", call.envState, stateHome)
	}
	if call.envHome != stateHome {
		t.Errorf("child SYNCYARD_HOME = %q, want the state home %q (Rust tools read SYNCYARD_HOME as state home)", call.envHome, stateHome)
	}
	// And the parent's environment is restored afterwards.
	if got := getenvAfterForward(); got != checkout {
		t.Errorf("parent SYNCYARD_HOME = %q after forward, want restored %q", got, checkout)
	}
}

func getenvAfterForward() string {
	return os.Getenv("SYNCYARD_HOME")
}
