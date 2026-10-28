#!/usr/bin/env bash
# ARTEX 优化批次的可重复验证脚本。
#
# 为什么需要它：db/ 与 server/ 两个包在没有 DSN 时会把用例 t.Skip 掉，
# 于是 `go test ./...` 全绿但什么都没验。本脚本强制要求 ARTEX_PG_DSN，
# 并断言输出里没有 SKIP，避免"假绿"。
#
# 用法：
#   ARTEX_PG_DSN='postgres://artex:pass@127.0.0.1:5432/artex?sslmode=disable' \
#     bash scripts/verify.sh            # 后端全量
#   bash scripts/verify.sh --web        # 额外构建前端静态导出
#   bash scripts/verify.sh --race       # 额外跑一遍 -race（慢）
#
# 退出码：0 全绿；1 任一环节失败。
set -u -o pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT" || exit 1

WITH_WEB=0
WITH_RACE=0
NO_DB=0
for arg in "$@"; do
  case "$arg" in
    --web) WITH_WEB=1 ;;
    --race) WITH_RACE=1 ;;
    --no-db) NO_DB=1 ;;
    *) echo "未知参数: $arg" >&2; exit 1 ;;
  esac
done

fail=0
step() { printf '\n=== %s ===\n' "$1"; }
ok()   { printf '  [OK] %s\n' "$1"; }
bad()  { printf '  [FAIL] %s\n' "$1"; fail=1; }
warn() { printf '  [WARN] %s\n' "$1"; }

# 1. 编译
step "go build ./..."
if go build ./... ; then ok "编译通过"; else bad "编译失败"; fi

# 2. 单元测试（默认必须带 DSN，且不允许静默 SKIP）
step "go test ./..."
if [ -z "${ARTEX_PG_DSN:-}" ]; then
  if [ "$NO_DB" = "1" ]; then
    # 本机没有 PostgreSQL 时的退路：只跑不依赖数据库的包，并明确标注 db/server 未验证。
    warn "未设置 ARTEX_PG_DSN，按 --no-db 只跑非数据库包；db/ 与 server/ 本次未验证"
    PKGS="$(go list ./... | grep -v -E '/(db|server)$')"
    if go test $PKGS ; then ok "非数据库包测试通过"; else bad "非数据库包测试失败"; fi
  else
    bad "未设置 ARTEX_PG_DSN：db/ 与 server/ 的用例会被静默跳过，这种绿没有意义（本机无 PG 时可加 --no-db 明确降级）"
  fi
else
  TEST_LOG="$(mktemp)"
  if go test ./... 2>&1 | tee "$TEST_LOG"; then
    if grep -qE '^(---|    ---) SKIP' "$TEST_LOG"; then
      bad "测试输出含 SKIP（用例被跳过），逐条确认后再放行："
      grep -E '^(---|    ---) SKIP' "$TEST_LOG" | sed 's/^/    /'
    else
      ok "测试通过且无 SKIP"
    fi
  else
    bad "测试失败"
  fi
  rm -f "$TEST_LOG"
fi

# 3. settings 键读写一致性（拼错的键会让功能静默失效，见 scripts/settings_contract.py）
step "settings 键一致性"
if command -v python3 >/dev/null 2>&1; then
  PY=python3
elif command -v python >/dev/null 2>&1; then
  PY=python
else
  PY=""
fi
if [ -z "$PY" ]; then
  warn "没有 python，跳过 settings 键检查"
elif "$PY" scripts/settings_contract.py; then
  ok "settings 键读写一致"
else
  bad "存在只读不写的 settings 键"
fi

# 4. 可选竞态检测
if [ "$WITH_RACE" = "1" ]; then
  step "go test -race ./agent/... ./db/... ./server/..."
  if go test -race ./agent/... ./db/... ./server/... ; then ok "race 检测通过"; else bad "race 检测失败"; fi
fi

# 5. 可选前端构建（Next 静态导出 → server/webui，最终由 go:embed 打包）
if [ "$WITH_WEB" = "1" ]; then
  step "web 构建"
  if [ ! -d web/node_modules ]; then
    bad "web/node_modules 不存在，先执行 (cd web && npm ci)"
  elif (cd web && npm run build); then
    ok "前端构建通过"
  else
    bad "前端构建失败"
  fi
fi

step "结果"
if [ "$fail" = "0" ]; then
  echo "  全部通过"
  exit 0
fi
echo "  存在失败项，见上方 [FAIL]"
exit 1
