package main

const helpText = `gh syncyard — Syncyard for the GitHub CLI

Syncyard dispatches jobs to your own machines through self-hosted
runner lanes, supervised by a local gatekeeper. This extension is
the front door over the gatekeeper engines: the Python gatekeeper
(gatekeeper/gk.py) is the live engine today, and the Rust engine
(syncyard-cli + syncyard-daemon) owns the newest verbs. Routing
is per verb, listed below; ` + "`gh syncyard parity`" + ` prints the
same surface as a table and ` + "`gh syncyard schema`" + ` as JSON.

Usage:
  gh syncyard <command> [args...]

Gatekeeper commands (forwarded verbatim to gk.py):
  submit      queue a job script for dispatch to a runner lane.
              Spec 016 auto routing: --lane auto (or omitted
              where enabled) plus resource hints --needs-cpu,
              --needs-ram-mb, --needs-storage-mb, --needs-gpu
              and --venue are forwarded untouched; the sweep
              picks the lane from hints, history and host
              capability snapshots (see probe-host, profile).
  status      queue, lane and run state (--repo adds CI health)
  snapshot    spec 014 fleet snapshot as one JSON object
  sweep       one dispatch + watch pass
  outbox      push notifications owed to registered chats (--ack
              advances the cursor after delivery)
  cancel      cancel a queued (not yet dispatched) job
  pause       stop dispatching (watching/harvesting continues);
              --drain lets executing jobs finish first
  focus       pause all but the selected jobs (spec 013 FR-001)
  resume      resume dispatching; with --job, withdraw one job's
              checkpoint hold (spec 013 FR-009)
  hold        request a cooperative checkpoint hold on a job
  priority    change a waiting job's priority (spec 013 FR-004)
  lock        acquire/renew/suspend/resume a lane priority lock
              (spec 015); --list shows the locks
  unlock      release a lane priority lock (spec 015 FR-003)
  start       clear the pipeline stop
  stop        stop the pipeline (--reason recorded)
  drain       drain one host: hold its dispatch, signal running
              jobs to checkpoint, print the CLEAN / INTERRUPTING
              verdict (spec 011 FR-007)
  host-resume clear a host's going-down and drain holds
  reboot      request a controlled reboot of a host (spec 011)
  adopt       adopt an externally started run into the queue
  purge       archive/delete terminal records (dry run unless
              --yes)
  profile     render the spec 016 lane profiles; 'profile
              suggest' drafts a profile from a host capability
              snapshot and applies nothing
  probe-host  record a host capability snapshot for auto
              routing (spec 016 FR-005)
  artifact-delete
              delete one grab job's GitHub artifact (--job,
              --landed-sha256); the artifact is deleted only
              when its contents hash to exactly the given set,
              as verified by the harvesting chat
  staging-run run the staging-store harvest for completed jobs
              with pending staging (--job for one job, --all
              for every one; the sweep's own cadence stays one
              job per pass)

Rust engine commands (delegate to syncyard-cli, explicit --home):
  progress JOB        live progress view for a running job
                      (spec 034; fraction, markers, staleness)
  kill-switch [on|off]
                      the FIX-08 repair kill switch (spec 037):
                      read it, or set/clear it as a ledgered
                      operator act
  ledger [--tail N]   recent ledger entries
  dispatch JOB        lift one queued job now (daemon transport)
  migrate-history --from DIR
                      import a Python gatekeeper directory with
                      its SHA-256 manifest (spec 038)
  flush               flush daemon RAM state to disk now

Daemon control commands (daemon socket, spec 037):
  control-outcomes    the gatekeeper's published outcomes for
                      tray control commands (the only surface an
                      app may treat as confirmation)
  control-command FILE
                      deliver one signed tray command file to
                      the local daemon for verification and
                      execution (verification is daemon-side)

Extension-native commands:
  gh syncyard version   print the extension version
  gh syncyard doctor    show how each engine was located
  gh syncyard parity    the verb parity table (--json for JSON)
  gh syncyard schema    the parity surface as JSON
  gh syncyard help      this text

Known gaps (named, not silent):
  - sweep, priority, lock, unlock, focus, drain, probe-host,
    adopt and purge are served by the Python gatekeeper; the
    Rust CLI does not implement them yet (its schema verb lists
    them as not_yet_verbs). They land with the GitHub transport
    phase, and routing flips at the spec 038 one-move cutover.
  - The daemon's agent_poll / agent_completion socket verbs are
    the syncyard-agent venue channel, not operator verbs; they
    are deliberately not exposed here.
  - control-command delivers to a daemon on THIS host. The
    backchannel leg that carries signed command files from a
    tray host to the gatekeeper host is field wiring (spec 037
    gap carried from P4), not part of this extension.
  - The daemon socket channel is unix-only in this build (the
    tray carries the same limitation); on Windows the control
    surface goes through the tray app.

Gatekeeper lookup: $SYNCYARD_GK, then
$SYNCYARD_HOME/gatekeeper/gk.py, then ~/syncyard, ~/.syncyard,
~/workspace/desktop-access-migration/gatekeeper/gk.py (this
fleet's VM gatekeeper), /var/lib/syncyard/gatekeeper/gk.py
(mini-server), beside this binary, or ./gatekeeper.
Python: $SYNCYARD_PYTHON, else python3 / python / py on PATH.
syncyard-cli: $SYNCYARD_CLI, then rust/target/{release,debug}
under the Syncyard checkout, beside this binary, or PATH.
Rust state home: $SYNCYARD_STATE_HOME, else the gatekeeper's
directory (always passed explicitly; there is no default).
`
