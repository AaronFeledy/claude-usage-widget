#!/bin/sh
set -eu

repo=AaronFeledy/claude-usage-widget
package_path=
manifest_path=
install_root=${XDG_DATA_HOME:-"$HOME/.local/share"}/headroom
entry_path=$HOME/.local/bin/headroom
no_launch=0
dry_run=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --package) package_path=$2; shift 2 ;;
    --release-manifest) manifest_path=$2; shift 2 ;;
    --install-root) install_root=$2; shift 2 ;;
    --entry-path) entry_path=$2; shift 2 ;;
    --no-launch) no_launch=1; shift ;;
    --dry-run) dry_run=1; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

case $(uname -m) in x86_64|amd64) ;; *) echo 'Headroom currently supports Linux x86_64 only.' >&2; exit 1 ;; esac
if [ "$dry_run" -eq 1 ]; then
  printf 'Would validate and install Headroom at %s with stable entry %s\n' "$install_root" "$entry_path"
  exit 0
fi
command -v curl >/dev/null && command -v python3 >/dev/null && command -v tar >/dev/null || { echo 'curl, python3, and tar are required.' >&2; exit 1; }
if { [ -n "$package_path" ] && [ -z "$manifest_path" ]; } || { [ -z "$package_path" ] && [ -n "$manifest_path" ]; }; then
  echo '--package and --release-manifest must be supplied together.' >&2
  exit 2
fi

private_root=$(mktemp -d "${TMPDIR:-/tmp}/.headroom-install.XXXXXXXX")
trap 'rm -rf -- "$private_root"' EXIT HUP INT TERM
release_json=$private_root/release.json
github_json=$private_root/github-release.json

download() {
  current=$1
  destination=$2
  maximum=${3:-8388608}
  response=$private_root/download-response
  headers=$private_root/download-headers
  redirects=0
  while [ "$redirects" -le 5 ]; do
    python3 - "$current" <<'PY'
import sys, urllib.parse
u = urllib.parse.urlparse(sys.argv[1])
allowed = {'api.github.com', 'github.com', 'objects.githubusercontent.com', 'release-assets.githubusercontent.com'}
if u.scheme != 'https' or u.username or u.password or u.port not in (None,443) or (u.hostname or '').lower() not in allowed:
    raise SystemExit('refusing untrusted download URL: ' + sys.argv[1])
PY
    status=$(curl --silent --show-error --proto '=https' --connect-timeout 20 --max-time 600 --max-filesize "$maximum" \
      --user-agent Headroom-Installer --dump-header "$headers" --output "$response" --write-out '%{http_code}' "$current")
    case "$status" in
      200) mv -- "$response" "$destination"; return ;;
      301|302|303|307|308)
        location=$(python3 - "$headers" "$current" <<'PY'
import sys, urllib.parse
lines=open(sys.argv[1], encoding='iso-8859-1').read().splitlines()
values=[line.split(':',1)[1].strip() for line in lines if line.lower().startswith('location:')]
if len(values)!=1: raise SystemExit('redirect location is missing or ambiguous')
print(urllib.parse.urljoin(sys.argv[2], values[0]))
PY
)
        current=$location; redirects=$((redirects+1)) ;;
      *) echo "download failed with HTTP $status" >&2; exit 1 ;;
    esac
  done
  echo 'too many download redirects' >&2
  exit 1
}

if [ -n "$package_path" ]; then
  cp -- "$manifest_path" "$release_json"
else
  download "https://api.github.com/repos/$repo/releases/latest" "$github_json"
  release_url=$(python3 - "$github_json" <<'PY'
import json, re, sys
d=json.load(open(sys.argv[1], encoding='utf-8'))
m=re.fullmatch(r'v(.+)', str(d.get('tag_name','')))
if not m: raise SystemExit('latest release tag is not a Headroom version tag')
n=f'Headroom-v{m.group(1)}-release.json'
a=[x for x in d.get('assets',[]) if x.get('name') == n]
if len(a)!=1: raise SystemExit('release manifest asset is missing or ambiguous')
print(a[0]['browser_download_url'])
PY
)
  download "$release_url" "$release_json" 4194304
fi
[ "$(wc -c < "$release_json")" -le 4194304 ] || { echo 'release manifest is too large' >&2; exit 1; }
metadata=$(python3 - "$release_json" <<'PY'
import json, re, sys
d=json.load(open(sys.argv[1], encoding='utf-8'))
semver=r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?'
v=str(d.get('version',''))
if d.get('schema') != 1 or d.get('product') != 'Headroom' or not re.fullmatch(semver,v): raise SystemExit('unrecognized release manifest')
n=f'Headroom-v{v}-linux-x86_64.tar.gz'
p=[x for x in d.get('packages',[]) if x.get('platform')=='linux' and x.get('architecture')=='x86_64' and x.get('asset_name')==n]
if len(p)!=1 or type(p[0].get('size')) is not int or not (0 < p[0]['size'] <= 2147483648) or not re.fullmatch(r'[0-9a-f]{64}',str(p[0].get('sha256',''))): raise SystemExit('Linux package metadata is missing or invalid')
print(v); print(n); print(p[0]['size']); print(p[0]['sha256'])
PY
)
version=$(printf '%s\n' "$metadata" | sed -n '1p')
asset_name=$(printf '%s\n' "$metadata" | sed -n '2p')
expected_size=$(printf '%s\n' "$metadata" | sed -n '3p')
expected_hash=$(printf '%s\n' "$metadata" | sed -n '4p')
archive=$private_root/$asset_name
if [ -n "$package_path" ]; then cp -- "$package_path" "$archive"; else
  package_url=$(python3 - "$github_json" "$asset_name" <<'PY'
import json, sys
d=json.load(open(sys.argv[1], encoding='utf-8')); a=[x for x in d.get('assets',[]) if x.get('name')==sys.argv[2]]
if len(a)!=1: raise SystemExit('package asset is missing or ambiguous')
print(a[0]['browser_download_url'])
PY
)
  download "$package_url" "$archive" "$expected_size"
fi
[ "$(wc -c < "$archive")" = "$expected_size" ] || { echo 'package size does not match release manifest' >&2; exit 1; }
actual_hash=$(sha256sum "$archive" | cut -d ' ' -f 1)
[ "$actual_hash" = "$expected_hash" ] || { echo 'package hash does not match release manifest' >&2; exit 1; }

root_name=${asset_name%.tar.gz}
manager_entry=$root_name/bootstrap/headroom-package
listing=$(tar -tvzf "$archive" "$manager_entry")
[ "$(printf '%s\n' "$listing" | wc -l)" -eq 1 ] || { echo 'package bootstrap entry is missing or ambiguous' >&2; exit 1; }
manager_size=$(printf '%s\n' "$listing" | awk '{print $3}')
case "$listing" in -*) ;; *) echo 'package bootstrap entry is not a regular file' >&2; exit 1 ;; esac
[ "$manager_size" -gt 0 ] && [ "$manager_size" -le 67108864 ] || { echo 'package bootstrap entry is oversized' >&2; exit 1; }
manager=$private_root/headroom-package
tar -xOzf "$archive" "$manager_entry" > "$manager"
chmod 0700 "$manager"
"$manager" install --archive "$archive" --install-root "$install_root" --entry-path "$entry_path" \
  --version "$version" --platform linux --arch x86_64 --asset "$asset_name"

applications=${XDG_DATA_HOME:-"$HOME/.local/share"}/applications
mkdir -p -- "$applications"
desktop_tmp=$private_root/headroom.desktop
desktop_exec=$(python3 - "$entry_path" <<'PY'
import re, sys
print('"' + re.sub(r'([\\"`$])', r'\\\1', sys.argv[1]) + '"')
PY
)
cat > "$desktop_tmp" <<EOF
[Desktop Entry]
Type=Application
Name=Headroom
Comment=Usage monitor
Exec=$desktop_exec
Terminal=false
Categories=Utility;
EOF
install -m 0644 "$desktop_tmp" "$applications/headroom.desktop"
if [ "$no_launch" -eq 0 ]; then "$entry_path" >/dev/null 2>&1 & fi
printf 'Installed Headroom %s at %s\n' "$version" "$install_root"
