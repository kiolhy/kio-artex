#!/usr/bin/env bash
# docker-local.sh —— 本机一条命令把 ARTEX 跑起来（compose + 自带 PostgreSQL）。
#
# 相比手写 docker compose 命令，这里多做三件事：
#   1. 没有 .env 就生成一份，并把 POSTGRES_PASSWORD 换成随机值（不给弱口令起库的机会）；
#   2. 等健康检查真正通过再报成功，而不是 up -d 完就说好了；
#   3. 打印的地址是"实际绑定的地址"，避免以为起了 0.0.0.0 其实只听回环。
set -euo pipefail

cd "$(dirname "$0")/.."

usage() {
  cat <<'EOF'
用法：./scripts/docker-local.sh <命令>

  up        生成 .env（若缺）并启动，等健康检查通过
  down      停止并移除容器（保留数据卷与 data/）
  restart   重启 artex 容器
  status    查看容器状态与健康
  logs      跟踪 artex 日志
  rebuild   本地重新构建镜像后重启（改了代码用这个）
  reset     停服并删除数据卷（会丢数据库，需二次确认）
EOF
}

need_docker() {
  command -v docker >/dev/null 2>&1 || {
    echo "没有找到 docker。离线环境请改用 scripts/build-offline.sh 出的包。" >&2
    exit 1
  }
  docker compose version >/dev/null 2>&1 || {
    echo "需要 docker compose v2（docker compose 子命令）。" >&2
    exit 1
  }
}

rand_pw() {
  # 优先 openssl，其次 /dev/urandom，最后退化成时间戳（仅保证不重复）
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 24 | tr -d '/+=' | cut -c1-24
  elif [ -r /dev/urandom ]; then
    LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 24
  else
    date +%s%N | cut -c1-24
  fi
}

ensure_env() {
  if [ -f .env ]; then
    return 0
  fi
  [ -f deploy/.env.docker.example ] || { echo "缺少 deploy/.env.docker.example" >&2; exit 1; }
  pw="$(rand_pw)"
  # 只替换密码那一行，其余保持模板原样（含注释，方便现场改）。
  awk -v pw="$pw" '
    /^POSTGRES_PASSWORD=/ { print "POSTGRES_PASSWORD=" pw; next }
    { print }
  ' deploy/.env.docker.example > .env
  echo "已生成 .env（POSTGRES_PASSWORD 为随机值，可用 vi .env 查看/修改）"
}

env_val() {
  # 从 .env 读一个键，缺省给默认值
  key="$1"; def="$2"
  if [ -f .env ]; then
    v="$(grep -E "^${key}=" .env | tail -n 1 | cut -d= -f2- || true)"
    [ -n "${v:-}" ] && { printf '%s' "$v"; return 0; }
  fi
  printf '%s' "$def"
}

wait_healthy() {
  host="$(env_val ARTEX_BIND_HOST 127.0.0.1)"
  port="$(env_val ARTEX_PORT 8787)"
  # 绑在 0.0.0.0 时用回环探测，绑在具体地址时用那个地址
  probe_host="$host"
  [ "$host" = "0.0.0.0" ] && probe_host="127.0.0.1"
  url="http://${probe_host}:${port}/api/health"
  echo "等待健康检查：$url"
  i=0
  while [ "$i" -lt 60 ]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      echo "启动成功：http://${probe_host}:${port}/"
      case "$host" in
        127.0.0.1|localhost|::1) echo "（当前只监听回环：仅本机可访问）" ;;
        *) echo "（监听 ${host}：局域网可访问，请自行确认访问控制）" ;;
      esac
      return 0
    fi
    sleep 2
    i=$((i + 1))
  done
  echo "120 秒内健康检查未通过，最近日志：" >&2
  docker compose logs --tail=40 artex >&2 || true
  return 1
}

cmd="${1:-up}"
need_docker
case "$cmd" in
  up)
    ensure_env
    docker compose up -d
    wait_healthy
    ;;
  down)
    docker compose down
    echo "已停止（数据卷 pgdata 与 ./data 保留）"
    ;;
  restart)
    docker compose restart artex
    wait_healthy
    ;;
  status)
    docker compose ps
    ;;
  logs)
    docker compose logs -f --tail=100 artex
    ;;
  rebuild)
    ensure_env
    docker compose build artex
    docker compose up -d
    wait_healthy
    ;;
  reset)
    printf '这会删除数据库数据卷与 ./data，确定？输入 yes 继续：'
    read -r ans
    [ "$ans" = "yes" ] || { echo "已取消"; exit 1; }
    docker compose down -v
    echo "已清空数据卷（./data 目录未动，如需清空请手动删除）"
    ;;
  -h|--help|help|'') usage ;;
  *) echo "未知命令：$cmd" >&2; usage; exit 2 ;;
esac
