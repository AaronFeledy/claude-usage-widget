#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 8 ]]; then
  echo "usage: package-linux.sh VERSION BUILD_DIR WORK_DIR OUTPUT_DIR SERVER LAUNCHER MANAGER QT_ROOT" >&2
  exit 2
fi

version=$1
build_dir=$2
work_dir=$3
output_dir=$4
server=$5
launcher=$6
manager=$7
qt_root=$8
"$manager" asset-name --version "$version" --platform linux --arch x86_64 >/dev/null
asset="Headroom-v${version}-linux-x86_64.tar.gz"
package_root="${work_dir}/Headroom-v${version}-linux-x86_64"
[[ "$work_dir" = /* && "$output_dir" = /* && "$package_root" != "/" ]] || { echo "work and output directories must be absolute" >&2; exit 2; }

rm -rf -- "$package_root"
mkdir -p -- "$package_root/bundle" "$package_root/bootstrap" "$output_dir"
cmake --install "$build_dir" --prefix "$package_root/bundle"
copy_platform_plugin() {
  local filename=$1
  local source=
  for candidate in "$qt_root/plugins/platforms/$filename" "$qt_root/lib/qt6/plugins/platforms/$filename"; do
    if [[ -f "$candidate" ]]; then source=$candidate; break; fi
  done
  [[ -n "$source" ]] || { echo "Qt platform plugin $filename was not found under $qt_root" >&2; exit 1; }
  install -Dm0755 "$source" "$package_root/bundle/lib/qt6/plugins/platforms/$filename"
  patchelf --set-rpath '$ORIGIN/../../../' "$package_root/bundle/lib/qt6/plugins/platforms/$filename"
}
copy_platform_plugin libqxcb.so
copy_platform_plugin libqoffscreen.so
if [[ -f "$qt_root/plugins/platforms/libqwayland-generic.so" || -f "$qt_root/lib/qt6/plugins/platforms/libqwayland-generic.so" ]]; then
  copy_platform_plugin libqwayland-generic.so
else
  copy_platform_plugin libqwayland.so
fi
install -m 0755 "$server" "$package_root/bundle/bin/usage-server"
install -m 0755 "$launcher" "$package_root/bootstrap/headroom"
install -m 0755 "$manager" "$package_root/bootstrap/headroom-package"
install -m 0755 "$manager" "$package_root/bundle/bin/headroom-package"
install -m 0644 packaging/THIRD_PARTY_NOTICES.txt "$package_root/bundle/share/headroom/THIRD_PARTY_NOTICES.txt"
install -Dm0644 LICENSE "$package_root/bundle/share/licenses/headroom/LICENSE"
mkdir -p "$package_root/bundle/share/licenses/qt" "$package_root/bundle/share/licenses/go/runtime" "$package_root/bundle/share/licenses/go/protobuf" "$package_root/bundle/share/licenses/go/yaml" "$package_root/bundle/share/licenses/openssl"
mapfile -d '' qt_licenses < <(find "$qt_root" -maxdepth 3 -type f \( -name 'LICENSE*' -o -name '*NOTICE*' \) -print0)
((${#qt_licenses[@]} > 0)) || { echo "Qt license inventory not found under $qt_root" >&2; exit 1; }
for license in "${qt_licenses[@]}"; do relative=${license#"$qt_root"/}; install -Dm0644 "$license" "$package_root/bundle/share/licenses/qt/$relative"; done
install -m 0644 "$(go env GOROOT)/LICENSE" "$package_root/bundle/share/licenses/go/runtime/LICENSE"
module_cache=$(go env GOMODCACHE)
install -m 0644 "$module_cache/google.golang.org/protobuf@v1.36.11/LICENSE" "$package_root/bundle/share/licenses/go/protobuf/LICENSE"
install -m 0644 "$module_cache/gopkg.in/yaml.v3@v3.0.1/LICENSE" "$package_root/bundle/share/licenses/go/yaml/LICENSE"
install -m 0644 "$module_cache/gopkg.in/yaml.v3@v3.0.1/NOTICE" "$package_root/bundle/share/licenses/go/yaml/NOTICE"
openssl_license=
for candidate in /usr/share/doc/libssl3/copyright /usr/share/licenses/openssl/LICENSE.txt /usr/share/licenses/openssl/LICENSE; do
  if [[ -f "$candidate" ]]; then openssl_license=$candidate; break; fi
done
[[ -n "$openssl_license" ]] || { echo 'OpenSSL license inventory not found' >&2; exit 1; }
install -m 0644 "$openssl_license" "$package_root/bundle/share/licenses/openssl/$(basename "$openssl_license")"
"$manager" materialize-links --root "$package_root"
env -u QT_PLUGIN_PATH -u QML2_IMPORT_PATH -u QML_IMPORT_PATH -u QT_QPA_PLATFORM_PLUGIN_PATH \
  "$manager" create-package --root "$package_root" --output "$output_dir/$asset" \
    --version "$version" --platform linux --arch x86_64 --qt-version 6.8.3 \
    --baseline ubuntu-22.04-glibc-2.35
"$manager" verify --archive "$output_dir/$asset" --version "$version" --platform linux --arch x86_64 --asset "$asset"
env -u QT_PLUGIN_PATH -u QML2_IMPORT_PATH -u QML_IMPORT_PATH -u QT_QPA_PLATFORM_PLUGIN_PATH \
  LD_LIBRARY_PATH="$package_root/bundle/lib" ldd "$package_root/bundle/bin/headroom" | tee "$output_dir/linux-runtime-dependencies.txt"
if grep -F 'not found' "$output_dir/linux-runtime-dependencies.txt"; then exit 1; fi
