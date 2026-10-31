# Changelog

## 0.1.0 — 2026-10-31

- First release: thin Go dispatcher forwarding `gh syncyard ...` to
  the locally installed Syncyard gatekeeper (AXIOVEX/syncyard,
  spec 010 FR-012). Gatekeeper lookup via $SYNCYARD_GK /
  $SYNCYARD_HOME / ~/syncyard / ~/.syncyard / beside the binary /
  ./gatekeeper; Python via $SYNCYARD_PYTHON or PATH.
- Extension-native `version`, `doctor`, and `help` commands.
- Precompiled binaries (windows/linux/darwin × amd64/arm64) built
  by the official gh-extension-precompile action.
