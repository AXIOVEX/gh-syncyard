package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractGKVerbs(t *testing.T) {
	src := []byte(`
p = sub.add_parser("submit")
p = sub.add_parser('status')
p = sub.add_parser("submit")
sub.add_parser( "sweep" )
`)
	got := extractGKVerbs(src)
	want := []string{"status", "submit", "sweep"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("verbs = %v, want %v", got, want)
	}
}

// writeEngineTree builds a minimal engine source layout:
// a gk.py, a syncyard-cli crate (src/lib.rs + Cargo.toml), a
// workspace Cargo.toml, and a syncyard-contract crate.
func writeEngineTree(t *testing.T) (gkPath, cliPath string) {
	t.Helper()
	root := t.TempDir()
	gkPath = filepath.Join(root, "gk.py")
	if err := os.WriteFile(gkPath, []byte("sub.add_parser(\"submit\")\nsub.add_parser(\"status\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cliDir := filepath.Join(root, "rust", "crates", "syncyard-cli")
	srcDir := filepath.Join(cliDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lib := `pub const VERBS: &[&str] = &[
    "submit",
    "status",
    "version",
    "schema",
];

pub const NOT_YET_VERBS: &[(&str, &str)] = &[
    (
        "sweep",
        "lands later",
    ),
    ("purge", "destructive; own design"),
];
`
	cliPath = filepath.Join(srcDir, "lib.rs")
	if err := os.WriteFile(cliPath, []byte(lib), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cliDir, "Cargo.toml"),
		[]byte("[package]\nname = \"syncyard-cli\"\nversion.workspace = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rust", "Cargo.toml"),
		[]byte("[workspace.package]\nversion = \"9.9.9\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	contractDir := filepath.Join(root, "rust", "crates", "syncyard-contract", "src")
	if err := os.MkdirAll(contractDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contractDir, "lib.rs"),
		[]byte("pub const CONTRACT_VERSION: u32 = 7;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return gkPath, cliPath
}

func TestGenerateCheckAndDrift(t *testing.T) {
	gkPath, cliPath := writeEngineTree(t)
	out := filepath.Join(t.TempDir(), "testdata")

	if err := execute(gkPath, cliPath, out, false); err != nil {
		t.Fatalf("generate: %v", err)
	}
	gkFixture, err := os.ReadFile(filepath.Join(out, "gk_verbs.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gkFixture) != "status\nsubmit\n" {
		t.Fatalf("gk fixture = %q", gkFixture)
	}
	schemaFixture, err := os.ReadFile(filepath.Join(out, "cli_schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"version": "9.9.9"`, `"contract_version": 7`, `"verb": "sweep"`, `"note": "destructive; own design"`} {
		if !strings.Contains(string(schemaFixture), want) {
			t.Errorf("schema fixture missing %s:\n%s", want, schemaFixture)
		}
	}
	// Check mode passes on fresh fixtures.
	if err := execute(gkPath, cliPath, out, true); err != nil {
		t.Fatalf("check on fresh fixtures: %v", err)
	}
	// A new verb in the engine source is drift: check fails.
	f, err := os.OpenFile(gkPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("sub.add_parser(\"newverb\")\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := execute(gkPath, cliPath, out, true); err == nil {
		t.Fatal("check must fail when the engine source drifts from the fixtures")
	}
	// Regenerating clears the drift.
	if err := execute(gkPath, cliPath, out, false); err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if err := execute(gkPath, cliPath, out, true); err != nil {
		t.Fatalf("check after regenerate: %v", err)
	}
}
