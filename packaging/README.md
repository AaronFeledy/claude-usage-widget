# Headroom package contract

Official desktop packages use manifest schema 1 and exact versioned names:

- `Headroom-v<VERSION>-windows-x64.zip`
- `Headroom-v<VERSION>-windows-arm64.zip`
- `Headroom-v<VERSION>-linux-x86_64.tar.gz`
- `Headroom-v<VERSION>-release.json`

`VERSION` is strict SemVer without a leading `v` in JSON. The release manifest
contains exactly the three desktop packages, their byte sizes and SHA-256
digests. Standalone `usage-server-*` and legacy `ClaudeUsageWidget-*` assets are
never desktop-package candidates.

Every archive has one top-level directory matching its filename without the
archive suffix. It contains `package-manifest.json`, `bootstrap/`, and `bundle/`.
Links, devices, absolute paths, traversal, Windows device/ADS aliases,
case-colliding names, unlisted files and unsafe modes are forbidden. The inner
manifest lists the size and SHA-256 digest of every regular payload file.

The bundle keeps `headroom`, `usage-server`, and the Windows-only
`headroom-credential-helper.exe` adjacent under `bundle/bin`. Qt libraries,
plugins and QML imports live in the relative paths recorded by `qt.conf`.
`QtQuick.Controls.Basic` is an explicit deployed dependency. Notices and exact
license texts are under `bundle/share` and are part of the hash inventory.

The bootstrap directory contains the shared Go package tool and the stable
launcher. Windows builds compile the launcher as a GUI-subsystem executable.
The bootstrap script verifies the release-manifest archive size and digest,
extracts only the exact package-tool entry into a private path, and delegates
all archive inspection, extraction and installation to that tool.

Per-user installations use immutable version directories. Windows stores the
stable launcher, package tool, atomic state and versions under
`%LOCALAPPDATA%\Headroom`; shortcuts and startup target the stable
`headroom.exe`. Linux stores state and versions under
`${XDG_DATA_HOME:-$HOME/.local/share}/headroom`; the real launcher file at
`$HOME/.local/bin/headroom` remains the stable entry. Settings stay in their
existing roaming/XDG locations and are never part of an application bundle.

The portable Linux x86_64 archive is built on Ubuntu 22.04. It bundles Qt but
uses the baseline desktop's glibc, libstdc++, graphics, font, X11/XCB, Wayland,
D-Bus and OpenSSL 3 ABI libraries. The generic bundle supports native X11 and
Wayland rendering. Precise KDE Wayland tray attachment requires a distro/source
build with `HEADROOM_WITH_KDE_TRAY` and `HEADROOM_WITH_LAYER_SHELL`; those source
options remain enabled by default. Package smoke tests do not count as an
interactive tray-placement test.

Source and system-managed CMake installs remain direct installs and use their
own update method. They do not create package state or redirect startup through
the per-user launcher.
