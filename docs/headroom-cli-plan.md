# Unified Headroom CLI and local updates

This work is stacked on PR #12. It adds a public `headroom` command while
retaining the desktop app, existing configuration, and the `usage-server`
compatibility entry point.

## User-facing behavior

- `headroom` shows provider meters, remaining usage, reset times, pacing, and
  warning levels. A terminal can refresh live; redirected output is one
  snapshot, with explicit plain-text and JSON options.
- `headroom serve` runs the existing server with its existing flags,
  configuration, provider behavior, SSH access, and private desktop sessions.
- `headroom update` checks, verifies, and applies the appropriate installation
  update. A local desktop and server share an update operation, whichever
  component initiated it. A CLI-only installation does not acquire Qt.
- Desktop shortcuts and login startup continue opening the desktop app.
- A remote server is updated on its own host. The desktop explains when its
  installed version is newer than the connected server and directs the user
  to update that server manually.

Windows plus a WSL server is a local pairing only when explicitly associated
through native local mechanisms. A URL, a matching hostname, or a server's
claim to be local cannot authorize updating another installation.

## Implementation order

1. Extract the server runtime into a reusable Go package and retain the
   existing `usage-server` entry point as a compatibility shim.
2. Implement the CLI command surface, read-only usage transport, terminal
   rendering, and pacing/warning parity. Preserve real empty states and last
   successful readings; do not introduce demonstration usage or reset actions.
3. Extend the existing Go package manager for CLI installation and updates,
   using its bounded downloads, archive validation, immutable generations,
   process identity checks, and recovery machinery.
4. Connect desktop-initiated and CLI-initiated updates to the same local
   installation ownership. Add explicit Windows/WSL pairing, staged updates,
   restart handling, and actionable failure/recovery results.
5. Add the desktop remote-server version notice, refreshed after update and
   when the selected connection changes.
6. Assemble and verify the CLI artifacts and installers, preserve desktop
   package/update compatibility, and run native package/transaction gates.
7. Document commands, installation, local pairing, service migration, update
   behavior, and recovery. Keep the stacked PR based on its parent until the
   parent is merged.

## Compatibility and verification

Server configuration and service paths, module/import identities, the
`usage-server` alias, API provider key `Codex`, and existing API fields remain
compatible. The terminal labels that provider ChatGPT. Existing desktop
installations retain a working stable launch path throughout migration.

Validation uses synthetic usage snapshots, provider-disabled server fixtures,
disposable installation roots, interrupted-update recovery tests, and native
Windows/macOS/Linux jobs. No test, smoke command, CLI interaction, or update
validation may invoke a usage-reset button, confirmation, endpoint, or code
that could consume a banked reset.
