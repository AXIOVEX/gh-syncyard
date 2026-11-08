package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// The parity surface as data: the same structure backs
// `gh syncyard parity`, `gh syncyard schema`, and the help
// text's counts, so the three can never disagree.

type parityRow struct {
	Verb   string `json:"verb"`
	Engine string `json:"engine"` // python | rust-cli | rust-socket | extension
	Note   string `json:"note,omitempty"`
}

type parityData struct {
	ExtensionVersion string      `json:"extension_version"`
	EngineVersion    string      `json:"engine_version"` // syncyard repo VERSION (one train)
	ContractVersion  int         `json:"contract_version"`
	Verbs            []parityRow `json:"verbs"`
	NotYetInRust     []parityRow `json:"rust_not_yet_verbs"`
	DaemonInternal   []parityRow `json:"daemon_internal_not_user_verbs"`
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func buildParity() parityData {
	d := parityData{
		ExtensionVersion: version,
		EngineVersion:    "0.4.0",
		ContractVersion:  1,
	}
	notes := map[string]string{
		"submit":           "spec 016 auto routing: --lane auto + --needs-cpu/--needs-ram-mb/--needs-storage-mb/--needs-gpu/--venue hints forward verbatim",
		"resume":           "also withdraws one job's checkpoint hold with --job (spec 013 FR-009)",
		"profile":          "spec 016 lane profiles; `profile suggest` drafts from a capability snapshot",
		"probe-host":       "spec 016 FR-005 capability snapshot for auto routing",
		"progress":         "spec 034 live progress view (daemon RAM; degrades honestly in fallback)",
		"kill-switch":      "spec 037 FIX-08 repair kill switch (daemon kill_switch verb)",
		"migrate-history":  "spec 038: import a Python gatekeeper dir with SHA-256 manifest",
		"dispatch":         "single-job dispatch lift (daemon-side transport)",
		"control-outcomes": "spec 037: published outcomes for tray control commands",
		"control-command":  "spec 037: deliver one signed tray command file to the local daemon",
	}
	for _, v := range sortedKeys(pythonVerbs) {
		d.Verbs = append(d.Verbs, parityRow{v, "python", notes[v]})
	}
	for _, v := range sortedKeys(rustCLIVerbs) {
		d.Verbs = append(d.Verbs, parityRow{v, "rust-cli", notes[v]})
	}
	for _, v := range sortedKeys(socketVerbs) {
		d.Verbs = append(d.Verbs, parityRow{v, "rust-socket", notes[v]})
	}
	for _, v := range []string{"version", "help", "doctor", "parity", "schema"} {
		d.Verbs = append(d.Verbs, parityRow{v, "extension", ""})
	}
	sort.Slice(d.Verbs, func(i, j int) bool { return d.Verbs[i].Verb < d.Verbs[j].Verb })

	for _, v := range sortedKeys(mapBool(rustNotYetVerbs)) {
		d.NotYetInRust = append(d.NotYetInRust, parityRow{v, "python (rust: not yet)", rustNotYetVerbs[v]})
	}
	for _, v := range sortedKeys(mapBool(daemonInternalVerbs)) {
		d.DaemonInternal = append(d.DaemonInternal, parityRow{v, "rust-daemon (internal)", daemonInternalVerbs[v]})
	}
	return d
}

func mapBool(m map[string]string) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// parityCmd prints the parity table (or JSON with --json).
func parityCmd(rest []string, stdout, stderr io.Writer) int {
	d := buildParity()
	for _, a := range rest {
		if a == "--json" {
			b, err := json.MarshalIndent(d, "", "  ")
			if err != nil {
				fmt.Fprintf(stderr, "gh-syncyard: %v\n", err)
				return 1
			}
			fmt.Fprintln(stdout, string(b))
			return 0
		}
	}
	fmt.Fprintf(stdout, "gh-syncyard %s — verb parity with the Syncyard engines (one version train: engine %s, contract %d)\n\n", d.ExtensionVersion, d.EngineVersion, d.ContractVersion)
	fmt.Fprintln(stdout, "verb              engine        notes")
	for _, r := range d.Verbs {
		fmt.Fprintf(stdout, "%-17s %-13s %s\n", r.Verb, r.Engine, r.Note)
	}
	fmt.Fprintln(stdout, "\nServed by the Python gatekeeper, not yet in the Rust CLI (schema not_yet_verbs):")
	for _, r := range d.NotYetInRust {
		fmt.Fprintf(stdout, "%-17s %-22s %s\n", r.Verb, r.Engine, r.Note)
	}
	fmt.Fprintln(stdout, "\nDaemon-internal socket verbs (not user verbs, by design):")
	for _, r := range d.DaemonInternal {
		fmt.Fprintf(stdout, "%-17s %-22s %s\n", r.Verb, r.Engine, r.Note)
	}
	return 0
}

// schemaCmd emits the parity surface as JSON: the extension's
// counterpart of the Rust CLI's `schema` verb.
func schemaCmd(stdout io.Writer) int {
	b, err := json.MarshalIndent(buildParity(), "", "  ")
	if err != nil {
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	return 0
}
