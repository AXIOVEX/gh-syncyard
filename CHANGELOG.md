# Changelog

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

Spec 031 refresh: full verb parity with the engines as they
stand, proven by drift tests rather than by eye. The version
joins the Syncyard one-version train (engine VERSION 0.4.0,
spec 038 UPD-01); 0.2.x/0.3.x were never released.

- All 22 Python gatekeeper verbs are now named and routed
  (previously help listed 7 and everything else forwarded
  blind): submit (with spec 016 auto routing arguments),
  status, snapshot, sweep, outbox, cancel, pause, focus,
  resume (including hold withdrawal via --job), hold, priority,
  lock, unlock, start, stop, drain, host-resume, reboot, adopt,
  purge, profile, probe-host.
- Rust engine verbs delegate to syncyard-cli with an explicit
  --home: progress (spec 034), kill-switch (spec 037 FIX-08),
  ledger, dispatch, migrate-history, flush.
- Spec 037 control surface over the daemon's newline-JSON Unix
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
  the locally installed Syncyard gatekeeper (AXIOVEX/syncyard,
  spec 010 FR-012). Gatekeeper lookup via $SYNCYARD_GK /
  $SYNCYARD_HOME / ~/syncyard / ~/.syncyard / beside the binary /
  ./gatekeeper; Python via $SYNCYARD_PYTHON or PATH.
- Extension-native `version`, `doctor`, and `help` commands.
- Precompiled binaries (windows/linux/darwin × amd64/arm64) built
  by the official gh-extension-precompile action.
