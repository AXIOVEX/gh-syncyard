package main

import (
	"os"
	"strings"
	"testing"
)

// ---------- fix 1: gatekeeper discovery ----------

func TestGatekeeperCandidatesIncludeMiniServerGovernedPath(t *testing.T) {
	cleanEnv(t)
	var paths []string
	for _, c := range gatekeeperCandidates() {
		paths = append(paths, c.path)
	}
	const governed = "/var/lib/syncyard/gatekeeper/gatekeeper/gk.py"
	const legacy = "/var/lib/syncyard/gatekeeper/gk.py"
	gi, li := -1, -1
	for i, p := range paths {
		if p == governed {
			gi = i
		}
		if p == legacy {
			li = i
		}
	}
	if gi < 0 {
		t.Fatalf("candidates %v missing governed system-install path %q", paths, governed)
	}
	if li < 0 {
		t.Fatalf("candidates %v missing legacy system-install path %q (kept for transition)", paths, legacy)
	}
	if gi > li {
		t.Fatalf("governed path must be tried before the legacy path: %v", paths)
	}
}

// ---------- fix 3: native verb argument strictness ----------

func TestNativeVerbsRejectUnexpectedArguments(t *testing.T) {
	cases := [][]string{
		{"version", "extra"},
		{"--version", "extra"},
		{"schema", "extra"},
		{"doctor", "extra"},
		{"help", "extra"},
		{"parity", "garbage"},
		{"parity", "garbage", "--json"},
		{"parity", "--json", "extra"},
		{"parity", "--json", "--json"},
	}
	for _, args := range cases {
		cleanEnv(t)
		code, _, errOut := run(t, args...)
		if code != 2 || !strings.Contains(errOut, "usage:") {
			t.Errorf("run %v: code=%d err=%q, want exit 2 with a usage error", args, code, errOut)
		}
	}
}

// ---------- fix 2: socket reply validation ----------

func TestSocketReplyValidation(t *testing.T) {
	cases := []struct {
		name     string
		reply    string
		wantCode int // -1 means "any nonzero"
	}{
		{"malformed json", `not json at all`, -1},
		{"missing ok field", `{"outcomes":[]}`, -1},
		{"non-object reply", `true`, -1},
		{"ok not a bool", `{"ok":"yes"}`, -1},
		{"ok true", `{"ok":true,"outcomes":[]}`, 0},
		{"ok false", `{"ok":false,"error":"no"}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleanEnv(t)
			t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
			captureSocket(t, tc.reply, nil)
			code, _, errOut := run(t, "control-outcomes")
			if tc.wantCode == -1 {
				if code == 0 {
					t.Fatalf("reply %q: exit 0, want nonzero (invalid replies must not pass silently)", tc.reply)
				}
				if !strings.Contains(errOut, "invalid daemon reply") {
					t.Fatalf("reply %q: err=%q, want an 'invalid daemon reply' error", tc.reply, errOut)
				}
			} else if code != tc.wantCode {
				t.Fatalf("reply %q: code=%d, want %d", tc.reply, code, tc.wantCode)
			}
		})
	}
}

// ---------- fix 7: documentation completeness ----------

func TestDocsCoverSubmitArgsAndNewerVerbs(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"artifact-delete", "staging-run", "--image", "--mounts", "install", "upgrade", "SYNCYARD_STATE_HOME"} {
		if !strings.Contains(string(readme), want) {
			t.Errorf("README omits %q", want)
		}
		if !strings.Contains(helpText, want) {
			t.Errorf("help omits %q", want)
		}
	}
	// The parity note for submit names the docker-transport
	// arguments too (spec 031 FR-031-03).
	for _, r := range buildParity().Verbs {
		if r.Verb == "submit" {
			if !strings.Contains(r.Note, "--image") || !strings.Contains(r.Note, "--mounts") {
				t.Errorf("parity submit note omits --image/--mounts: %q", r.Note)
			}
		}
	}
}

func TestControlOutcomesRejectsArguments(t *testing.T) {
	cleanEnv(t)
	t.Setenv("SYNCYARD_STATE_HOME", t.TempDir())
	captureSocket(t, `{"ok":true}`, nil)
	code, _, errOut := run(t, "control-outcomes", "garbage")
	if code != 2 || !strings.Contains(errOut, "usage:") {
		t.Fatalf("control-outcomes garbage: code=%d err=%q, want exit 2 with a usage error", code, errOut)
	}
}
