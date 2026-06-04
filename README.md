# gobao-user

GoBao 的用户服务，负责注册、登录、获取当前用户和 JWT 相关能力。

## 作用

- 用户注册
- 用户登录
- 当前用户查询
- JWT 签发与校验基础能力

## 关系

- 依赖 `gobao-proto`、`gobao-pkg`
- 被 `gobao-gateway` 调用

## 独立使用前准备

单独 clone 本仓后，先执行：

```bash
bash scripts/bootstrap-deps.sh
ln -sfn workspace/gobao-pkg ../gobao-pkg
ln -sfn workspace/gobao-proto ../gobao-proto
```

## 环境变量

可参考仓库根目录 `.env.example`：

- `USER_MYSQL_DSN`
- `USER_REDIS_ADDR`
- `USER_JWT_SECRET`
- `USER_JWT_EXPIRY`

## 启动

```bash
go test ./...
go run ./cmd/server
```

如需容器化启动，可直接使用仓库内 `Dockerfile`，或由 `gobao-deploy` / `GoBao` 主仓统一编排。
