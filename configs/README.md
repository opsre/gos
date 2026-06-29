# Config Usage

## 基本原则

- 不要把真实密码、Token、DSN、内网 IP 或生产地址提交到仓库。
- 服务端直接运行时读取 JSON 配置文件；Docker 镜像由 `docker/entrypoint.sh` 从环境变量生成运行时配置。
- 默认启动配置文件是 `configs/config.local.json`。
- 需要切换配置时，使用启动参数 `-config` 指定文件路径。

## Local

```bash
go run ./cmd/server
```

默认会读取：

- `configs/config.local.json`

本地配置默认使用 SQLite，方便不依赖外部数据库启动。

如需连接 MySQL，请复制配置文件到本地未提交文件中，再填入真实 DSN：

```bash
cp configs/config.local.json configs/config.local.private.json
go run ./cmd/server -config configs/config.local.private.json
```

## Production

先复制并编辑本地私有配置：

- `configs/config.production.json`

重点检查：

- `database.mysql_dsn`
- `jenkins.*`
- `release.*`
- `auth.admin_password`
- `security.encryption_key`

然后启动：

```bash
go run ./cmd/server -config configs/config.production.json
```

## Notes

- 生产环境请在部署系统、密钥管理系统或本地私有配置中保存真实值，不要提交到 Git。
- `security.encryption_key` 用于加密 Agent Token、GitOps / ArgoCD 凭据、通知源 Secret。
- `config.production.json` 中的敏感字段默认留空，启动前必须在私有配置中填写。
- 当 `jenkins.enabled=true` 且 `jenkins.startup_check_enabled=true`，服务启动时会先检查 Jenkins 连通性。
- ArgoCD 与 GitOps 实例改为数据库管理，不再要求在配置文件中维护默认实例。
- 服务会后台定时拉取数据库中 `active` 的 ArgoCD 实例应用信息并写入数据库。
