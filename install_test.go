package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- fix 8: install / upgrade ----------

var releasePayloads = map[string]string{
	"syncyard-daemon": "#!/bin/sh\necho daemon\n",
	"syncyard-cli":    "#!/bin/sh\necho cli\n",
	"syncyard-agent":  "#!/bin/sh\necho agent\n",
	"syncyard-tray":   "#!/bin/sh\necho tray\n",
}

type manifestEntry struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Sha256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// newReleaseServer serves one fake Syncyard release in the
// real v0.4.0 layout: a MANIFEST.json with rust_binaries
// entries and flat assets named <component>-linux-x86_64.
// corrupt names a component whose manifest sha256 is wrong.
func newReleaseServer(t *testing.T, ver, corrupt string) *httptest.Server {
	t.Helper()
	var entries []manifestEntry
	for _, name := range []string{"syncyard-daemon", "syncyard-cli", "syncyard-agent", "syncyard-tray"} {
		payload := releasePayloads[name]
		sum := sha256.Sum256([]byte(payload))
		sha := hex.EncodeToString(sum[:])
		if name == corrupt {
			sha = strings.Repeat("0", 64)
		}
		entries = append(entries, manifestEntry{
			Name:   name,
			Target: "linux-x86_64",
			Path:   "bin/linux-x86_64/" + name,
			Sha256: sha,
			Bytes:  len(payload),
		})
	}
	manifest, err := json.Marshal(map[string]any{
		"train":         ver,
		"rust_binaries": entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v"+ver+"/MANIFEST.json" {
			// Asset request: /v<ver>/<component>-linux-x86_64
			for name, payload := range releasePayloads {
				if strings.HasSuffix(r.URL.Path, "/"+name+"-linux-x86_64") {
					fmt.Fprint(w, payload)
					return
				}
			}
			http.NotFound(w, r)
			return
		}
		w.Write(manifest)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// installEnv points the installer at the fake release server
// and a fresh install root, with a fake gatekeeper so the
// post-install doctor run can probe successfully.
func installEnv(t *testing.T, baseURL string) string {
	t.Helper()
	cleanEnv(t)
	fakeGK(t)
	captureChildren(t, 0) // doctor's probes succeed
	root := filepath.Join(t.TempDir(), "install-root")
	t.Setenv("SYNCYARD_RELEASE_BASE_URL", baseURL)
	t.Setenv("SYNCYARD_INSTALL_ROOT", root)
	return root
}

func TestInstallHappyPath(t *testing.T) {
	srv := newReleaseServer(t, "0.4.0", "")
	root := installEnv(t, srv.URL)
	code, out, errOut := run(t, "install", "--version", "0.4.0", "--agent-id", "test-host")
	if code != 0 {
		t.Fatalf("install: code=%d out=%q err=%q", code, out, errOut)
	}
	for name, payload := range releasePayloads {
		p := filepath.Join(root, "bin", name)
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("placed %s: %v", name, err)
		}
		if string(b) != payload {
			t.Errorf("%s content = %q, want the verified release payload", name, b)
		}
		if st, _ := os.Stat(p); st != nil && st.Mode()&0o111 == 0 {
			t.Errorf("%s is not executable (mode %v)", name, st.Mode())
		}
	}
	// Registration material pins the explicit agent id and
	// never derives one from $HOSTNAME.
	reg, err := os.ReadFile(filepath.Join(root, "registration", "service-registration.txt"))
	if err != nil {
		t.Fatalf("registration material: %v", err)
	}
	if !strings.Contains(string(reg), "--agent-id test-host") {
		t.Errorf("registration material does not pin the agent id:\n%s", reg)
	}
	if strings.Contains(string(reg), "HOSTNAME") {
		t.Errorf("registration material must not reference HOSTNAME:\n%s", reg)
	}
	if !strings.Contains(out, "Next action") {
		t.Errorf("install must print the service-registration step as an explicit next action:\n%s", out)
	}
	if !strings.Contains(out, "doctor verdict: OK") {
		t.Errorf("install must run doctor after placement and report its verdict:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, "install.json")); err != nil {
		t.Errorf("install record: %v", err)
	}
}

func TestInstallManifestMismatchRefusesAndPlacesNothing(t *testing.T) {
	srv := newReleaseServer(t, "0.4.0", "syncyard-cli")
	root := installEnv(t, srv.URL)
	code, _, errOut := run(t, "install", "--version", "0.4.0")
	if code == 0 {
		t.Fatal("install with a manifest hash mismatch: exit 0, want refusal")
	}
	if !strings.Contains(errOut, "mismatch") {
		t.Fatalf("err = %q, want a SHA-256 mismatch refusal", errOut)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("install root %s exists after a refused install; nothing may be placed", root)
	}
}

func TestUpgradeUnknownVersionRefuses(t *testing.T) {
	srv := newReleaseServer(t, "0.4.0", "")
	root := installEnv(t, srv.URL)
	code, _, errOut := run(t, "upgrade", "--version", "9.9.9")
	if code == 0 {
		t.Fatal("upgrade to an unknown version: exit 0, want refusal")
	}
	if !strings.Contains(errOut, "9.9.9") {
		t.Fatalf("err = %q, want the refused version named", errOut)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("install root exists after a refused upgrade; nothing may be placed")
	}
}

func TestUpgradeRequiresVersion(t *testing.T) {
	srv := newReleaseServer(t, "0.4.0", "")
	installEnv(t, srv.URL)
	code, _, errOut := run(t, "upgrade")
	if code != 2 || !strings.Contains(errOut, "usage:") {
		t.Fatalf("upgrade without --version: code=%d err=%q, want usage error exit 2", code, errOut)
	}
}

func TestInstallDefaultsToOwnVersion(t *testing.T) {
	srv := newReleaseServer(t, version, "")
	root := installEnv(t, srv.URL)
	code, _, errOut := run(t, "install")
	if code != 0 {
		t.Fatalf("install (default version): code=%d err=%q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(root, "bin", "syncyard-cli")); err != nil {
		t.Fatalf("default-version install placed nothing: %v", err)
	}
}

func TestInstallRefusesPlatformWithoutReleaseAssets(t *testing.T) {
	srv := newReleaseServer(t, "0.4.0", "")
	root := installEnv(t, srv.URL)
	// The real v0.4.0 release carries Linux assets only; on
	// darwin/arm64 the verb must refuse with that fact.
	oldOS, oldArch := installGOOS, installGOARCH
	installGOOS, installGOARCH = "darwin", "arm64"
	t.Cleanup(func() { installGOOS, installGOARCH = oldOS, oldArch })
	code, _, errOut := run(t, "install", "--version", "0.4.0")
	if code == 0 {
		t.Fatal("install on a platform with no release assets: exit 0, want refusal")
	}
	if !strings.Contains(errOut, "darwin-aarch64") || !strings.Contains(errOut, "linux-x86_64") {
		t.Fatalf("err = %q, want the platform named with the release's actual targets", errOut)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("install root exists after a refused install; nothing may be placed")
	}
}

func TestInstallIdempotentRerun(t *testing.T) {
	srv := newReleaseServer(t, "0.4.0", "")
	root := installEnv(t, srv.URL)
	if code, _, errOut := run(t, "install", "--version", "0.4.0", "--agent-id", "test-host"); code != 0 {
		t.Fatalf("first install: code=%d err=%q", code, errOut)
	}
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	var mtimes []time.Time
	for name := range releasePayloads {
		p := filepath.Join(root, "bin", name)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
		mtimes = append(mtimes, old)
	}
	code, out, errOut := run(t, "install", "--version", "0.4.0", "--agent-id", "test-host")
	if code != 0 {
		t.Fatalf("re-install: code=%d out=%q err=%q", code, out, errOut)
	}
	i := 0
	for name := range releasePayloads {
		st, err := os.Stat(filepath.Join(root, "bin", name))
		if err != nil {
			t.Fatal(err)
		}
		if !st.ModTime().Equal(mtimes[i]) {
			t.Errorf("%s was rewritten on an idempotent re-run (mtime %v, want unchanged %v)", name, st.ModTime(), mtimes[i])
		}
		i++
	}
}
