# Changelog

## 0.5.0 — 2026-11-10

Audit 2026-11-10 robustness release (PR #1): the extension
gains the install/upgrade verbs and a set of
robustness fixes found by the Syncyard audit. The verb
surface grows from 37 to 39 canonical verbs. Extension-only
release: the Syncyard engine remains 0.4.0.

- install / upgrade: download a named Syncyard
  release bundle, verify every asset against the release
  manifest's SHA-256 before anything is placed, place the
  daemon/CLI/agent/tray binaries atomically and idempotently
  (identical files keep their mtimes), never elevate silently,
  write registration material with the agent ID pinned
  explicitly (flag, then prior install record, then a stable
  root-derived ID; never $HOSTNAME), print the registration
  step as an explicit next action, and run doctor afterwards.
  A manifest mismatch, an unknown version, or a platform with
  no release assets refuses with the install root not created.
- Discovery: the mini-server gatekeeper path is now the
  governed nested layout /var/lib/syncyard/gatekeeper/gatekeeper/gk.py
  (tried ahead of the legacy flat path, which is kept); setup
  help, comments, help and README corrected to match.
- Socket verbs: daemon replies must be a JSON object with an
  explicit boolean ok (malformed, missing or non-bool replies
  fail with exit 1); replies are bounded at 1 MiB and an
  EOF-terminated partial line is an error, never a silent
  success. Real-transport tests spin an actual unix socket
  server; socketCallUnix coverage 0.0% to 85.7%.
- Native verbs: version/help/schema/doctor take no arguments
  and parity takes at most one --json; extra arguments are a
  usage error (exit 2) instead of being silently ignored.
- Doctor is functional: it executes gk.py --help, syncyard-cli
  version/schema, and pings an existing daemon socket (live vs
  stale); a component that cannot execute reports FAIL with a
  nonzero exit.
- SYNCYARD_HOME collision resolved: SYNCYARD_STATE_HOME is
  preferred for the Rust state home, the gatekeeper directory
  is the fallback, and SYNCYARD_HOME is never used as a state
  home; Rust forwarding sets both env names to the resolved
  state home for the call's duration.
- Parity drift enforcement: new tools/genparity generator
  re-extracts the parity tables from the gk.py parser and the
  Rust CLI schema, with a --check mode wired into CI; README
  documents the refresh command.
- Docs and release: README gains artifact-delete, staging-run
  and an Install section; submit's --image/--mounts are
  documented and --mounts forwarding is tested; the release
  workflow runs vet and tests and asserts the tag equals the
  binary-reported version.
- Tests: 24 to 50 test functions, root-package statement
  coverage 75.8% to 81.7%.

## 0.4.1 — 2026-11-10

Parity refresh: the Python gatekeeper gained two verbs after
0.4.0, and the extension names and routes both, so the verb
surface matches the live gk.py parser again (24 Python verbs).

- artifact-delete: delete one grab job's GitHub artifact
  (--job, --landed-sha256); the gatekeeper deletes only when
  the artifact's contents hash to exactly the given set.
- staging-run: run the staging-store harvest for completed
  jobs with pending staging (--job for one, --all for every
  pending; landed with the staging-store build, syncyard
  PR #92).

testdata/gk_verbs.txt re-extracted from the live gk.py parser;
the drift tests prove the tables match. Help and the parity
table carry both verbs with notes. Extension-only release:
the Syncyard engine remains 0.4.0.

## 0.4.0 — 2026-11-08

Full verb parity with the engines as they stand, proven by
drift tests rather than by eye. The version joins the Syncyard
one-version train (engine VERSION 0.4.0); 0.2.x/0.3.x were
never released.

- All 22 Python gatekeeper verbs are now named and routed
  (previously help listed 7 and everything else forwarded
  blind): submit (with auto routing arguments),
  status, snapshot, sweep, outbox, cancel, pause, focus,
  resume (including hold withdrawal via --job), hold, priority,
  lock, unlock, start, stop, drain, host-resume, reboot, adopt,
  purge, profile, probe-host.
- Rust engine verbs delegate to syncyard-cli with an explicit
  --home: progress, kill-switch,
  ledger, dispatch, migrate-history, flush.
- Control surface over the daemon's newline-JSON Unix
  socket: control-outcomes, control-command (delivers one
  signed tray command file; verification is daemon-side).
- New native verbs: parity (the routing table, --json) and
  schema (the parity surface as JSON, counterpart of the Rust
  CLI's schema verb). doctor now reports the gatekeeper, the
  Rust CLI, the state home and the daemon socket.
- Install-path lookup knows this fleet: the VM gatekeeper at
  ~/workspace/desktop-access-migration/gatekeeper and the
  mini-server install at /var/lib/syncyard/gatekeeper, with an
  honest failure (and setup help) when nothing is found.
- Unknown verbs are refused with the parity surface printed,
  instead of being forwarded blind — the drift this refresh
  exists to close can no longer pass silently.
- Test suite (24 tests) pins the parity tables to machine
  sources: testdata/gk_verbs.txt (gk.py's parser verbs) and
  testdata/cli_schema.json (the built syncyard-cli's schema
  verb output), plus mocked-transport forwarding tests for
  every routed verb.
- Named gaps, also printed by `gh syncyard help`: the Rust
  CLI's nine not_yet_verbs (sweep, priority, lock, unlock,
  focus, drain, probe-host, adopt, purge — all still served by
  the Python gatekeeper until the GitHub transport phase);
  daemon-internal agent_poll/agent_completion (not operator
  verbs); the control-command backchannel transport leg; the
  unix-only daemon socket in this build.

## 0.1.0 — 2026-10-31

- First release: thin Go dispatcher forwarding `gh syncyard ...` to
  the locally installed Syncyard gatekeeper (electrohire/syncyard).
  Gatekeeper lookup via $SYNCYARD_GK /
  $SYNCYARD_HOME / ~/syncyard / ~/.syncyard / beside the binary /
  ./gatekeeper; Python via $SYNCYARD_PYTHON or PATH.
- Extension-native `version`, `doctor`, and `help` commands.
- Precompiled binaries (windows/linux/darwin × amd64/arm64) built
  by the official gh-extension-precompile action.
