#!/usr/bin/env bash
# build-offline.sh —— 打「离线安装包」：内网/断网机器上解压即用，不依赖 npm、Go、Docker。
#
# 与 build.sh 的分工：build.sh 负责编译与 release zip（自更新用的那种），本脚本负责
# 把「跑起来真正需要的东西」凑齐——二进制 + 内置 skills + 启动脚本 + 环境模板。
# 现场最常见的失败不是编译，而是：skills 忘了带、env 里没写 DSN、直接 ./artex 起在
# 0.0.0.0 上。这三件事在本脚本里都堵住了。
#
# 用法：
#   ./scripts/build-offline.sh                     # 当前平台
#   ./scripts/build-offline.sh --target linux/amd64
#   ./scripts/build-offline.sh --version 0.4.0 --out dist/offline
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$PWD"

TARGET_OS="$(go env GOOS)"
TARGET_ARCH="$(go env GOARCH)"
VERSION=""
OUT_DIR="dist/offline"

usage() {
  sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --target)  [ "$#" -ge 2 ] || { echo "--target 需要 OS/ARCH" >&2; exit 2; }
               TARGET_OS="${2%%/*}"; TARGET_ARCH="${2##*/}"; shift 2 ;;
    --version) [ "$#" -ge 2 ] || { echo "--version 需要版本号" >&2; exit 2; }
               VERSION="$2"; shift 2 ;;
    --out)     [ "$#" -ge 2 ] || { echo "--out 需要目录" >&2; exit 2; }
               OUT_DIR="$2"; shift 2 ;;
    -h|--help) usage 0 ;;
    *) echo "未知参数：$1" >&2; usage 2 ;;
  esac
done

if [ -z "$VERSION" ]; then
  VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
fi
VERSION="${VERSION#v}"

PKG="artex-${VERSION}-${TARGET_OS}-${TARGET_ARCH}"
STAGE="$OUT_DIR/$PKG"
BIN_NAME="artex"
[ "$TARGET_OS" = "windows" ] && BIN_NAME="artex.exe"

echo "==> 编译 ${TARGET_OS}/${TARGET_ARCH}（版本 ${VERSION}）"
# 复用 build.sh：它已经处理了前端静态导出、webui 嵌入、版本注入和 UPX 开关。
# 离线包要的是「能跑」而不是「最小」，所以压缩关掉——UPX 自解压在部分内网安全策略下会崩。
ARTEX_TARGET_OS="$TARGET_OS" \
ARTEX_TARGET_ARCH="$TARGET_ARCH" \
ARTEX_BUILD_VERSION="$VERSION" \
ARTEX_COMPRESS=off \
ARTEX_PACKAGE=0 \
ARTEX_OUTPUT="$ROOT/$OUT_DIR/$PKG/$BIN_NAME" \
  ./build.sh --target "${TARGET_OS}/${TARGET_ARCH}"

echo "==> 组装离线包目录"
mkdir -p "$STAGE/deploy" "$STAGE/run" "$STAGE/logs"
# 内置 skills 是运行时从文件系统加载的（不在二进制里），漏了就等于功能缺一半。
if [ -d skills ]; then
  rm -rf "$STAGE/skills"
  cp -R skills "$STAGE/skills"
fi
cp scripts/artexctl "$STAGE/artexctl"
chmod +x "$STAGE/artexctl" "$STAGE/$BIN_NAME" 2>/dev/null || true
cp deploy/.env.offline.example "$STAGE/deploy/.env.offline.example"
cp deploy/OFFLINE.md "$STAGE/README.md"

echo "==> 打包"
ARCHIVE="$OUT_DIR/$PKG.tar.gz"
tar -C "$OUT_DIR" -czf "$ARCHIVE" "$PKG"
if command -v sha256sum >/dev/null 2>&1; then
  ( cd "$OUT_DIR" && sha256sum "$(basename "$ARCHIVE")" > "$(basename "$ARCHIVE").sha256" )
else
  ( cd "$OUT_DIR" && shasum -a 256 "$(basename "$ARCHIVE")" > "$(basename "$ARCHIVE").sha256" )
fi

echo
echo "离线包：$ARCHIVE"
echo "校验和：$ARCHIVE.sha256"
echo "解压后：cd $PKG && cp deploy/.env.offline.example deploy/.env && vi deploy/.env && ./artexctl start"
