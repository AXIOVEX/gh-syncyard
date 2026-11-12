# gh-syncyard — Syncyard for the GitHub CLI

Run jobs on **your own machines** from the command you already live in:

```console
$ gh extension install electrohire/gh-syncyard
$ gh syncyard status
$ gh syncyard submit --lane auto --needs-gpu 12000 --script check.ps1
```

[Syncyard](https://github.com/electrohire/syncyard) dispatches work to
self-hosted runner lanes on your Windows, WSL2, macOS, and Linux
hosts, supervised by a local **gatekeeper**: a queued, budgeted,
watchdogged door between "an agent wants this run" and your hardware.

## What this extension is

The **front door** over the Syncyard gatekeeper engines. Two
engines exist during the Rust migration (the one-move
cutover has not happened yet), and the extension routes per verb:

- The **Python gatekeeper** (`gatekeeper/gk.py`) is the live
  engine. Every verb it implements is forwarded verbatim —
  output and exit codes stream through unchanged.
- The **Rust engine** (`syncyard-cli` + `syncyard-daemon`) owns
  the newest verbs: live progress, the FIX-08 kill
  switch and the tray control surface, ledger,
  dispatch, migrate-history and flush. Those delegate to the
  `syncyard-cli` binary (always with an explicit `--home`), or
  speak the daemon's newline-JSON Unix socket directly for the
  control surface — the same channel the tray uses.

`gh syncyard parity` prints the full routing table; the test
suite proves it against both engines' machine-readable surfaces
(gk.py's parser verb list and the Rust CLI's `schema` verb), so
drift fails CI instead of shipping.

## Setup

The extension needs a local Syncyard checkout (that's where the
gatekeeper lives):

```console
$ git clone https://github.com/electrohire/syncyard ~/syncyard
$ gh syncyard doctor
```

`doctor` shows exactly how each engine was found, and it is a
functional check: it executes `gk.py --help` and
`syncyard-cli version` / `schema`, and pings the daemon
socket when one exists. A component that cannot execute
reports FAIL and doctor exits nonzero. Gatekeeper
lookup order: `$SYNCYARD_GK`, `$SYNCYARD_HOME/gatekeeper/gk.py`,
`~/syncyard`, `~/.syncyard`,
`~/workspace/desktop-access-migration/gatekeeper/gk.py` (the
fleet's VM gatekeeper),
`/var/lib/syncyard/gatekeeper/gatekeeper/gk.py` (mini-server's
governed install; the flat legacy path is tried after it),
beside the extension binary, then
`./gatekeeper`. Python is found via `$SYNCYARD_PYTHON`, else
`python3` / `python` / `py` on your PATH. `syncyard-cli` is
found via `$SYNCYARD_CLI`, then `rust/target/{release,debug}`
under the checkout, the install root (below), beside the
binary, or PATH.

Two environment variables sound alike and are not:

- `SYNCYARD_HOME` is a Syncyard **checkout root**, read by
  this extension only. The Rust tools read the same name as
  their **state home**, so the extension never uses it as a
  state home.
- `SYNCYARD_STATE_HOME` is the Rust engine's **state home**,
  preferred over every other source; when it is unset, the
  gatekeeper's directory is used. When the extension
  delegates to `syncyard-cli` it passes the state home
  explicitly, both as `--home` and in the child's
  environment (as `SYNCYARD_STATE_HOME` and, for the Rust
  tools' own convention, `SYNCYARD_HOME`), so the checkout
  meaning cannot leak into the child.

## Install

```console
$ gh syncyard install                 # this extension's version
$ gh syncyard install --version 0.4.0 --agent-id my-host
$ gh syncyard upgrade --version 0.5.0
```

`install` / `upgrade` fetch the named
Syncyard release bundle for this host's OS/arch from
[electrohire/syncyard releases](https://github.com/electrohire/syncyard/releases),
verify every component against the release SHA-256 manifest
(`MANIFEST.json`) **before placing anything** (a mismatch
refuses and places nothing), and place `syncyard-daemon`,
`syncyard-cli`, `syncyard-agent` and `syncyard-tray` (where
the release carries one for the platform) under the install
root: `$SYNCYARD_INSTALL_ROOT`, else
`~/.local/share/syncyard`. The v0.4.0 release carries Linux
x86_64 and aarch64 assets only; on any other platform the
verb refuses with that fact rather than improvising.
Re-running is idempotent. The verb never elevates silently:
it writes the per-OS service-registration material, with
the agent id pinned explicitly (`--agent-id`, never derived
from `$HOSTNAME`), under `<install root>/registration/`,
prints the registration step as an explicit next action,
and finishes by running `doctor` and reporting its verdict.

## Commands

**Gatekeeper (gk.py):** `submit` (with auto routing:
`--lane auto` plus `--needs-cpu` / `--needs-ram-mb` /
`--needs-storage-mb` / `--needs-gpu` / `--venue` hints, and
the docker-transport `--image` / `--mounts` arguments),
`status`, `snapshot`, `sweep`, `outbox`, `cancel`, `pause`,
`focus`, `resume` (also `--job` to withdraw a job's hold),
`hold`, `priority`, `lock`, `unlock`, `start`, `stop`, `drain`,
`host-resume`, `reboot`, `adopt`, `purge`, `profile`,
`probe-host`, `artifact-delete`, `staging-run`.

**Rust engine:** `progress`, `kill-switch`, `ledger`,
`dispatch`, `migrate-history`, `flush`, plus the daemon-socket
control verbs `control-outcomes` and `control-command`.

**Extension-native:**

| Command | What it does |
| --- | --- |
| `gh syncyard version` | Print the extension version |
| `gh syncyard doctor` | Functionally check each engine surface (executes the engines, pings the daemon) |
| `gh syncyard parity` | The verb parity table (`--json` for JSON) |
| `gh syncyard schema` | The parity surface as JSON |
| `gh syncyard help` | Usage, lookup rules, and the named gaps |
| `gh syncyard install [--version X]` | Install the Syncyard stack from the verified release bundle |
| `gh syncyard upgrade --version X` | Upgrade the stack to a named release |

## Development

The parity fixtures under `testdata/` are generated from
the engines' sources, not hand-maintained. Refresh them
after an engine verb change with:

```console
$ go run ./tools/genparity \
    --gk  /path/to/electrohire/syncyard/gatekeeper/gk.py \
    --cli /path/to/syncyard/rust/crates/syncyard-cli/src/lib.rs \
    --out testdata
```

CI runs the same generator against pinned engine revisions
and fails on drift. One allowance is documented in the CI
job: the live fleet gatekeeper carries `artifact-delete`,
which electrohire/syncyard main does not have yet, so that
single line is excluded from the CI comparison until
syncyard main gains the verb; `genparity --check` against
the live gatekeeper remains the strict full comparison.

Known gaps are named in `gh syncyard help`: the nine verbs the
Rust CLI has not implemented yet (all still served by the Python
gatekeeper), the daemon-internal agent channel verbs (not
operator verbs), the control-command backchannel transport leg,
and the unix-only daemon socket in this build.

## License

Apache-2.0. The design lives in
[electrohire/syncyard](https://github.com/electrohire/syncyard).
