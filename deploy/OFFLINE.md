# ARTEX 离线包说明

解压即用，不需要 Go / Node / Docker。整个包就这些内容：

```
artex-<版本>-<os>-<arch>/
  artex                       主程序（前端已内嵌，单文件）
  artexctl                    启停/升级脚本
  skills/                     内置 skill（运行时从文件系统加载，别删）
  deploy/.env.offline.example 环境模板
  run/  logs/                 PID 与日志
```

## 三步起起来

```sh
cp deploy/.env.offline.example deploy/.env
vi deploy/.env          # 至少填 ARTEX_PG_DSN，以及模型接入
./artexctl start
```

`artexctl start` 会做健康检查（`/api/health`），30 秒内不通就打印最近日志并退出，
不会留下一个"看着在跑其实没起来"的进程。

## 数据库

需要一个 PostgreSQL（13 及以上，推荐 16）。库和表不用手工建：程序启动时会自动建库、
建表、跑迁移，全部幂等。

```sql
CREATE ROLE artex LOGIN PASSWORD 'CHANGE_ME';
CREATE DATABASE artex OWNER artex;
```

## 访问与暴露

管理面默认只监听 `127.0.0.1:8787`——里面有凭据、有全部战果，不该默认对网开放。
要让同事访问，两种做法选一种：

1. 前面放反向代理（Nginx/Caddy）做 TLS + 认证，ARTEX 继续只听回环；
2. 设 `ARTEX_BIND_HOST=0.0.0.0` 与 `ARTEX_ALLOW_REMOTE_ACCESS=1`，同时用防火墙
   限定来源网段。

## 升级

```sh
./artexctl upgrade /path/to/artex-<新版本>     # 有 .sha256 时会先校验
```

脚本会：校验和 → 停服 → 备份旧二进制（`artex.bak.<时间戳>`）→ 换新 → 启动并健康检查；
起不来自动回滚到旧件再启动。手工回滚就是把 `.bak.*` 复制回 `artex`。

## 常见问题

- **起不来，日志里全是连不上数据库**：`deploy/.env` 里的 `ARTEX_PG_DSN` 没填或密码错。
  `./artexctl env` 可以从模板重新生成一份。
- **页面能开但模型调用失败**：模型配置在界面里（不在 env），首次进入后到「模型」页配置；
  内网网关记得 base url 带 `/v1`。
- **抓 HTTPS 站点报证书错误**：这是默认行为（失败即失败，不静默透传）。把公司 CA 装进
  系统信任库；临时排障可设 `ARTEX_TRAFFIC_INSECURE_TLS=1`。
- **Agent 里执行命令拿不到自己的 token**：默认只摘掉平台凭据；需要连自己的 token 也清掉
  时开 `ARTEX_BASH_ENV_STRICT=1`。
