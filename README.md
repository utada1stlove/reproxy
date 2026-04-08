# reproxy

`reproxy` 是一个面向入口机的轻量级 edge-proxy-manager。用户只需要提供域名、目标 IP、目标端口，系统就会持久化路由定义、生成 Nginx 反向代理配置，并在配置好证书钩子后支持 HTTPS 自动化。

## 项目定位

MVP 只解决最核心的一件事：
- 把“域名 -> 后端 IP:端口”的映射可靠地落盘
- 把这份映射自动转换成可运维、可审查的 Nginx 配置
- 给 HTTPS 留出自动化接口，但不把 ACME 协议本身塞进管理器里

当前实现遵循几个原则：
- 轻量优先：Go 单二进制，标准库实现
- 易维护优先：本地 JSON 状态文件 + 明文生成配置
- MVP 优先：先做单机、单进程、单入口机场景

## 架构概览

核心组件如下：

1. `reproxy` API 服务
   接收路由录入请求，校验输入，更新本地状态。

2. JSON 状态存储
   路由定义保存在 `data/routes.json`，它是系统的 source of truth。

3. Nginx 配置渲染器
   根据路由定义生成 `deployments/nginx/reproxy.conf`。

4. HTTPS 自动化钩子
   通过可选的 `REPROXY_CERT_COMMAND_TEMPLATE` 调用外部 ACME 工具，例如 `certbot`。

5. Nginx 校验与 reload 钩子
   通过可选的 `REPROXY_VALIDATE_COMMAND` 先做配置预检查，再通过 `REPROXY_RELOAD_COMMAND` 触发 reload。

运行时流程：

1. 启动服务
2. 读取 `data/routes.json`
3. 检查每个域名的证书文件是否存在
4. 生成 `deployments/nginx/reproxy.conf`
5. 如已配置，则执行证书申请命令、Nginx 配置校验和 reload 命令
6. 记录最近一次同步、校验、reload、证书申请状态
7. 对外暴露 `/healthz`、`/status` 与 `/routes`

当证书未就绪时，MVP 会先生成 HTTP server block，并保留 ACME challenge 路径；证书文件存在后，会自动生成 443 server block，并把 80 跳转到 HTTPS。

## 目录结构

```text
reproxy/
├── cmd/reproxy/             # 应用入口
├── internal/app/            # 路由校验与编排
├── internal/httpapi/        # HTTP API
├── internal/nginx/          # Nginx 渲染与同步
├── internal/runtime/        # 环境变量配置
├── internal/store/          # JSON 文件存储
├── data/                    # MVP 状态文件目录
├── deployments/nginx/       # 生成的 Nginx 配置目录
└── codex-action/            # 计划与进度记录
```

## 启动方式

### 0. GitHub 一键安装

如果你要在目标机器上直接安装，推荐这一条：

```bash
curl -fsSL https://raw.githubusercontent.com/utada1stlove/reproxy/main/deployments/bootstrap.sh | sudo bash
```

这条命令会尝试：
- 从 GitHub 下载当前仓库源码
- 安装缺失依赖，例如 `go`、`nginx`、`certbot`
- 执行项目安装脚本
- 安装并启动 `reproxy.service`

常用可选变量：

```bash
curl -fsSL https://raw.githubusercontent.com/utada1stlove/reproxy/main/deployments/bootstrap.sh | \
  sudo env REPROXY_REPO_REF=main REPROXY_INSTALL_DIR=/opt/reproxy bash
```

如果你只想安装但先不启动服务：

```bash
curl -fsSL https://raw.githubusercontent.com/utada1stlove/reproxy/main/deployments/bootstrap.sh | \
  sudo env REPROXY_SKIP_START=1 bash
```

### 1. 构建或直接运行

```bash
go run ./cmd/reproxy
```

或：

```bash
go build -o bin/reproxy ./cmd/reproxy
./bin/reproxy
```

或使用项目任务：

```bash
make fmt
make test
make build
```

### 2. 推荐环境变量

```bash
export REPROXY_LISTEN_ADDR=:8080
export REPROXY_STORAGE_PATH=./data/routes.json
export REPROXY_NGINX_CONFIG_PATH=./deployments/nginx/reproxy.conf
export REPROXY_ACME_WEBROOT=/tmp/reproxy-acme
export REPROXY_CERTS_DIR=/etc/letsencrypt/live
export REPROXY_ADMIN_EMAIL=ops@example.com
export REPROXY_CERT_COMMAND_TEMPLATE='certbot certonly --webroot -w {{.Webroot}} -d {{.Domain}} --email {{.Email}} --agree-tos --non-interactive'
export REPROXY_VALIDATE_COMMAND='nginx -t'
export REPROXY_RELOAD_COMMAND='nginx -s reload'
```

本地只做骨架验证时，可以先不设置 `REPROXY_CERT_COMMAND_TEMPLATE`、`REPROXY_VALIDATE_COMMAND` 和 `REPROXY_RELOAD_COMMAND`。这样系统仍然会生成 HTTP 配置，只是不自动申请证书，也不自动校验和 reload Nginx。

### 3. Nginx 接入方式

把生成文件 include 到 Nginx 的 `http {}` 块里，例如：

```nginx
http {
    include /path/to/reproxy/deployments/nginx/reproxy.conf;
}
```

## API

### 健康检查

```bash
curl http://127.0.0.1:8080/healthz
```

### 服务状态

```bash
curl http://127.0.0.1:8080/status
```

`/status` 会返回：
- 路由总数
- TLS ready 路由数
- 最近一次同步是否成功
- 最近一次校验、reload、证书申请的时间和错误

### 查看路由

```bash
curl http://127.0.0.1:8080/routes
```

现在 `GET /routes` 会同时返回：
- 路由基础字段
- `tls_ready`
- 当前证书路径和私钥路径

### 查看单个路由

```bash
curl http://127.0.0.1:8080/routes/demo.example.com
```

### 创建或更新路由

```bash
curl -X POST http://127.0.0.1:8080/routes \
  -H 'Content-Type: application/json' \
  -d '{
    "domain": "demo.example.com",
    "target_ip": "10.0.0.12",
    "target_port": 8080
  }'
```

`POST /routes` 在 MVP 中按域名做 upsert：
- 域名不存在时创建
- 域名已存在时更新目标 IP 和端口

### 删除路由

```bash
curl -X DELETE http://127.0.0.1:8080/routes/demo.example.com
```

删除成功后，系统会重新生成 Nginx 配置，并在已配置的情况下继续执行校验和 reload。

## 部署样例

- `systemd` 服务样例：`deployments/systemd/reproxy.service`
- 环境文件样例：`deployments/env/reproxy.env.example`
- 安装脚本：`deployments/install.sh`
- GitHub bootstrap 脚本：`deployments/bootstrap.sh`
- Nginx include 样例：`deployments/nginx/reproxy.http.include.example`

安装脚本会优先复用现成的 `bin/reproxy`；如果不存在，则使用本地 Go 工具链直接构建再安装。
GitHub bootstrap 脚本会先拉源码，再调用安装脚本，并在默认情况下启动 systemd 服务。

建议部署步骤：

1. 运行 `bash deployments/install.sh`
2. 编辑生成的环境文件
3. 把 `deployments/nginx/reproxy.http.include.example` 的 include 路径接入 Nginx 主配置
4. 执行 `systemctl daemon-reload && systemctl enable --now reproxy`
5. 用 `curl /status` 确认服务状态

当前提供的 `systemd` 样例默认以 `root` 运行，因为它通常需要写入 Nginx 配置、访问证书目录并触发 reload。后续如果改成受限用户运行，需要同时调整文件权限和 reload 策略。

## 关键配置项

- `REPROXY_LISTEN_ADDR`: API 监听地址，默认 `:8080`
- `REPROXY_STORAGE_PATH`: 路由状态文件，默认 `data/routes.json`
- `REPROXY_NGINX_CONFIG_PATH`: 生成的 Nginx 配置路径，默认 `deployments/nginx/reproxy.conf`
- `REPROXY_ACME_WEBROOT`: ACME challenge 目录，默认 `/var/www/reproxy-acme`
- `REPROXY_CERTS_DIR`: 证书目录根路径，默认 `/etc/letsencrypt/live`
- `REPROXY_ADMIN_EMAIL`: 证书申请邮箱
- `REPROXY_CERT_COMMAND_TEMPLATE`: 证书申请命令模板
- `REPROXY_CERT_FILE_TEMPLATE`: 证书文件模板，默认 `{{.CertsDir}}/{{.Domain}}/fullchain.pem`
- `REPROXY_CERT_KEY_TEMPLATE`: 私钥文件模板，默认 `{{.CertsDir}}/{{.Domain}}/privkey.pem`
- `REPROXY_VALIDATE_COMMAND`: reload 前的校验命令，例如 `nginx -t`
- `REPROXY_RELOAD_COMMAND`: Nginx 重载命令

## 当前 MVP 边界

- 没有认证、授权和多租户
- 没有数据库和集群同步
- 默认证书路径按 `certbot` 目录结构设计
- HTTPS 自动化依赖外部 ACME 工具，而不是内建实现
- 当前同步状态保存在进程内存中，进程重启后会从新的同步周期重新统计

## 后续待办方向

- 支持更丰富的反向代理参数，例如 header、path、健康检查
- 支持 `nginx -t` 失败时更细粒度的错误回传
- 增加认证和审计日志
