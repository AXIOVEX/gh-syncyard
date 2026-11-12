package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Spec 031 amendment: `gh syncyard install` /
// `gh syncyard upgrade`.
//
// The verb fetches the versioned Syncyard release bundle
// for the host OS/arch from electrohire/syncyard releases,
// verifies every component against the release SHA-256
// manifest (MANIFEST.json, the real v0.4.0 layout) BEFORE
// placing anything, and refuses — placing nothing — on any
// mismatch, unknown version, or missing platform asset.
// Placement is idempotent, never elevates, and service
// registration is handed to the per-OS supervisor as an
// explicit printed next action with the agent id pinned.
//
// Test seams: the release base URL and the install root are
// environment-overridable ($SYNCYARD_RELEASE_BASE_URL,
// $SYNCYARD_INSTALL_ROOT), and the platform is a package
// variable, so tests run entirely against a local httptest
// server and temp dirs.

const defaultReleaseBaseURL = "https://github.com/electrohire/syncyard/releases/download"

var (
	installHTTPClient = &http.Client{Timeout: 60 * time.Second}
	installGOOS       = runtime.GOOS
	installGOARCH     = runtime.GOARCH
	// installDoctor runs after placement; a variable so the
	// production path and tests share the real doctor.
	installDoctor = doctor
)

func releaseBaseURL() string {
	if v := os.Getenv("SYNCYARD_RELEASE_BASE_URL"); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return defaultReleaseBaseURL
}

// installRoot resolves where the stack is placed:
// $SYNCYARD_INSTALL_ROOT, else a per-user data dir (never a
// system path, so install never needs elevation).
func installRoot() string {
	if v := os.Getenv("SYNCYARD_INSTALL_ROOT"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if cfg, err := os.UserConfigDir(); err == nil {
			return filepath.Join(cfg, "Syncyard")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".local", "share", "syncyard")
	}
	return filepath.Join(home, ".local", "share", "syncyard")
}

// releaseTarget maps this host to the manifest's target
// naming (linux-x86_64 / linux-aarch64 in the real release).
func releaseTarget() string {
	arch := installGOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}
	return installGOOS + "-" + arch
}

type releaseEntry struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Asset  string `json:"asset,omitempty"`
	Sha256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type releaseManifest struct {
	Train        string         `json:"train"`
	Version      string         `json:"version,omitempty"`
	RustBinaries []releaseEntry `json:"rust_binaries"`
}

func (e releaseEntry) assetName() string {
	if e.Asset != "" {
		return e.Asset
	}
	return e.Name + "-" + e.Target
}

type installOptions struct {
	version string
	agentID string
	root    string
}

var agentIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func parseInstallArgs(upgrade bool, rest []string) (installOptions, error) {
	verb := "install"
	if upgrade {
		verb = "upgrade"
	}
	var o installOptions
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		name, val := arg, ""
		hasVal := false
		if eq := strings.Index(arg, "="); eq >= 0 && strings.HasPrefix(arg, "--") {
			name, val, hasVal = arg[:eq], arg[eq+1:], true
		}
		take := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 >= len(rest) {
				return "", fmt.Errorf("%s requires a value", name)
			}
			i++
			return rest[i], nil
		}
		var err error
		switch name {
		case "--version":
			o.version, err = take()
		case "--agent-id":
			o.agentID, err = take()
		case "--install-root":
			o.root, err = take()
		default:
			return o, fmt.Errorf("unexpected argument %q", arg)
		}
		if err != nil {
			return o, err
		}
	}
	if o.version == "" {
		if upgrade {
			return o, fmt.Errorf("gh syncyard upgrade --version X (a named target version is required)")
		}
		o.version = version // the extension's own version (one train)
	}
	o.version = strings.TrimPrefix(o.version, "v")
	if o.agentID != "" && !agentIDRe.MatchString(o.agentID) {
		return o, fmt.Errorf("--agent-id %q is not a valid agent id (letters, digits, '.', '_', '-')", o.agentID)
	}
	if o.root == "" {
		o.root = installRoot()
	}
	_ = verb
	return o, nil
}

func fetchRelease(url string, limit int64) (int, []byte, error) {
	resp, err := installHTTPClient.Get(url)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if int64(len(body)) > limit {
		return resp.StatusCode, nil, fmt.Errorf("%s: response exceeds %d bytes", url, limit)
	}
	return resp.StatusCode, body, nil
}

func installOrUpgrade(upgrade bool, rest []string, stdout, stderr io.Writer) int {
	verb := "install"
	if upgrade {
		verb = "upgrade"
	}
	o, err := parseInstallArgs(upgrade, rest)
	if err != nil {
		return usageError(stderr, "gh syncyard %s [--version X] [--agent-id ID] [--install-root DIR] — %v", verb, err)
	}
	target := releaseTarget()
	base := releaseBaseURL()

	// 1. Manifest first. A 404 here is an unknown version.
	mURL := fmt.Sprintf("%s/v%s/MANIFEST.json", base, o.version)
	status, mBody, err := fetchRelease(mURL, 4<<20)
	if err != nil {
		fmt.Fprintf(stderr, "gh-syncyard: %s: fetch manifest for v%s: %v\n", verb, o.version, err)
		return 1
	}
	if status == http.StatusNotFound {
		fmt.Fprintf(stderr, "gh-syncyard: %s: unknown version %s: no Syncyard release v%s was found at %s\n", verb, o.version, o.version, base)
		return 2
	}
	if status != http.StatusOK {
		fmt.Fprintf(stderr, "gh-syncyard: %s: manifest for v%s: HTTP %d\n", verb, o.version, status)
		return 1
	}
	var manifest releaseManifest
	if err := json.Unmarshal(mBody, &manifest); err != nil {
		fmt.Fprintf(stderr, "gh-syncyard: %s: release manifest for v%s is not valid JSON: %v\n", verb, o.version, err)
		return 1
	}
	train := manifest.Train
	if train == "" {
		train = manifest.Version
	}
	if train != o.version {
		fmt.Fprintf(stderr, "gh-syncyard: %s: release manifest train %q does not match the requested version %s; refusing\n", verb, train, o.version)
		return 1
	}

	// 2. Select this platform's components. The core set is
	// required; the tray is placed where the release has one.
	var selected []releaseEntry
	available := map[string]bool{}
	present := map[string]bool{}
	for _, e := range manifest.RustBinaries {
		available[e.Target] = true
		if e.Target == target {
			selected = append(selected, e)
			present[e.Name] = true
		}
	}
	for _, core := range []string{"syncyard-daemon", "syncyard-cli", "syncyard-agent"} {
		if !present[core] {
			var targets []string
			for t := range available {
				targets = append(targets, t)
			}
			sort.Strings(targets)
			fmt.Fprintf(stderr, "gh-syncyard: %s: release v%s has no %s asset for platform %s (release targets: %s); refusing, nothing placed\n",
				verb, o.version, core, target, strings.Join(targets, ", "))
			return 2
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Name < selected[j].Name })

	// 3. Download and verify EVERY component before placing
	// anything. A mismatch anywhere refuses the whole
	// install with nothing on disk.
	type verified struct {
		entry releaseEntry
		data  []byte
	}
	var bundle []verified
	for _, e := range selected {
		aURL := fmt.Sprintf("%s/v%s/%s", base, o.version, e.assetName())
		status, data, err := fetchRelease(aURL, 64<<20)
		if err != nil {
			fmt.Fprintf(stderr, "gh-syncyard: %s: fetch %s: %v\n", verb, e.assetName(), err)
			return 1
		}
		if status == http.StatusNotFound {
			fmt.Fprintf(stderr, "gh-syncyard: %s: release v%s lists %s but the asset was not found (HTTP 404); refusing, nothing placed\n", verb, o.version, e.assetName())
			return 2
		}
		if status != http.StatusOK {
			fmt.Fprintf(stderr, "gh-syncyard: %s: fetch %s: HTTP %d\n", verb, e.assetName(), status)
			return 1
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if e.Bytes > 0 && int64(len(data)) != e.Bytes {
			fmt.Fprintf(stderr, "gh-syncyard: %s: size mismatch for %s: manifest %d bytes, downloaded %d; refusing, nothing placed\n", verb, e.assetName(), e.Bytes, len(data))
			return 1
		}
		if !strings.EqualFold(got, e.Sha256) {
			fmt.Fprintf(stderr, "gh-syncyard: %s: SHA-256 mismatch for %s: manifest %s, downloaded %s; refusing, nothing placed\n", verb, e.assetName(), e.Sha256, got)
			return 1
		}
		bundle = append(bundle, verified{e, data})
	}

	// 4. Resolve the agent id: the flag, else the previous
	// install record, else a stable root-derived id. Never
	// $HOSTNAME: supervised environments (launchd) may not
	// provide it, which is how agents registered as "host".
	agentID := o.agentID
	if agentID == "" {
		if rec, err := readInstallRecord(o.root); err == nil && rec.AgentID != "" {
			agentID = rec.AgentID
		} else {
			abs, _ := filepath.Abs(o.root)
			sum := sha256.Sum256([]byte(abs))
			agentID = "agent-" + hex.EncodeToString(sum[:])[:12]
		}
	}

	// 5. Place. Files whose bytes already match the manifest
	// are left untouched (idempotent re-runs).
	binDir := filepath.Join(o.root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "gh-syncyard: %s: %v\n", verb, err)
		return 1
	}
	for _, v := range bundle {
		name := v.entry.Name
		if installGOOS == "windows" {
			name += ".exe"
		}
		dest := filepath.Join(binDir, name)
		if existing, err := os.ReadFile(dest); err == nil {
			sum := sha256.Sum256(existing)
			if hex.EncodeToString(sum[:]) == strings.ToLower(v.entry.Sha256) {
				fmt.Fprintf(stdout, "%s: unchanged %s\n", v.entry.Name, dest)
				continue
			}
		}
		if err := writeFileAtomic(binDir, name, v.data, 0o755); err != nil {
			fmt.Fprintf(stderr, "gh-syncyard: %s: place %s: %v\n", verb, v.entry.Name, err)
			return 1
		}
		fmt.Fprintf(stdout, "%s: placed %s (sha256 %s)\n", v.entry.Name, dest, v.entry.Sha256[:12])
	}

	// Keep the verified manifest and the registration
	// material with the install.
	relDir := filepath.Join(o.root, "releases", "v"+o.version)
	if err := os.MkdirAll(relDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "gh-syncyard: %s: %v\n", verb, err)
		return 1
	}
	_ = writeFileAtomic(relDir, "MANIFEST.json", mBody, 0o644)

	regDir := filepath.Join(o.root, "registration")
	next := writeRegistration(regDir, o, agentID, binDir)
	if next == "" {
		fmt.Fprintf(stderr, "gh-syncyard: %s: write registration material: failed\n", verb)
		return 1
	}

	components := map[string]string{}
	for _, v := range bundle {
		components[v.entry.Name] = v.entry.Sha256
	}
	rec := installRecord{
		Version:    o.version,
		Target:     target,
		AgentID:    agentID,
		Components: components,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if b, err := json.MarshalIndent(rec, "", "  "); err == nil {
		_ = writeFileAtomic(o.root, "install.json", append(b, '\n'), 0o644)
	}

	fmt.Fprintf(stdout, "installed Syncyard %s for %s into %s\n", o.version, target, o.root)
	fmt.Fprintf(stdout, "service registration: NOT performed (this verb never elevates). Next action: %s\n", next)
	fmt.Fprintf(stdout, "registration material (agent id pinned: %s): %s\n", agentID, regDir)

	// 6. Doctor verifies the result.
	var dbuf strings.Builder
	dcode := installDoctor(&dbuf)
	fmt.Fprint(stdout, dbuf.String())
	if dcode != 0 {
		fmt.Fprintf(stderr, "gh-syncyard: doctor verdict: FAIL (exit %d) after placing Syncyard %s\n", dcode, o.version)
		return dcode
	}
	fmt.Fprintln(stdout, "doctor verdict: OK")
	return 0
}

type installRecord struct {
	Version    string            `json:"version"`
	Target     string            `json:"target"`
	AgentID    string            `json:"agent_id"`
	Components map[string]string `json:"components"`
	UpdatedAt  string            `json:"updated_at"`
}

func readInstallRecord(root string) (installRecord, error) {
	var rec installRecord
	b, err := os.ReadFile(filepath.Join(root, "install.json"))
	if err != nil {
		return rec, err
	}
	err = json.Unmarshal(b, &rec)
	return rec, err
}

// writeFileAtomic writes via a temp file in the same
// directory and renames, so a destination is never partial.
func writeFileAtomic(dir, name string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, name))
}

// writeRegistration writes the per-OS service-registration
// material (never registering anything itself) and returns
// the explicit next action to print, or "" on failure.
func writeRegistration(regDir string, o installOptions, agentID, binDir string) string {
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		return ""
	}
	stateHome := os.Getenv("SYNCYARD_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(o.root, "state")
	}
	daemon := filepath.Join(binDir, "syncyard-daemon")
	agent := filepath.Join(binDir, "syncyard-agent")
	var next string
	var b strings.Builder
	b.WriteString("Syncyard service registration (not performed by gh syncyard install)\n")
	b.WriteString("Agent ID (pinned): " + agentID + "\n\n")
	switch installGOOS {
	case "linux":
		unitDir := filepath.Join(regDir, "systemd-user")
		if err := os.MkdirAll(unitDir, 0o755); err != nil {
			return ""
		}
		daemonUnit := "[Unit]\nDescription=Syncyard daemon\nAfter=network-online.target\n\n" +
			"[Service]\nExecStart=" + daemon + " serve --home " + stateHome + " --allow-no-transport\nRestart=on-failure\n\n" +
			"[Install]\nWantedBy=default.target\n"
		agentUnit := "[Unit]\nDescription=Syncyard agent\nAfter=syncyard-daemon.service\n\n" +
			"[Service]\nExecStart=" + agent + " --daemon-home " + stateHome + " --install-path " + binDir + " --agent-id " + agentID + "\nRestart=on-failure\n\n" +
			"[Install]\nWantedBy=default.target\n"
		if err := writeFileAtomic(unitDir, "syncyard-daemon.service", []byte(daemonUnit), 0o644); err != nil {
			return ""
		}
		if err := writeFileAtomic(unitDir, "syncyard-agent.service", []byte(agentUnit), 0o644); err != nil {
			return ""
		}
		next = fmt.Sprintf("systemctl --user link %s %s && systemctl --user enable --now syncyard-daemon.service syncyard-agent.service (review the daemon transport flags in the unit first if this host serves jobs)",
			filepath.Join(unitDir, "syncyard-daemon.service"), filepath.Join(unitDir, "syncyard-agent.service"))
		b.WriteString("systemd user units are in systemd-user/. Register with:\n  " + next + "\n")
		b.WriteString("Agent command (from syncyard-agent.service): " + agent + " --daemon-home " + stateHome + " --install-path " + binDir + " --agent-id " + agentID + "\n")
	case "darwin":
		next = fmt.Sprintf("install the LaunchAgent plists from this directory into ~/Library/LaunchAgents with agent id %s pinned, then: launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/org.syncyard.agent.plist", agentID)
		b.WriteString("Register the agent with launchd using the pinned agent id:\n  " + next + "\n")
		b.WriteString("Agent command: " + agent + " --daemon-home " + stateHome + " --install-path " + binDir + " --agent-id " + agentID + "\n")
	case "windows":
		next = "register the Syncyard tray and agent with Task Scheduler (run the tray installer as your user; no elevation is taken by this verb)"
		b.WriteString(next + "\n")
		b.WriteString("Agent command: " + agent + " --daemon-home " + stateHome + " --install-path " + binDir + " --agent-id " + agentID + "\n")
	default:
		next = "register syncyard-agent with your OS supervisor using the pinned agent id " + agentID
		b.WriteString("Agent command: " + agent + " --daemon-home " + stateHome + " --install-path " + binDir + " --agent-id " + agentID + "\n")
	}
	if err := writeFileAtomic(regDir, "service-registration.txt", []byte(b.String()), 0o644); err != nil {
		return ""
	}
	return next
}
