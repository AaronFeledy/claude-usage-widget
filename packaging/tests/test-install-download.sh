#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf -- "$fixture"' EXIT
mkdir -p "$fixture/bin" "$fixture/private"
cat > "$fixture/bin/curl" <<'CURL'
#!/usr/bin/env bash
set -euo pipefail
headers=
while (($#)); do
  case "$1" in
    --dump-header) headers=$2; shift 2 ;;
    --connect-timeout|--max-time|--user-agent|--proto|--output) shift 2 ;;
    --silent|--show-error) shift ;;
    *) shift ;;
  esac
done
printf 'called\n' >> "$HEADROOM_FAKE_CURL_CALLS"
printf 'HTTP/1.1 200 OK\r\n\r\n' > "$headers"
python3 - "$HEADROOM_FAKE_BYTES" <<'PY'
import sys
sys.stdout.buffer.write(b'x' * int(sys.argv[1]))
PY
CURL
chmod 0700 "$fixture/bin/curl"

(
export PATH="$fixture/bin:$PATH"
export HEADROOM_FAKE_CURL_CALLS="$fixture/curl-calls"
export HEADROOM_FAKE_BYTES=8192
export HEADROOM_INSTALLER_SOURCE_ONLY=1
# shellcheck source=../../install.sh
source "$repo_root/install.sh"

if download 'https://github.com/synthetic-unknown-length' "$fixture/oversized" 1024 30; then
  echo 'unknown-length oversized response was accepted' >&2
  exit 1
fi
[[ ! -e "$fixture/oversized" ]]
[[ $(wc -l < "$HEADROOM_FAKE_CURL_CALLS") -eq 1 ]]

: > "$HEADROOM_FAKE_CURL_CALLS"
export HEADROOM_FAKE_BYTES=512
download 'https://github.com/synthetic-within-limit' "$fixture/within-limit" 1024 30
[[ $(wc -c < "$fixture/within-limit") -eq 512 ]]
[[ $(wc -l < "$HEADROOM_FAKE_CURL_CALLS") -eq 1 ]]

: > "$HEADROOM_FAKE_CURL_CALLS"
download 'https://github-releases.githubusercontent.com/synthetic-asset' "$fixture/release-host" 1024 30
[[ $(wc -c < "$fixture/release-host") -eq 512 ]]
[[ $(wc -l < "$HEADROOM_FAKE_CURL_CALLS") -eq 1 ]]

: > "$HEADROOM_FAKE_CURL_CALLS"
if download 'https://github-releases.githubusercontent.com:444/synthetic-asset' "$fixture/untrusted-port" 1024 30; then
  echo 'untrusted release redirect port was accepted' >&2
  exit 1
fi
[[ ! -e "$fixture/untrusted-port" ]]
[[ ! -s "$HEADROOM_FAKE_CURL_CALLS" ]]

: > "$HEADROOM_FAKE_CURL_CALLS"
if download 'https://github.com/synthetic-deadline' "$fixture/deadline" 1024 0; then
  echo 'expired aggregate deadline was accepted' >&2
  exit 1
fi
[[ ! -e "$fixture/deadline" ]]
[[ ! -s "$HEADROOM_FAKE_CURL_CALLS" ]]
)
