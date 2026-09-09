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
plugins and QML imports live under `bundle/lib`, `bundle/plugins`, and
`bundle/qml`; `bundle/bin/qt.conf` records those relative roots consistently
across Qt deployment-tool versions.
`QtQuick.Controls.Basic` is an explicit deployed dependency. Notices and exact
license texts are under `bundle/share` and are part of the hash inventory.

The bootstrap directory contains the shared Go package tool and the stable
launcher. Windows builds compile the launcher as a GUI-subsystem executable.
The bootstrap script verifies the release-manifest archive size and digest,
extracts only the exact package-tool entry into a private path, and delegates
all archive inspection, extraction and installation to that tool.

## Package tool interface

The portable command is `headroom-package` (`headroom-package.exe` on
Windows). Every command writes exactly one JSON object to standard output. A
successful command sets `ok` to `true`, names the `command`, includes its
typed `result`, and exits 0. A rejected command sets `ok` to `false`, includes
a bounded human-readable `error`, and exits 2. Diagnostics intended for a
person may also be written to standard error by the flag parser; callers must
make decisions from the exit status and JSON object rather than matching error
text.

- `inspect --archive <file>` validates bounded archive structure and returns
  `manifest` plus `archive_root`. It is cross-platform metadata inspection and
  does not establish payload hashes or permission to install. With
  `--install-root <path>` (or no option for the per-user default), `inspect`
  returns installation identity, completeness, active version, launcher and
  missing/corrupt paths.
- `verify --archive <file> [--version ... --platform ... --arch ... --asset
  ...]` privately extracts the package, verifies every recorded size, digest
  and mode, checks required runtime inventory and executable PE/ELF machines,
  then removes the extraction. It can verify a foreign target for build and
  release tooling.
- `stage` accepts the same archive expectations plus `--install-root`. It
  requires the package OS and CPU to match the running host and extracts into
  an owned private `staging/package-<random>/contents/<archive-root>` path. The
  result records that `package_root`, package identity and manifest SHA-256;
  `verified-stage.json` beside `contents` records the same result. Failed
  validation removes its private stage.
- `install` adds `--entry-path`, stages first, copies the complete bundle to
  `versions/<VERSION>`, writes its manifest and atomic `install-state.json`,
  and replaces the stable launcher only after validation. Installing the
  active version again is the repair operation for a missing or corrupt
  component.
- `check-update --install-root <path>` checks the public GitHub release metadata
  for a newer stable version of the exact native package. `stage-update` performs
  the same check, downloads the complete archive into a private directory,
  verifies its release size and SHA-256 plus the inner package contract, and
  writes a verified stage without changing the active version. `stage-repair`
  uses the installed version's exact release tag and is accepted only for a
  trusted installation with a missing server or credential helper. These
  commands accept `--cancel-stdin`; closing their input cancels the bounded
  operation and removes its incomplete private download or stage.
- `asset-name`, `create-package`, `create-release`, and `materialize-links`
  are build-recipe commands. `create-package` verifies its resulting archive;
  `create-release` fully verifies all three input archives before writing the
  release JSON.

The stable launcher is `headroom.exe` at the Windows install root and
`headroom-launcher` at the Linux install root, copied to the user-facing Linux
entry path. Each launcher has an adjacent mode-0600 `<launcher>.root` file
containing one absolute clean install-root path. This association makes custom
roots work without environment variables. The launcher verifies
`install-state.json`, its canonical `versions/<VERSION>` path, manifest digest,
application and required runtime files before starting the Qt executable. A
missing `usage-server` or Windows credential helper keeps the verified package
identity trusted and permits the UI to open so a matching-version repair can
run; a missing or corrupt desktop/runtime blocks launch. It passes
`HEADROOM_INSTALL_ROOT`, `HEADROOM_LAUNCHER_PATH`, and
`HEADROOM_PACKAGE_VERSION` to Qt. `StartupService` registers that stable path,
so version changes do not rewrite login configuration.

## Release JSON

`Headroom-v<VERSION>-release.json` is strict schema 1 JSON:

```json
{
  "schema": 1,
  "product": "Headroom",
  "version": "1.2.3",
  "packages": [{
    "platform": "windows",
    "architecture": "x86_64",
    "asset_name": "Headroom-v1.2.3-windows-x64.zip",
    "size": 123456,
    "sha256": "<64 lowercase hex characters>",
    "package_manifest_path": "Headroom-v1.2.3-windows-x64/package-manifest.json",
    "components": {
      "application": {"path": "bundle/bin/headroom.exe", "version": "1.2.3"},
      "server": {"path": "bundle/bin/usage-server.exe", "version": "1.2.3"},
      "credential_helper": {"path": "bundle/bin/headroom-credential-helper.exe", "version": "1.2.3"},
      "launcher": {"path": "bootstrap/headroom.exe", "version": "1.2.3"},
      "manager": {"path": "bootstrap/headroom-package.exe", "version": "1.2.3"}
    }
  }]
}
```

The array contains exactly `windows/x86_64`, `windows/arm64`, and
`linux/x86_64`; Linux uses executable names without `.exe` and a JSON `null`
credential helper. Archive size is an integer from 1 byte through 2 GiB. The
inner package manifest adds the Qt version, runtime baseline and a sorted list
of every payload path, size, SHA-256 and Linux mode. All component versions and
the release tag must equal the top-level strict SemVer.

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
Wayland rendering. On Ubuntu 22.04, install the runtime packages `libegl1`,
`libgl1`, `libglx0`, `libopengl0`, `libdrm2`, `libgbm1`, `libfontconfig1`,
`libfreetype6`, `libglib2.0-0`, `libgssapi-krb5-2`, `libssl3`,
`libwayland-client0`, `libwayland-cursor0`, `libwayland-egl1`, `libx11-6`,
`libx11-xcb1`, `libxkbcommon0`, `libxkbcommon-x11-0`, `libxcb1`,
`libxcb-cursor0`, `libxcb-glx0`, `libxcb-icccm4`, `libxcb-image0`,
`libxcb-keysyms1`, `libxcb-randr0`, `libxcb-render-util0`, `libxcb-render0`,
`libxcb-shape0`, `libxcb-shm0`, `libxcb-sync1`, `libxcb-xfixes0`,
`libxcb-xkb1`, `zlib1g`, and `libzstd1`. `libc6`, `libgcc-s1`, `libstdc++6`,
and `libdbus-1-3` are also part of the baseline and normally already installed
on an Ubuntu desktop. Precise KDE
Wayland tray attachment requires a distro/source
build with `HEADROOM_WITH_KDE_TRAY` and `HEADROOM_WITH_LAYER_SHELL`; those source
options remain enabled by default. Package smoke tests do not count as an
interactive tray-placement test.

Source and system-managed CMake installs remain direct installs and use their
own update method. They do not create package state or redirect startup through
the per-user launcher. Only a native, trusted per-user installation may use the
public acquisition commands. The desktop performs one delayed startup check,
automatically downloads and verifies a newer bundle, and advertises restart only
after the package tool returns a matching `verified-stage.json`. Preview, demo,
capture, and explicit-config sessions do not make public update requests.
