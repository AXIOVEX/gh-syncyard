# gh-syncyard — Syncyard for the GitHub CLI

Run jobs on **your own machines** from the command you already live in:

```console
$ gh extension install AXIOVEX/gh-syncyard
$ gh syncyard status
$ gh syncyard submit --lane auto --needs-gpu 12000 --script check.ps1
```

[Syncyard](https://github.com/AXIOVEX/syncyard) dispatches work to
self-hosted runner lanes on your Windows, WSL2, macOS, and Linux
hosts, supervised by a local **gatekeeper**: a queued, budgeted,
watchdogged door between "an agent wants this run" and your hardware.

## What this extension is

The **front door** over the Syncyard gatekeeper engines. Two
engines exist during the Rust migration (spec 038's one-move
cutover has not happened yet), and the extension routes per verb:

- The **Python gatekeeper** (`gatekeeper/gk.py`) is the live
  engine. Every verb it implements is forwarded verbatim —
  output and exit codes stream through unchanged.
- The **Rust engine** (`syncyard-cli` + `syncyard-daemon`) owns
  the newest verbs: live progress (spec 034), the FIX-08 kill
  switch and the tray control surface (spec 037), ledger,
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
$ git clone https://github.com/AXIOVEX/syncyard ~/syncyard
$ gh syncyard doctor
```

`doctor` shows exactly how each engine was found. Gatekeeper
lookup order: `$SYNCYARD_GK`, `$SYNCYARD_HOME/gatekeeper/gk.py`,
`~/syncyard`, `~/.syncyard`,
`~/workspace/desktop-access-migration/gatekeeper/gk.py` (the
fleet's VM gatekeeper), `/var/lib/syncyard/gatekeeper/gk.py`
(mini-server install), beside the extension binary, then
`./gatekeeper`. Python is found via `$SYNCYARD_PYTHON`, else
`python3` / `python` / `py` on your PATH. `syncyard-cli` is
found via `$SYNCYARD_CLI`, then `rust/target/{release,debug}`
under the checkout, beside the binary, or PATH. The Rust state
home is `$SYNCYARD_STATE_HOME`, else the gatekeeper's directory.

## Commands

**Gatekeeper (gk.py):** `submit` (with spec 016 auto routing:
`--lane auto` plus `--needs-cpu` / `--needs-ram-mb` /
`--needs-storage-mb` / `--needs-gpu` / `--venue` hints),
`status`, `snapshot`, `sweep`, `outbox`, `cancel`, `pause`,
`focus`, `resume` (also `--job` to withdraw a job's hold),
`hold`, `priority`, `lock`, `unlock`, `start`, `stop`, `drain`,
`host-resume`, `reboot`, `adopt`, `purge`, `profile`,
`probe-host`.

**Rust engine:** `progress`, `kill-switch`, `ledger`,
`dispatch`, `migrate-history`, `flush`, plus the daemon-socket
control verbs `control-outcomes` and `control-command`.

**Extension-native:**

| Command | What it does |
| --- | --- |
| `gh syncyard version` | Print the extension version |
| `gh syncyard doctor` | Show how each engine surface was resolved |
| `gh syncyard parity` | The verb parity table (`--json` for JSON) |
| `gh syncyard schema` | The parity surface as JSON |
| `gh syncyard help` | Usage, lookup rules, and the named gaps |

Known gaps are named in `gh syncyard help`: the nine verbs the
Rust CLI has not implemented yet (all still served by the Python
gatekeeper), the daemon-internal agent channel verbs (not
operator verbs), the control-command backchannel transport leg,
and the unix-only daemon socket in this build.

## License

Apache-2.0. Design: spec 010 FR-012 and spec 031 (extension
refresh) in
[AXIOVEX/syncyard](https://github.com/AXIOVEX/syncyard).
