#!/usr/bin/env bash
set -euo pipefail

if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI is required. Install from https://cli.github.com/" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
  echo "Usage: scripts/publish_release.sh <tag> [notes-file]" >&2
  exit 1
fi

TAG="$1"
NOTES="${2:-RELEASE_NOTES.md}"

if [ ! -f "$NOTES" ]; then
  echo "Release notes file '$NOTES' not found" >&2
  exit 1
fi

# 当前发布优先 x86_64 Linux，macOS 和 Windows 暂时停用。恢复时取消本文件相关注释。
if [ "$(uname -m)" != "x86_64" ]; then
  echo "Linux release packages must be built on x86_64, got $(uname -m)" >&2
  exit 1
fi

# MAC_APP_PRIMARY="bin/Portico.app"
# MAC_ARCHS=("arm64" "amd64")
# MAC_ZIPS=()
#
# package_macos_arch() {
#   local arch="$1"
#   local staging_dir="bin/package-${arch}"
#   local staging_app="${staging_dir}/Portico.app"
#   local zip_path="bin/portico-macos-${arch}.zip"
#
#   echo "==> Building macOS ${arch}"
#   env ARCH="$arch" wails3 task package ${BUILD_OPTS:-}
#
#   local bundle_path="$MAC_APP_PRIMARY"
#   if [ ! -d "$bundle_path" ]; then
#     echo "Missing asset: $MAC_APP_PRIMARY" >&2
#     exit 1
#   fi
#
#   rm -rf "$staging_dir"
#   mkdir -p "$staging_dir"
#   cp -R "$bundle_path" "$staging_app"
#
#   echo "==> Archiving macOS app bundle (${arch})"
#   rm -f "$zip_path"
#   ditto -c -k --sequesterRsrc --keepParent "$staging_app" "$zip_path"
#   rm -rf "$staging_dir"
#
#   MAC_ZIPS+=("$zip_path")
# }
#

perl -0pi -e "s/const\\s+AppVersion\\s*=\\s*\"[^\"]*\"/const AppVersion = \"$TAG\"/" version_service.go

# wails3 task common:update:build-assets
# for arch in "${MAC_ARCHS[@]}"; do
#   package_macos_arch "$arch"
# done
#
# env ARCH=amd64 wails3 task windows:package ${BUILD_OPTS:-}

# nfpm.yaml 里的版本号是固定值，打包前改成当前 tag，否则 deb/rpm 无法正常升级
sed -i "s/^version: .*/version: \"${TAG#v}\"/" build/linux/nfpm/nfpm.yaml

echo "==> Building Linux amd64"
env ARCH=amd64 GOARCH=amd64 \
  GIT_COMMITTER_NAME="${GIT_COMMITTER_NAME:-$(git config user.name)}" \
  GIT_COMMITTER_EMAIL="${GIT_COMMITTER_EMAIL:-$(git config user.email)}" \
  wails3 task package ${BUILD_OPTS:-}

mv bin/*.AppImage bin/portico-linux-amd64.AppImage
mv bin/Portico.deb bin/portico-linux-amd64.deb
mv bin/Portico.rpm bin/portico-linux-amd64.rpm
mv bin/Portico.pkg.tar.zst bin/portico-linux-amd64.pkg.tar.zst

ASSETS=(
  "bin/portico-linux-amd64.AppImage"
  "bin/portico-linux-amd64.deb"
  "bin/portico-linux-amd64.rpm"
  "bin/portico-linux-amd64.pkg.tar.zst"
  # "${MAC_ZIPS[@]}"
  # "bin/Portico-amd64-installer.exe"
  # "bin/Portico.exe"
)

for asset in "${ASSETS[@]}"; do
  [ -e "$asset" ] || { echo "Missing asset: $asset" >&2; exit 1; }
  echo "  asset: $asset"
done

# gh release create "$TAG" "${ASSETS[@]}" \
#   --title "$TAG" \
#   --notes-file "$NOTES"
