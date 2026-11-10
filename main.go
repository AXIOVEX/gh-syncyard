// Command gh-syncyard is the Syncyard extension for the GitHub CLI.
//
// It is a front door over the Syncyard gatekeeper engines. Two
// engines exist during the Rust migration (spec 038's one-move
// cutover has not happened yet):
//
//   - the Python gatekeeper (gatekeeper/gk.py, AXIOVEX/syncyard)
//     is the live engine. Every verb it implements is forwarded to
//     it verbatim, streaming stdin, stdout, stderr and the exit
//     code unchanged — the 0.1.0 pattern, kept.
//   - the Rust engine (syncyard-cli + syncyard-daemon, spec 038)
//     owns the verbs that exist only there: progress (spec 034),
//     kill-switch (spec 037 FIX-08), ledger, dispatch,
//     migrate-history, flush, and the daemon-socket control
//     surface (control_command / control_outcomes, spec 037).
//     Those delegate to the syncyard-cli binary or, for the
//     control surface, speak the daemon's newline-JSON Unix
//     socket protocol directly — the same channel the tray uses.
//
// Verbs both engines implement (submit, status, snapshot, outbox,
// pause, stop, start, hold, resume, cancel) route to the Python
// gatekeeper until the cutover; the Rust CLI's own `schema` verb
// is the parity target for this routing table (see parity.go and
// the drift tests in main_test.go).
//
// Gatekeeper resolution order (first hit wins):
//  1. $SYNCYARD_GK            — explicit path to gk.py
//  2. $SYNCYARD_HOME/gatekeeper/gk.py
//  3. ~/syncyard/gatekeeper/gk.py
//  4. ~/.syncyard/gatekeeper/gk.py
//  5. ~/workspace/desktop-access-migration/gatekeeper/gk.py
//     (this fleet's VM-local gatekeeper, Desktop Access program)
//  6. /var/lib/syncyard/gatekeeper/gk.py
//     (mini-server system install)
//  7. gatekeeper/gk.py beside, or one level up from, this binary
//  8. ./gatekeeper/gk.py relative to the working directory
//
// Python resolution: $SYNCYARD_PYTHON, else the first of
// python3 / python / py found on PATH (py is invoked as `py -3`).
//
// syncyard-cli resolution order: $SYNCYARD_CLI, then
// rust/target/{release,debug}/syncyard-cli under the checkout
// that holds the resolved gk.py (or under $SYNCYARD_HOME), then
// syncyard-cli beside this binary, then PATH.
//
// The Rust state home is always passed explicitly as --home (the
// CLI has no compiled-in default by design, FR-038-11): it is
// $SYNCYARD_STATE_HOME when set, else the directory holding the
// resolved gk.py. The daemon socket is <state home>/gk.sock.
package main

import (
	"os"
)

// version is the extension version. One version train with the
// Syncyard engine (spec 038 UPD-01): the 0.4.0 release carried
// the syncyard repo's VERSION, 0.4.0. (0.1.0 predated the train;
// the jump is deliberate and recorded in CHANGELOG.md.) 0.4.1 is
// an extension-only parity refresh on the same train; the
// engine remains 0.4.0.
const version = "0.4.1"

func main() {
	os.Exit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}
