# gh-syncyard — Syncyard for the GitHub CLI

Run jobs on **your own machines** from the command you already live in:

```console
$ gh extension install AXIOVEX/gh-syncyard
$ gh syncyard status
$ gh syncyard submit --lane quick --script check.ps1
```

[Syncyard](https://github.com/AXIOVEX/syncyard) dispatches work to
self-hosted runner lanes on your Windows, WSL2, macOS, and Linux
hosts, supervised by a local **gatekeeper**: a queued, budgeted,
watchdogged door between "an agent wants this run" and your hardware.

## What this extension is

A **thin dispatcher**. The gatekeeper stays the engine (Python, in
the Syncyard repo); this small Go binary locates your installed
gatekeeper and forwards `gh syncyard ...` to it, streaming output and
exit codes unchanged. Precompiled binaries for Windows, Linux, and
macOS (amd64 + arm64) attach to every release — no interpreter is
needed for the extension itself.

## Setup

The extension needs a local Syncyard checkout (that's where the
gatekeeper lives):

```console
$ git clone https://github.com/AXIOVEX/syncyard ~/syncyard
$ gh syncyard doctor
```

`doctor` shows exactly how the gatekeeper was found. Lookup order:
`$SYNCYARD_GK`, `$SYNCYARD_HOME/gatekeeper/gk.py`, `~/syncyard`,
`~/.syncyard`, beside the extension binary, then `./gatekeeper`.
Python is found via `$SYNCYARD_PYTHON`, else `python3` / `python` /
`py` on your PATH.

## Commands

Everything after `gh syncyard` forwards to the gatekeeper —
`submit`, `status`, `sweep`, `outbox`, `cancel`, `pause`, `resume`.
Three commands are the extension's own:

| Command | What it does |
| --- | --- |
| `gh syncyard version` | Print the extension version |
| `gh syncyard doctor` | Show the resolved gatekeeper + Python |
| `gh syncyard help` | Usage and lookup rules |

## License

Apache-2.0. Design: spec 010 FR-012 in
[AXIOVEX/syncyard](https://github.com/AXIOVEX/syncyard).
