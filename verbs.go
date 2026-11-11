package main

// The parity tables. These are the extension's verb surface,
// classified by which engine serves each verb today. They are
// checked against machine sources by the drift tests:
//
//   - pythonVerbs must equal the live gk.py argparse verb list
//     (testdata/gk_verbs.txt, extracted from gk.py's parser).
//   - every verb in the Rust CLI's `schema` output
//     (testdata/cli_schema.json, captured from the built
//     syncyard-cli) must be accounted for here, and every
//     schema not_yet_verb must appear in pythonVerbs (it is
//     served by the Python engine until its Rust phase lands).
//
// One-move cutover (spec 038): when the Rust engine becomes the
// live engine, the shared verbs move from pythonVerbs routing to
// the CLI; until then the routing below is the honest state.

// pythonVerbs are forwarded verbatim to gatekeeper/gk.py.
var pythonVerbs = map[string]bool{
	"submit":          true, // incl. spec 016 auto routing (see help)
	"status":          true,
	"sweep":           true,
	"outbox":          true,
	"cancel":          true,
	"pause":           true,
	"focus":           true,
	"resume":          true, // also: withdraw one job's hold (--job)
	"priority":        true,
	"hold":            true,
	"lock":            true,
	"unlock":          true,
	"start":           true,
	"stop":            true,
	"purge":           true,
	"adopt":           true,
	"artifact-delete": true, // delete a grab artifact after hash-verified landing
	"staging-run":     true, // staging-store harvest pass (--job / --all)
	"host-resume":     true,
	"reboot":          true,
	"drain":           true,
	"snapshot":        true,
	"profile":         true, // spec 016 lane profiles (+ suggest)
	"probe-host":      true,
}

// rustCLIVerbs exist only in the Rust engine; they delegate to
// syncyard-cli with an explicit --home.
var rustCLIVerbs = map[string]bool{
	"progress":        true, // spec 034 live progress view
	"kill-switch":     true, // spec 037 FIX-08 (daemon kill_switch)
	"ledger":          true,
	"dispatch":        true,
	"migrate-history": true,
	"flush":           true,
}

// socketVerbs speak the daemon's newline-JSON socket protocol
// directly (the tray's GatekeeperChannel surface, spec 037).
var socketVerbs = map[string]bool{
	"control-outcomes": true, // daemon control_outcomes
	"control-command":  true, // daemon control_command
}

// nativeVerbs are the extension's own.
var nativeVerbs = map[string]bool{
	"version":   true,
	"--version": true,
	"-version":  true,
	"help":      true,
	"--help":    true,
	"-h":        true,
	"doctor":    true,
	"parity":    true,
	"schema":    true,
	"install":   true, // spec 031 amendment: verified release-bundle install
	"upgrade":   true, // spec 031 amendment: named-version upgrade
}

// cliSharedVerbs are implemented by BOTH the Python gatekeeper
// and the Rust CLI (per its schema verb); they route to Python
// until the spec 038 cutover. Kept as data so the parity test
// can assert the schema's shared verbs are all routed somewhere.
var cliSharedVerbs = map[string]bool{
	"submit":   true,
	"status":   true,
	"snapshot": true,
	"outbox":   true,
	"pause":    true,
	"stop":     true,
	"start":    true,
	"hold":     true,
	"resume":   true,
	"cancel":   true,
}

// cliOnlyVerbs are the Rust CLI schema verbs that route to the
// CLI from here (rustCLIVerbs) plus the CLI's own meta verbs,
// which the extension answers natively.
var cliOnlyVerbs = map[string]bool{
	"progress":        true,
	"kill-switch":     true,
	"ledger":          true,
	"dispatch":        true,
	"migrate-history": true,
	"flush":           true,
	"version":         true, // native: prints the extension's own version (same train)
	"schema":          true, // native: prints this parity surface as JSON
}

// daemonInternalVerbs are daemon socket verbs that are NOT user
// verbs: the agent venue's poll/completion channel. They are
// named here, and in help, so their absence from the verb
// surface is a documented decision, not drift.
var daemonInternalVerbs = map[string]string{
	"agent_poll":       "agent venue channel; driven by syncyard-agent, not operators",
	"agent_completion": "agent venue channel; driven by syncyard-agent, not operators",
	"ping":             "daemon liveness probe; use `gh syncyard doctor` / `status`",
}

// rustNotYetVerbs mirrors the Rust CLI schema's not_yet_verbs:
// gk.py verbs the Rust CLI does not implement yet. All are
// served by the Python engine today (they are in pythonVerbs);
// the gap is Rust-side only and closes with the GitHub
// transport phase. Values are the schema's own notes.
var rustNotYetVerbs = map[string]string{
	"sweep":      "harvest/reconcile loop lands with the GitHub transport",
	"priority":   "lane priority surface, later phase",
	"lock":       "lane locks, later phase",
	"unlock":     "lane locks, later phase",
	"focus":      "dispatch focus, later phase",
	"drain":      "host drain, later phase",
	"probe-host": "capability probe, later phase",
	"adopt":      "run adoption, later phase",
	"purge":      "destructive; requires its own operator design",
}
