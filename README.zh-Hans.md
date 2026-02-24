[English](README.md) | [简体中文](README.zh-Hans.md) | [繁體中文](README.zh-Hant.md) | [日本語](README.ja.md)

![ApiNatsBridge](ico/icon.png)

# ApiNatsBridge

轻量级 HTTP-to-NATS 网关桥接器，将标准 HTTP REST 请求转换为 NATS 消息并转发至后端微服务，再将微服务的响应返回给 HTTP 客户端。适用于微服务架构中作为 API 网关层使用。

## 功能特性

- **HTTP 到 NATS 请求转发** — 接收 HTTP 请求，序列化为 JSON 结构体后通过 NATS Request/Reply 模式发送到后端微服务
- **YAML 声明式路由配置** — 通过配置文件定义 HTTP 路径与 NATS Subject 的映射关系
- **JSON Schema 请求体校验** — 支持对请求体进行 JSON Schema 验证，拒绝不合规请求
- **表单到 JSON 自动转换** — `application/x-www-form-urlencoded` 表单数据可依据 Schema 进行类型强转后作为 JSON 转发
- **选择性字段转发** — 通过 `return_fields` 配置仅将需要的请求信息传递给微服务
- **请求字段长度限制** — 全局及路由级别的路径、标头、Cookie、参数、请求体长度限制
- **AES 加密 NATS 通信** — 支持全局密钥和按 Subject 独立密钥的 AES 对称加密
- **CDN 真实 IP 解析** — 支持 Cloudflare、Akamai、Fastly、AWS CloudFront、阿里云 CDN 等主流 CDN 的真实客户端 IP 提取
- **自动 UUID Cookie 生成** — 为客户端自动生成跟踪用 UUID Cookie
- **IP 速率限制** — 内置 IP 维度的请求频率限制与封禁机制
- **TLS/HTTPS 支持** — 配置证书即可启用 HTTPS
- **`/ping` 端点** — 通过 NATS 微服务实现的延迟测量端点
- **IP 白名单错误详情** — 仅允许指定 IP 查看详细错误信息，生产环境仅返回通用错误
- **多国语言支持 (i18n/l10n)** — 日志、HTTP 响应、CLI 帮助文本均支持多语言，可在配置文件中分别设置（支持 en、zh、zh_Hant、ja）
- **优雅关闭** — 捕获系统信号后有序关闭 HTTP 服务器、取消 NATS 订阅并断开连接

## 架构概览

```
┌──────────┐         ┌───────────────────┐         ┌────────────────┐
│  HTTP    │  HTTP   │  ApiNatsBridge    │  NATS   │  Microservice  │
│  Client  │ ──────> │  (Gateway/Bridge) │ ──────> │  (Backend)     │
│          │ <────── │                   │ <────── │                │
└──────────┘         └───────────────────┘         └────────────────┘
```

1. HTTP 客户端发送请求到 ApiNatsBridge
2. ApiNatsBridge 进行路由匹配、方法校验、Content-Type 校验、长度限制校验、Schema 校验
3. 将请求数据序列化为 `BridgeRequest` JSON，通过 NATS Request 发送到对应 Subject
4. 后端微服务处理请求后返回 `BridgeResponse` JSON
5. ApiNatsBridge 将响应内容作为 HTTP 响应返回给客户端

## 日志模块前缀

运行时日志输出使用以下前缀区分来源模块：

| 前缀            | 来源文件            | 颜色   | 用途                  |
| --------------- | ------------------- | ------ | --------------------- |
| `[MAIN]`        | `src/logger.go`     | Cyan   | 主流程生命周期日志    |
| `[NATS]`        | `src/natsLogger.go` | Green  | NATS 客户端连接与事件 |
| `[BRIDGE]`      | `src/logger.go`     | Yellow | 桥接路由与转发日志    |
| `[HTTP]`        | `src/logger.go`     | Blue   | HTTP 请求日志行       |
| `[HTTPSTAT]`    | `src/logger.go`     | Purple | HTTP 服务器运行时统计 |
| `[MODULE]`      | `src/logger.go`     | Cyan   | 通用模块日志          |
| `[NATS][ERROR]` | `src/logger.go`     | Red    | NATS 连接错误         |
| `[HTTP][ERROR]` | `src/logger.go`     | Red    | HTTP 服务器错误       |
| `[MAIN][ERROR]` | `src/logger.go`     | Red    | 主流程致命错误        |

所有前缀均通过本地库 `libNyaruko_Go/nyalog` 的 `LogCC()` 函数输出。

## 数据结构

### BridgeRequest（发送给微服务）

```json
{
  "method": "POST",
  "path": "/test",
  "headers": { "Content-Type": "application/json", "...": "..." },
  "cookies": { "session_id": "abc123" },
  "remote_addr": "192.168.1.100:54321",
  "ip": "203.0.113.50",
  "params": { "key": "value" },
  "body": "{\"message\":\"hello\"}"
}
```

### BridgeResponse（微服务返回）

```json
{
  "status_code": 200,
  "headers": { "Content-Type": "application/json; charset=utf-8" },
  "body": "{\"result\":\"success\"}"
}
```

## 安装与编译

### 前置条件

- Go 1.24.4 或更高版本
- 本项目使用 Git 子模块管理依赖，克隆后需初始化子模块（详见下方）

### 初始化 Git 子模块

本项目包含以下 Git 子模块：

| 子模块                                                                         | 路径                     | 说明                                               |
| ------------------------------------------------------------------------------ | ------------------------ | -------------------------------------------------- |
| [libNyaruko_Go](https://github.com/kagurazakayashi/libNyaruko_Go)              | `libNyaruko_Go/`         | 依赖库（`nyalog`、`nyanats`、`nyaapiserver` 模块） |
| [ApiNatsBridgeTemplate](https://github.com/MasaeProject/ApiNatsBridgeTemplate) | `ApiNatsBridgeTemplate/` | 微服务模板项目                                     |

克隆时一并拉取子模块：

```bash
git clone --recursive <repo_url>
```

已克隆的项目初始化子模块：

```bash
git submodule init
git submodule update
```

或合并为一条命令：

```bash
git submodule update --init
```

### 编译 go-gen-l10n 工具

子模块 `libNyaruko_Go` 中包含本地化代码生成工具 `go-gen-l10n`。进入其目录，生成资源并编译：

```bash
# 进入子模块目录
cd libNyaruko_Go/go-gen-l10n

# 生成 Windows 资源（如适用），然后编译
go generate .
go build .

# 将二进制复制到项目根目录（以便 go generate ./src/l10nGlobal.go 能找到它）
# Linux / macOS
cd ../..
cp libNyaruko_Go/go-gen-l10n/go-gen-l10n .

# Windows
cd ..\..
copy libNyaruko_Go\go-gen-l10n\go-gen-l10n.exe .
```

编译后会在项目根目录生成 `go-gen-l10n`（或 `go-gen-l10n.exe`），修改 `l10n/app_*.arb` 语言文件后执行以下命令重新生成代码：

```bash
# Linux / macOS
./go-gen-l10n -dir ./l10n -pkg l10n -lang zh_Hant

# Windows
.\go-gen-l10n.exe -dir .\l10n -pkg l10n -lang zh_Hant
```

或使用 `go generate`：

```bash
go generate ./src/l10nGlobal.go
```

### Windows 可执行文件图标嵌入

首先安装 `go-winres` 工具：

```bash
go install github.com/tc-hib/go-winres@latest
```

然后生成资源文件（`.syso`），之后 `go build` 会自动链接：

```bash
# 通过 go generate 自动调用 go-winres（推荐）
go generate ./...

# 或手动执行
go-winres make
```

> 资源配置文件位于 `winres/winres.json`，图标源文件位于 `ico/icon.png`。
> `.syso` 文件已在 `.gitignore` 中被忽略，每次构建前需重新生成。
> 此步骤为可选——`go run .` 无需 `.syso` 文件也可正常运行。

### 本平台编译

```bash
# 先生成图标资源（Windows 平台需要，其他平台可跳过）
go generate ./...

# 编译（ Windows 的话加上 .exe ）
go build -o ApiNatsBridge .
```

### 多平台交叉编译

#### Linux (amd64)

```bash
GOOS=linux GOARCH=amd64 go build -o ApiNatsBridge-linux-amd64 .
```

#### Linux (arm64)

```bash
GOOS=linux GOARCH=arm64 go build -o ApiNatsBridge-linux-arm64 .
```

#### macOS (amd64)

```bash
GOOS=darwin GOARCH=amd64 go build -o ApiNatsBridge-darwin-amd64 .
```

#### macOS (Apple Silicon / arm64)

```bash
GOOS=darwin GOARCH=arm64 go build -o ApiNatsBridge-darwin-arm64 .
```

#### Windows (amd64)

```bash
GOOS=windows GOARCH=amd64 go build -o ApiNatsBridge-windows-amd64.exe .
```

#### Windows (arm64)

```bash
GOOS=windows GOARCH=arm64 go build -o ApiNatsBridge-windows-arm64.exe .
```

#### FreeBSD (amd64)

```bash
GOOS=freebsd GOARCH=amd64 go build -o ApiNatsBridge-freebsd-amd64 .
```

> **提示：** 在 Windows PowerShell 下设置环境变量使用 `$env:GOOS="linux"; $env:GOARCH="amd64"` 后再执行 `go build`。

### 批量多平台编译

使用提供的编译脚本一键编译所有支持的平台：

**Windows：**

```bat
build.bat
```

**Linux / macOS：**

```bash
chmod +x build.sh
./build.sh
```

> **注意：** 如果需要输出 HTML 格式的自述文件，请先安装 Python `markdown` 包：
>
> ```bash
> pip install markdown
> ```

## 使用方法

### 命令行参数

| 参数        | 说明                                                                       |
| ----------- | -------------------------------------------------------------------------- |
| `-c <路径>` | 指定 YAML 配置文件路径。若未指定，默认读取与可执行文件同名的 `.yaml` 文件  |
| `-v`        | 详细模式。输出完整的请求/响应数据（标头、参数、Cookie、Schema 校验错误等） |
| `-o <路径>` | 将所有日志输出到指定文件（同时仍会输出到控制台和各模块日志文件）           |

### 启动示例

```bash
# 使用默认配置文件（与可执行文件同名的 .yaml）
./ApiNatsBridge

# 指定配置文件
./ApiNatsBridge -c /etc/apibridge/config.yaml

# 详细模式
./ApiNatsBridge -c config.yaml -v

# 将所有日志输出到统一文件
./ApiNatsBridge -c config.yaml -o ../logs/all.log
```

## 配置文件详解

配置文件采用 YAML 格式，包含以下四个主要部分。

### 完整配置示例

```yaml
# ===========================================================
# ApiNatsBridge 配置文件
# ===========================================================

# --- HTTP API 服务器配置 ---
httpapiserver_config:
  # 监听地址（设为 0.0.0.0 则监听所有网卡）
  httpapiserver_host: "127.0.0.1"
  # 监听端口
  httpapiserver_port: 9080

  # TLS 证书路径（两项均填写时启用 HTTPS，留空则使用 HTTP）
  httpapiserver_tls_cert_file: ""
  httpapiserver_tls_key_file: ""

  # 超时设置（秒）
  httpapiserver_read_timeout: 5 # 读取请求超时
  httpapiserver_write_timeout: 30 # 写入响应超时
  httpapiserver_idle_timeout: 60 # 空闲连接超时

  # IP 速率限制
  httpapiserver_enable_rate_limit: true # 是否启用速率限制
  httpapiserver_limit_requests: 50 # 每个时间窗口内允许的最大请求数
  httpapiserver_limit_window: 1 # 时间窗口长度（秒）
  httpapiserver_block_duration: 600 # 超出限制后封禁时长（秒）

# --- NATS 客户端配置 ---
nats_config:
  # NATS 服务器地址和端口
  nats_server_host: 127.0.0.1
  nats_server_port: 4222

  # 连接认证（留空则不启用认证）
  nats_user: webapi
  nats_password: your_nats_password_here

  # 客户端标识名称（在 NATS 服务器端可见）
  nats_client_name: ApiNatsBridge

  # 重连策略
  nats_max_reconnects: 5 # 最大重连次数
  nats_reconnect_wait: 2 # 重连等待间隔（秒）
  nats_connect_timeout: 10 # 初次连接超时（秒）

  # AES 对称加密密钥（长度必须为 16、24 或 32 字节）
  # 留空则明文传输 NATS 消息
  nats_encryption_key: "YOUR_32_CHAR_ENCRYPTION_KEY_HERE!"

  # 按 Subject 独立设置加密密钥（优先于全局密钥）
  # 设为空字符串表示该 Subject 使用明文传输
  nats_theme_keys:
    "sensitive.subject": "PER_SUBJECT_KEY_32_CHARS_LONG!!!"
    "public.subject": ""

# --- 桥接层配置 ---
bridge:
  # 多国语言配置（支持 en、zh、zh_Hant、ja）
  language:
    log: "zh_Hant" # 日志输出语言
    http: "en" # HTTP 响应错误消息语言
    cli: "zh_Hant" # 命令行相关消息语言

  # 日志输出配置
  log:
    stdout: true # 是否同时输出到控制台，设为 false 则仅写入日志文件
    debug: true # 是否启用调试等级日志，设为 false 则仅输出 Info 及以上等级
    overwrite: false # 是否使用覆盖模式，设为 true 则启动时清空现有日志文件，设为 false 或不提供则仅追加
    color: true # 是否使用彩色控制台输出，设为 true 或不提供则使用彩色，设为 false 则纯文字
    files:
      # 各模块独立日志文件路径，可分别设定
      # 留空或不填则该模块不写入文件
      main: "logs/main.log" # 主流程日志
      bridge: "logs/bridge.log" # 桥接路由与转发日志
      http: "logs/http.log" # HTTP 请求日志
      nats: "logs/nats.log" # NATS 客户端事件日志
      httpstat: "logs/httpstat.log" # HTTP 服务器运行统计日志
      module: "logs/module.log" # 通用模块日志

  # 时区，影响所有日志时间戳，支持 IANA 时区名称（如 Asia/Shanghai）或小时偏移（如 8、-5）
  timezone: "Asia/Shanghai"

  # CDN 真实 IP 标头列表（按优先级排列）
  # 用于从 CDN 代理请求中提取客户端真实 IP 地址
  cdnheader:
    - "CF-Connecting-IP" # Cloudflare
    - "True-Client-IP" # Akamai / Cloudflare Enterprise
    - "Fastly-Client-IP" # Fastly
    - "CloudFront-Viewer-Address" # AWS CloudFront
    - "CDN-Viewer-IP" # Google Cloud CDN
    - "X-Azure-ClientIP" # Azure CDN
    - "Incap-Client-IP" # Imperva / Incapsula
    - "X-Sucuri-ClientIP" # Sucuri
    - "X-SP-Forwarding-IP" # StackPath
    - "Ali-Cdn-Real-Ip" # 阿里云 CDN
    - "Ar-Real-IP" # ArvanCloud

  # 允许查看详细错误信息的 IP 白名单（用于开发调试）
  # 不在此列表中的 IP 仅收到通用错误提示
  error_detail_ips:
    - "127.0.0.1"
    - "::1"

  # 全局请求字段长度限制（0 或省略表示不限制）
  limits:
    path:
      max_length: 2048 # 请求路径最大字节长度
    headers:
      max_count: 64 # 请求标头最大数量
      max_key_length: 256 # 标头名称最大字节长度
      max_value_length: 4096 # 标头值最大字节长度
    cookies:
      max_count: 32 # Cookie 最大数量
      max_key_length: 256 # Cookie 名称最大字节长度
      max_value_length: 4096 # Cookie 值最大字节长度
    params:
      max_count: 64 # 参数最大数量
      max_key_length: 256 # 参数名称最大字节长度
      max_value_length: 4096 # 参数值最大字节长度
    body:
      max_length: 1048576 # 请求体最大字节长度（1MB）

  # 全局响应字段长度限制（结构同 limits，可被路由级别覆蓋）
  response_limits:
    body:
      max_length: 1048576 # 响应体最大字节长度（1MB）
    headers:
      max_count: 64 # 响应头最大数量
      max_key_length: 256 # 响应头名称最大字节长度
      max_value_length: 4096 # 响应头值最大字节长度

# --- 路由转发规则 ---
routes:
  - path: "/api/user"
    nats_subject: "user_service"
    methods: ["GET", "POST"]
    content_type: "application/json"
    timeout: 30
    return_fields:
      - method
      - path
      - headers
      - cookies
      - remote_addr
      - ip
      - params
      - body
    limits:
      body:
        max_length: 65536
    schema_body:
      root_type: object
      strict: true
      required:
        - username
      properties:
        username:
          type: string
        email:
          type: string
    # cookie_uuid_key: "brid"  # UUID Cookie 键名（可选）
    # http_code_key: "status_code"  # 回应 JSON 中 HTTP 状态码的键名（可选）
    # error_code_key: "error_code"  # 回应 JSON 中错误码的键名，检测到时触发 response_error_schema_body 验证（可选）
```

### 配置项详解

#### `httpapiserver_config` — HTTP 服务器配置

| 配置项                            | 类型   | 说明                                   |
| --------------------------------- | ------ | -------------------------------------- |
| `httpapiserver_host`              | string | 服务器监听地址，`0.0.0.0` 监听所有网卡 |
| `httpapiserver_port`              | int    | 监听端口                               |
| `httpapiserver_tls_cert_file`     | string | TLS 证书文件路径，留空使用 HTTP        |
| `httpapiserver_tls_key_file`      | string | TLS 私钥文件路径，留空使用 HTTP        |
| `httpapiserver_read_timeout`      | int    | 读取请求超时（秒）                     |
| `httpapiserver_write_timeout`     | int    | 写入响应超时（秒）                     |
| `httpapiserver_idle_timeout`      | int    | 空闲连接超时（秒）                     |
| `httpapiserver_enable_rate_limit` | bool   | 是否启用 IP 速率限制                   |
| `httpapiserver_limit_requests`    | int    | 时间窗口内最大请求数                   |
| `httpapiserver_limit_window`      | int    | 速率限制时间窗口（秒）                 |
| `httpapiserver_block_duration`    | int    | 超限后封禁时长（秒）                   |

#### `nats_config` — NATS 客户端配置

| 配置项                 | 类型   | 说明                                        |
| ---------------------- | ------ | ------------------------------------------- |
| `nats_server_host`     | string | NATS 服务器地址                             |
| `nats_server_port`     | int    | NATS 服务器端口                             |
| `nats_user`            | string | NATS 用户名，留空不认证                     |
| `nats_password`        | string | NATS 密码                                   |
| `nats_client_name`     | string | 连接标识名称                                |
| `nats_max_reconnects`  | int    | 最大重连次数                                |
| `nats_reconnect_wait`  | int    | 重连间隔（秒）                              |
| `nats_connect_timeout` | int    | 连接超时（秒）                              |
| `nats_encryption_key`  | string | AES 全局加密密钥（16/24/32 字节），留空明文 |
| `nats_theme_keys`      | map    | 按 Subject 独立设置的加密密钥               |

#### `bridge` — 桥接层配置

| 配置项             | 类型     | 说明                                                    |
| ------------------ | -------- | ------------------------------------------------------- |
| `language`         | object   | 多国语言配置（详见下方）                                |
| `log`              | object   | 日志输出配置（详见下方）                                |
| `timezone`         | string   | 时区，影响所有日志时间戳，如 `"Asia/Shanghai"` 或 `"8"` |
| `cdnheader`        | []string | CDN 真实 IP 标头优先级列表                              |
| `error_detail_ips` | []string | 允许查看详细错误的 IP 白名单                            |
| `limits`           | object   | 全局请求字段长度限制                                    |
| `response_limits`  | object   | 全局回应字段长度限制（结构同 limits）                   |

##### `bridge.language` — 多国语言配置

| 配置项 | 类型   | 默认值      | 说明                  |
| ------ | ------ | ----------- | --------------------- |
| `log`  | string | `"zh_Hant"` | 日志输出语言          |
| `http` | string | `"en"`      | HTTP 响应错误消息语言 |
| `cli`  | string | `"zh_Hant"` | 命令行相关消息语言    |

> 支持的语言代码：`en`（英语）、`zh`（简体中文）、`zh_Hant`（繁体中文）、`ja`（日语）。
>
> **重要：** 语言文本的修改应在翻译源文件 `l10n/app_*.arb` 中进行，**不要**直接编辑 `l10n/app_localizations_*.go` 生成文件，它们会在重新生成时被覆盖。
>
> 修改 `.arb` 文件后需运行以下命令重新生成 Go 代码：
>
> ```bash
> # Windows
> .\go-gen-l10n.exe -dir .\l10n -pkg l10n -lang zh_Hant
>
> # 或使用 go generate
> go generate .\src\l10nGlobal.go
> ```
>
> #### 语言风格约定
>
> | 语言代码  | 语言     | 风格                 |
> | --------- | -------- | -------------------- |
> | `zh`      | 简体中文 | 大陆风格（大陆简体） |
> | `zh_Hant` | 繁体中文 | 台湾风格（臺灣繁體） |
> | `en`      | 英语     | 标准                 |
> | `ja`      | 日语     | 标准                 |
>
> #### ARB 文件
>
> | 文件                   | 语言             |
> | ---------------------- | ---------------- |
> | `l10n/app_zh.arb`      | 简体中文（大陆） |
> | `l10n/app_zh_Hant.arb` | 繁体中文（台湾） |
> | `l10n/app_en.arb`      | 英语             |
> | `l10n/app_ja.arb`      | 日语             |

##### `bridge.log` — 日志输出配置

| 配置项      | 类型   | 说明                                                                  |
| ----------- | ------ | --------------------------------------------------------------------- |
| `stdout`    | bool   | 是否同时输出到控制台，`false` 则仅写文件                              |
| `debug`     | bool   | 是否启用调试等级日志，`false` 仅 Info+                                |
| `overwrite` | bool   | 是否覆盖模式，`true` 则启动时清空现有日志文件，`false` 或不提供仅追加 |
| `color`     | bool   | 是否彩色控制台输出，`true` 或不提供则彩色，`false` 则纯文字           |
| `files`     | object | 各模块独立日志文件路径（详见下方）                                    |

##### `bridge.log.files` — 模块日志文件路径

| 配置项     | 类型   | 说明                            |
| ---------- | ------ | ------------------------------- |
| `main`     | string | 主流程日志文件路径              |
| `bridge`   | string | 桥接路由与转发日志文件路径      |
| `http`     | string | HTTP 请求日志文件路径           |
| `nats`     | string | NATS 客户端事件日志文件路径     |
| `httpstat` | string | HTTP 服务器运行统计日志文件路径 |
| `module`   | string | 通用模块日志文件路径            |

> 日志文件路径为相对或绝对路径均可。目录不存在时会自动创建。
> 路径留空或不填则该模块不写入文件。若 `stdout: false` 且所有文件路径均为空，则该模块无日志输出。

#### `routes` — 路由规则

| 配置项                 | 类型     | 默认值         | 说明                                             |
| ---------------------- | -------- | -------------- | ------------------------------------------------ |
| `path`                 | string   | （必填）       | HTTP 请求路径                                    |
| `nats_subject`         | string   | （必填）       | 转发的 NATS Subject                              |
| `methods`              | []string | []（允许全部） | 允许的 HTTP 方法列表                             |
| `content_type`         | string   | ""（不校验）   | 要求的 Content-Type 前缀                         |
| `timeout`              | int      | 30             | NATS 响应超时（秒）                              |
| `return_fields`        | []string | []（返回全部） | 转发给微服务的字段选择                           |
| `limits`               | object   | -              | 路由级别长度限制（覆盖全局）                     |
| `schema_body`          | object   | -              | 请求体 JSON Schema 校验                          |
| `response_limits`      | object   | -              | 路由级别回应长度限制（覆盖全局 response_limits） |
| `response_schema_body` | object   | -              | 回应体 JSON Schema 校验（结构同 schema_body）    |
| `response_error_schema_body` | object   | -              | 错误回应体 JSON Schema 校验（检测到 error_code_key 时使用，未设定则沿用 response_schema_body） |
| `cookie_uuid_key`  | string   | ""（不启用）    | UUID Cookie 键名，留空不启用                                    |
| `http_code_key`    | string   | ""（不启用）    | 微服务返回 JSON 中表示 HTTP 状态码的键名（100-599）；用于检测 BridgeResponse 格式 |
| `error_code_key`   | string   | ""（不启用）    | 微服务返回 JSON 中表示错误码的键名（int32）；检测到此键时触发 response_error_schema_body 验证 |

#### `return_fields` 可选值

| 字段名        | 说明                             |
| ------------- | -------------------------------- |
| `method`      | HTTP 请求方法                    |
| `path`        | 请求路径                         |
| `headers`     | 请求标头（键值对）               |
| `cookies`     | Cookie（键值对）                 |
| `remote_addr` | 直连 TCP 地址（含端口）          |
| `ip`          | 解析后的真实客户端 IP            |
| `params`      | URL 查询参数和表单参数（键值对） |
| `body`        | 请求体原始内容                   |

#### `schema_body` JSON Schema 校验

除标准 JSON Schema 字段外，支持两个控制键：

| 控制键      | 类型   | 说明                                             |
| ----------- | ------ | ------------------------------------------------ |
| `root_type` | string | 根节点预期类型（如 `object`、`array`）           |
| `strict`    | bool   | 严格模式，为 `true` 时拒绝 Schema 中未定义的字段 |

其余字段遵循 [JSON Schema](https://json-schema.org/) 规范（如 `required`、`properties`、`type` 等）。

## 用途示范

### 场景一：基础 JSON API 网关

将前端 JSON 请求转发至用户微服务：

```yaml
routes:
  - path: "/api/login"
    nats_subject: "auth.login"
    methods: ["POST"]
    content_type: "application/json"
    timeout: 10
    return_fields:
      - body
      - ip
    schema_body:
      root_type: object
      strict: true
      required:
        - username
        - password
      properties:
        username:
          type: string
        password:
          type: string
```

客户端请求：

```bash
curl -X POST http://127.0.0.1:9080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"123456"}'
```

### 场景二：表单提交转 JSON

将传统 HTML 表单提交自动转为 JSON 后转发：

```yaml
routes:
  - path: "/api/feedback"
    nats_subject: "feedback.submit"
    methods: ["POST"]
    content_type: "application/x-www-form-urlencoded"
    timeout: 30
    return_fields:
      - body
      - ip
      - cookies
    schema_body:
      root_type: object
      strict: true
      required:
        - message
      properties:
        message:
          type: string
        rating:
          type: integer
```

客户端请求：

```bash
curl -X POST http://127.0.0.1:9080/api/feedback \
  -d "message=Great+service&rating=5"
```

表单中的 `rating=5` 会根据 Schema 定义的 `type: integer` 自动从字符串转换为整数 `5`。

### 场景三：仅转发特定字段

只将 HTTP 方法和路径传给微服务（适用于简单的健康检查类路由）：

```yaml
routes:
  - path: "/health"
    nats_subject: "system.health"
    methods: ["GET"]
    timeout: 5
    return_fields:
      - method
      - path
```

### 场景四：使用 Ping 端点测量延迟

```bash
# 发送带时间戳参数的 GET 请求
curl "http://127.0.0.1:9080/ping?timestamp=$(date +%s%3N)"

# 返回示例: {"pong": 3, "ip": "127.0.0.1", "servertime": 1716000000042}  (单位: 毫秒)
```

`/ping` 路由通过 NATS 转发至 `ApiNatsBridgeTemplate` 微服务，由其计算延迟并返回客户端 IP。

### 场景五：加密 NATS 通信

为敏感的支付服务使用独立加密密钥：

```yaml
nats_config:
  nats_encryption_key: "DEFAULT_GLOBAL_KEY_32_CHARS_OK!!"
  nats_theme_keys:
    "payment.process": "PAYMENT_DEDICATED_KEY_32_CHARS!!"
    "public.notify": "" # 此 Subject 明文传输

routes:
  - path: "/api/pay"
    nats_subject: "payment.process"
    methods: ["POST"]
    content_type: "application/json"
    timeout: 60
```

## 客户端 IP 解析优先级

解析客户端真实 IP 地址时按以下优先级：

1. CDN 专属标头（`cdnheader` 列表中的标头，按列表顺序依次查找）
2. `X-Real-IP` 标头
3. `X-Forwarded-For` 标头中的第一个有效 IP
4. TCP 连接的远程地址（`RemoteAddr`）

所有候选值均会验证其是否为合法 IP 地址。

## 微服务端开发指南

后端微服务只需订阅对应的 NATS Subject，接收 `BridgeRequest` JSON，处理后返回 `BridgeResponse` JSON 即可。

### Go 示例

```go
type BridgeRequest struct {
    Method     string            `json:"method"`
    Path       string            `json:"path"`
    Headers    map[string]string `json:"headers"`
    Cookies    map[string]string `json:"cookies"`
    RemoteAddr string            `json:"remote_addr"`
    IP         string            `json:"ip"`
    Params     map[string]string `json:"params"`
    Body       string            `json:"body"`
}

type BridgeResponse struct {
    StatusCode int               `json:"status_code"`
    Headers    map[string]string `json:"headers"`
    Body       string            `json:"body"`
}
```

微服务处理逻辑：

1. 订阅 NATS Subject（如 `user_service`）
2. 收到消息后解析为 `BridgeRequest`
3. 执行业务逻辑
4. 构造 `BridgeResponse` JSON 并返回

如果微服务返回的不是合法的 `BridgeResponse` JSON，ApiNatsBridge 会将原始响应字符串作为 HTTP 200 响应体直接返回给客户端。

## 本地服务环境

项目包含完整的本地测试环境（位于 `test/` 目录）及模板微服务（`ApiNatsBridgeTemplate/`）：

```bash
# Windows — 一键启动（启动 NATS Server、ApiNatsBridge、ApiNatsBridgeTemplate）
serve.bat

# Linux / macOS — 一键启动
chmod +x serve.sh
./serve.sh
```

```bash
# 一键停止所有服务
serve_stop.bat   # Windows
./serve_stop.sh  # Linux / macOS
```

> **警告：** serve 系列脚本使用示例配置文件的默认端口。与已在运行的服务会产生冲突（NATS 端口 4222、HTTP 端口 9080）。请先停止已有服务。

启动流程：

1. 启动本地 NATS 服务器（`test/nats-server/`）
2. 启动 ApiNatsBridge 主程序
3. 启动 ApiNatsBridgeTemplate 微服务（`ApiNatsBridgeTemplate/`）

启动后 `serve.bat` 会在最后自动发送测试请求，您也可以手动发送：

```bash
# 发送 ping 请求（ApiNatsBridgeTemplate 返回 {pong, ip, servertime}）
curl "http://127.0.0.1:9080/ping?timestamp=0"

# Windows PowerShell
Invoke-RestMethod -Uri ("http://127.0.0.1:9080/ping?timestamp=" + [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())
```

### ApiNatsBridgeTemplate

模板微服务订阅 `ping_req` NATS 主题，读取 `timestamp` 参数，返回 `{"pong": <延迟毫秒>, "ip": "<客户端 IP>", "servertime": <服务器时间戳毫秒>}`。

```bash
# 使用默认配置启动
go run ./ApiNatsBridgeTemplate/ -c ApiNatsBridgeTemplate/config.yaml
```

详情请参见 `ApiNatsBridgeTemplate/README.md`。

## 依赖项

| 包                                                                                                        | 用途                |
| --------------------------------------------------------------------------------------------------------- | ------------------- |
| [github.com/google/uuid](https://github.com/google/uuid)                                                  | UUID 生成           |
| [github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver](https://github.com/kagurazakayashi/libNyaruko_Go) | HTTP API 服务器框架 |
| [github.com/kagurazakayashi/libNyaruko_Go/nyanats](https://github.com/kagurazakayashi/libNyaruko_Go)      | NATS 客户端封装     |
| [gopkg.in/yaml.v3](https://github.com/go-yaml/yaml)                                                       | YAML 配置解析       |
| [github.com/santhosh-tekuri/jsonschema/v6](https://github.com/santhosh-tekuri/jsonschema)                 | JSON Schema 校验    |
| [github.com/nats-io/nats.go](https://github.com/nats-io/nats.go)                                          | NATS Go 客户端      |

## 许可证

```LICENSE
Copyright (c) 2026 KagurazakaYashi
ApiNatsBridge is licensed under Mulan PSL v2.
You can use this software according to the terms and conditions of the Mulan PSL v2.
You may obtain a copy of Mulan PSL v2 at:
         http://license.coscl.org.cn/MulanPSL2
THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY KIND,
EITHER EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO NON-INFRINGEMENT,
MERCHANTABILITY OR FIT FOR A PARTICULAR PURPOSE.
See the Mulan PSL v2 for more details.
```
