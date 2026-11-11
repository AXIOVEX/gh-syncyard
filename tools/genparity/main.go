// Command genparity regenerates the gh-syncyard parity
// fixtures (testdata/gk_verbs.txt and testdata/cli_schema.json)
// from the engines' own sources, so verb-surface drift is
// mechanical to refresh and CI can fail on it.
//
// Usage:
//
//	go run ./tools/genparity \
//	  --gk  /path/to/AXIOVEX/syncyard/gatekeeper/gk.py \
//	  --cli /path/to/syncyard/rust/crates/syncyard-cli/src/lib.rs \
//	  --out testdata [--check]
//
// --check writes nothing: it exits 1 and names the fixture
// when the checked-in fixture differs from a fresh extraction.
// The two source paths are explicit arguments so CI can pin
// the exact engine revisions it checks against.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func main() {
	gk := flag.String("gk", "", "path to gk.py (Python gatekeeper parser source)")
	cli := flag.String("cli", "", "path to syncyard-cli/src/lib.rs (Rust CLI schema source)")
	out := flag.String("out", "testdata", "fixture output directory")
	check := flag.Bool("check", false, "do not write; fail if a fixture differs from a fresh extraction")
	flag.Parse()
	if *gk == "" || *cli == "" {
		fmt.Fprintln(os.Stderr, "genparity: --gk and --cli are required")
		os.Exit(2)
	}
	if err := execute(*gk, *cli, *out, *check); err != nil {
		fmt.Fprintf(os.Stderr, "genparity: %v\n", err)
		os.Exit(1)
	}
}

func execute(gkPath, cliPath, outDir string, check bool) error {
	fixtures, err := buildFixtures(gkPath, cliPath)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(fixtures))
	for n := range fixtures {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(outDir, name)
		if check {
			got, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("check: read %s: %w", path, err)
			}
			if !bytes.Equal(got, fixtures[name]) {
				return fmt.Errorf("check: %s drifts from the engine sources; regenerate with genparity (no --check)", path)
			}
			continue
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, fixtures[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// buildFixtures extracts both fixture files' bytes from the
// two engine sources.
func buildFixtures(gkPath, cliPath string) (map[string][]byte, error) {
	gkSrc, err := os.ReadFile(gkPath)
	if err != nil {
		return nil, err
	}
	verbs := extractGKVerbs(gkSrc)
	if len(verbs) == 0 {
		return nil, fmt.Errorf("%s: no add_parser verbs extracted", gkPath)
	}
	cliSrc, err := os.ReadFile(cliPath)
	if err != nil {
		return nil, err
	}
	schema, err := extractCLISchema(cliSrc, cliPath)
	if err != nil {
		return nil, err
	}
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		"gk_verbs.txt":    []byte(strings.Join(verbs, "\n") + "\n"),
		"cli_schema.json": append(schemaJSON, '\n'),
	}, nil
}

var addParserRe = regexp.MustCompile(`add_parser\(\s*["']([^"']+)["']`)

// extractGKVerbs pulls every add_parser("...") name out of
// the gatekeeper's argparse setup.
func extractGKVerbs(src []byte) []string {
	seen := map[string]bool{}
	for _, m := range addParserRe.FindAllSubmatch(src, -1) {
		seen[string(m[1])] = true
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

var quotedRe = regexp.MustCompile(`"([^"]+)"`)

// constSegment returns the text of a `pub const NAME ... ];`
// block in Rust source.
func constSegment(src []byte, name string) (string, error) {
	start := strings.Index(string(src), "pub const "+name)
	if start < 0 {
		return "", fmt.Errorf("const %s not found in CLI source", name)
	}
	rest := string(src)[start:]
	end := strings.Index(rest, "];")
	if end < 0 {
		return "", fmt.Errorf("const %s is not terminated in CLI source", name)
	}
	return rest[:end], nil
}

type notYet struct {
	Verb string
	Note string
}

// extractCLISchema builds the schema fixture from the Rust
// CLI source: the VERBS and NOT_YET_VERBS constants in
// lib.rs, the crate version (Cargo.toml, resolving the
// workspace package version), and CONTRACT_VERSION from the
// syncyard-contract crate beside it.
func extractCLISchema(src []byte, cliPath string) (map[string]any, error) {
	verbsSeg, err := constSegment(src, "VERBS")
	if err != nil {
		return nil, err
	}
	var verbs []string
	for _, m := range quotedRe.FindAllStringSubmatch(verbsSeg, -1) {
		verbs = append(verbs, m[1])
	}
	if len(verbs) == 0 {
		return nil, fmt.Errorf("no VERBS extracted from CLI source")
	}
	nySeg, err := constSegment(src, "NOT_YET_VERBS")
	if err != nil {
		return nil, err
	}
	strs := quotedRe.FindAllStringSubmatch(nySeg, -1)
	if len(strs)%2 != 0 {
		return nil, fmt.Errorf("NOT_YET_VERBS has an odd number of strings (%d); want verb/note pairs", len(strs))
	}
	var notYet []map[string]string
	for i := 0; i < len(strs); i += 2 {
		notYet = append(notYet, map[string]string{"note": strs[i+1][1], "verb": strs[i][1]})
	}
	version, err := cliVersion(cliPath)
	if err != nil {
		return nil, err
	}
	contract, err := contractVersion(cliPath)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"contract_version": contract,
		"not_yet_verbs":    notYet,
		"verbs":            verbs,
		"version":          version,
	}, nil
}

var versionRe = regexp.MustCompile(`(?m)^version\s*=\s*"([^"]+)"`)

// cliVersion reads the syncyard-cli version from its crate
// Cargo.toml, following version.workspace = true to the
// workspace root Cargo.toml's [workspace.package] version.
func cliVersion(cliPath string) (string, error) {
	srcDir := filepath.Dir(cliPath) // .../syncyard-cli/src
	cargoPath := filepath.Join(srcDir, "..", "Cargo.toml")
	b, err := os.ReadFile(cargoPath)
	if err != nil {
		return "", fmt.Errorf("read crate Cargo.toml: %w", err)
	}
	if m := versionRe.FindSubmatch(b); m != nil {
		return string(m[1]), nil
	}
	if !strings.Contains(string(b), "version.workspace") {
		return "", fmt.Errorf("%s: no version found", cargoPath)
	}
	rootPath := filepath.Join(srcDir, "..", "..", "..", "Cargo.toml")
	rb, err := os.ReadFile(rootPath)
	if err != nil {
		return "", fmt.Errorf("read workspace Cargo.toml: %w", err)
	}
	idx := strings.Index(string(rb), "[workspace.package]")
	if idx < 0 {
		return "", fmt.Errorf("%s: no [workspace.package] section", rootPath)
	}
	if m := versionRe.FindSubmatch(rb[idx:]); m != nil {
		return string(m[1]), nil
	}
	return "", fmt.Errorf("%s: no [workspace.package] version", rootPath)
}

var contractRe = regexp.MustCompile(`CONTRACT_VERSION:\s*u32\s*=\s*(\d+)`)

// contractVersion reads CONTRACT_VERSION from the
// syncyard-contract crate that sits beside syncyard-cli in
// the Rust workspace.
func contractVersion(cliPath string) (int, error) {
	srcDir := filepath.Dir(cliPath)
	p := filepath.Join(srcDir, "..", "..", "syncyard-contract", "src", "lib.rs")
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, fmt.Errorf("read syncyard-contract source: %w", err)
	}
	m := contractRe.FindSubmatch(b)
	if m == nil {
		return 0, fmt.Errorf("%s: CONTRACT_VERSION not found", p)
	}
	return strconv.Atoi(string(m[1]))
}
