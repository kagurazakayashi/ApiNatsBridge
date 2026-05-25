[English](README.md) | [簡體中文](README.zh-Hans.md) | [繁體中文](README.zh-Hant.md) | [日本語](README.ja.md)

![ApiNatsBridge](ico/icon.png)

# ApiNatsBridge

輕量級 HTTP-to-NATS 閘道橋接器，將標準 HTTP REST 請求轉換為 NATS 訊息並轉送至後端微服務，再將微服務的回應回傳給 HTTP 用戶端。適用於微服務架構中作為 API 閘道層使用。

## 功能特性

- **HTTP 到 NATS 請求轉送** — 接收 HTTP 請求，序列化為 JSON 結構後透過 NATS Request/Reply 模式傳送到後端微服務
- **YAML 宣告式路由設定** — 透過設定檔定義 HTTP 路徑與 NATS Subject 的對應關係
- **JSON Schema 請求主體驗證** — 支援對請求主體進行 JSON Schema 驗證，拒絕不合規請求
- **表單到 JSON 自動轉換** — `application/x-www-form-urlencoded` 表單資料可依據 Schema 進行型別強制轉換後作為 JSON 轉送
- **選擇性欄位轉送** — 透過 `return_fields` 設定僅將需要的請求資訊傳遞給微服務
- **請求欄位長度限制** — 全域及路由層級的路徑、標頭、Cookie、參數、請求主體長度限制
- **AES 加密 NATS 通訊** — 支援全域金鑰和按 Subject 獨立金鑰的 AES 對稱加密
- **CDN 真實 IP 解析** — 支援 Cloudflare、Akamai、Fastly、AWS CloudFront、阿里雲 CDN 等主流 CDN 的真實用戶端 IP 擷取
- **IP 速率限制** — 內建 IP 維度的請求頻率限制與封鎖機制
- **TLS/HTTPS 支援** — 設定憑證即可啟用 HTTPS
- **`/ping` 端點** — 透過 NATS 微服務實作的延遲測量端點
- **令牌驗證** — 可選的驗證令牌校驗，透過 NATS 微服務（如 NyarukoLogin UserValidator）進行，支援路徑白名單與記憶體快取
- **IP 白名單錯誤詳細資訊** — 僅允許指定 IP 檢視詳細錯誤資訊，正式環境僅回傳通用錯誤
- **微服務錯誤資訊顯示控制** — 透過 `error_info_show` 參數設定是否記錄、輸出微服務錯誤內容，以及是否將內容回傳給白名單 IP 使用者或所有使用者
- **多國語言支援 (i18n/l10n)** — 日誌、HTTP 回應、CLI 說明文字皆支援多語言，可在設定檔中分別設定（支援 en、zh、zh_Hant、ja）
- **優雅關閉** — 攔截系統訊號後有序關閉 HTTP 伺服器、取消 NATS 訂閱並中斷連線

## 架構概覽

```
┌──────────┐         ┌───────────────────┐         ┌────────────────┐
│  HTTP    │  HTTP   │  ApiNatsBridge    │  NATS   │  Microservice  │
│  Client  │ ──────> │  (Gateway/Bridge) │ ──────> │  (Backend)     │
│          │ <────── │                   │ <────── │                │
└──────────┘         └───────────────────┘         └────────────────┘
```

1. HTTP 用戶端傳送請求到 ApiNatsBridge
2. ApiNatsBridge 進行路由比對、方法驗證、Content-Type 驗證、長度限制驗證、Schema 驗證
3. 將請求資料序列化為 `BridgeRequest` JSON，透過 NATS Request 傳送到對應 Subject
4. 後端微服務處理請求後回傳 `BridgeResponse` JSON
5. ApiNatsBridge 將回應內容作為 HTTP 回應回傳給用戶端

## 日誌模組前綴

執行時日誌輸出使用以下前綴區分來源模組：

| 前綴            | 原始檔              | 顏色   | 用途                  |
| --------------- | ------------------- | ------ | --------------------- |
| `[MAIN]`        | `src/logger.go`     | Cyan   | 主流程生命週期日誌    |
| `[NATS]`        | `src/natsLogger.go` | Green  | NATS 用戶端連線與事件 |
| `[BRIDGE]`      | `src/logger.go`     | Yellow | 橋接路由與轉送日誌    |
| `[HTTP]`        | `src/logger.go`     | Blue   | HTTP 請求日誌行       |
| `[HTTPINFO]`    | `src/logger.go`     | Blue   | HTTP 標頭、Cookie 詳情（除錯） |
| `[HTTPBODY]`    | `src/logger.go`     | Blue   | HTTP 請求/回應本文（除錯）    |
| `[NATSBODY]`    | `src/logger.go`     | Green  | NATS 請求/回覆酬載（除錯）    |
| `[STATUS]`      | `src/logger.go`     | Purple | 定期 HTTP+NATS 執行統計       |
| `[MODULE]`      | `src/logger.go`     | Cyan   | 通用模組日誌          |
| `[NATS][ERROR]` | `src/logger.go`     | Red    | NATS 連線錯誤         |
| `[HTTP][ERROR]` | `src/logger.go`     | Red    | HTTP 伺服器錯誤       |
| `[MAIN][ERROR]` | `src/logger.go`     | Red    | 主流程致命錯誤        |

所有前綴皆透過本地庫 `libNyaruko_Go/nyalog` 的 `LogCC()` 函式輸出。

## 資料結構

### BridgeRequest（傳送給微服務）

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

### BridgeResponse（微服務回傳）

```json
{
  "status_code": 200,
  "headers": { "Content-Type": "application/json; charset=utf-8" },
  "body": "{\"result\":\"success\"}"
}
```

## 安裝與編譯

### 前置條件

- Go 1.24.4 或更高版本
- 本專案使用 Git 子模組管理相依性，複製後需初始化子模組（詳見下方）

### 初始化 Git 子模組

本專案包含以下 Git 子模組：

| 子模組                                                                         | 路徑                     | 說明                                                   |
| ------------------------------------------------------------------------------ | ------------------------ | ------------------------------------------------------ |
| [libNyaruko_Go](https://github.com/kagurazakayashi/libNyaruko_Go)              | `libNyaruko_Go/`         | 相依函式庫（`nyalog`、`nyanats`、`nyaapiserver` 模組） |
| [ApiNatsBridgeTemplate](https://github.com/MasaeProject/ApiNatsBridgeTemplate) | `ApiNatsBridgeTemplate/` | 微服務範本專案                                         |

複製時一併拉取子模組：

```bash
git clone --recursive <repo_url>
```

已複製的專案初始化子模組：

```bash
git submodule init
git submodule update
```

或合併為一條命令：

```bash
git submodule update --init
```

### 強制更新子模組（忽略本機更改）

如果子模組有本機修改導致無法正常更新，可使用以下命令：

```bash
# 捨棄子模組中的所有本機更改，然後強制更新
git submodule foreach --recursive "git reset --hard && git clean -fd"
git submodule update --init --recursive --force
```

### 編譯 go-gen-l10n 工具

子模組 `libNyaruko_Go` 中包含在地化程式碼產生工具 `go-gen-l10n`。進入其目錄，產生資源並編譯：

```bash
# 進入子模組目錄
cd libNyaruko_Go/go-gen-l10n

# 產生 Windows 資源（如適用），然後編譯
go generate .
go build .

# 將執行檔複製到專案根目錄（以便 go generate ./src/l10nGlobal.go 能找到它）
# Linux / macOS
cd ../..
cp libNyaruko_Go/go-gen-l10n/go-gen-l10n .

# Windows
cd ..\..
copy libNyaruko_Go\go-gen-l10n\go-gen-l10n.exe .
```

編譯後會在專案根目錄產生 `go-gen-l10n`（或 `go-gen-l10n.exe`），修改 `l10n/app_*.arb` 語言檔案後執行以下命令重新產生程式碼：

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

### Windows 可執行檔圖示嵌入

首先安裝 `go-winres` 工具：

```bash
go install github.com/tc-hib/go-winres@latest
```

然後產生資源檔案（`.syso`），之後 `go build` 會自動連結：

```bash
# 透過 go generate 自動呼叫 go-winres（建議）
go generate ./...

# 或手動執行
go-winres make
```

> 資源設定檔案位於 `winres/winres.json`，圖示原始檔案位於 `ico/icon.png`。
> `.syso` 檔案已在 `.gitignore` 中被忽略，每次建構前需重新產生。
> 此步驟為可選——`go run .` 無需 `.syso` 檔案也可正常執行。

### 本平台編譯

```bash
# 先產生圖示資源（Windows 平台需要，其他平台可跳過）
go generate ./...

# 編譯（ Windows 的話加上 .exe ）
go build -o ApiNatsBridge .
```

### 多平台交叉編譯

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

> **提示：** 在 Windows PowerShell 下設定環境變數使用 `$env:GOOS="linux"; $env:GOARCH="amd64"` 後再執行 `go build`。

### 批次多平台編譯

使用提供的編譯腳本一次編譯所有支援的平台：

**Windows：**

```bat
build.bat
```

**Linux / macOS：**

```bash
chmod +x build.sh
./build.sh
```

> **注意：** 如果需要輸出 HTML 格式的自述檔案，請先安裝 Python `markdown` 套件：
>
> ```bash
> pip install markdown
> ```

## 使用方法

### 命令列參數

| 參數        | 說明                                                                       |
| ----------- | -------------------------------------------------------------------------- |
| `-c <路徑>` | 指定 YAML 設定檔路徑，如果不指定則預設為與執行檔同名的 `.yaml` 檔案      |
| `-o <路徑>` | 將所有日誌輸出到指定檔案（除控制台和各模組獨立日誌檔案外）               |

### 啟動範例

```bash
# 使用預設設定檔（與執行檔同名的 .yaml 檔案）
./ApiNatsBridge

# 指定設定檔
./ApiNatsBridge -c /etc/apibridge/config.yaml

# 將所有日誌輸出到統一檔案
./ApiNatsBridge -c config.yaml -o ../logs/all.log
```

## 設定檔詳解

設定檔採用 YAML 格式，包含以下四個主要部分。

### 完整設定範例

```yaml
# ===========================================================
# ApiNatsBridge 設定檔
# ===========================================================

# --- HTTP API 伺服器設定 ---
httpapiserver_config:
  # 監聽位址（設為 0.0.0.0 則監聽所有網路介面卡）
  httpapiserver_host: "127.0.0.1"
  # 監聽連接埠
  httpapiserver_port: 9080

  # TLS 憑證路徑（兩項均填寫時啟用 HTTPS，留空則使用 HTTP）
  httpapiserver_tls_cert_file: ""
  httpapiserver_tls_key_file: ""

  # 逾時設定（秒）
  httpapiserver_read_timeout: 5 # 讀取請求逾時
  httpapiserver_write_timeout: 30 # 寫入回應逾時
  httpapiserver_idle_timeout: 60 # 閒置連線逾時

  # IP 速率限制
  httpapiserver_enable_rate_limit: true # 是否啟用速率限制
  httpapiserver_limit_requests: 50 # 每個時間視窗內允許的最大請求數
  httpapiserver_limit_window: 1 # 時間視窗長度（秒）
  httpapiserver_block_duration: 600 # 超出限制後封鎖時長（秒）

# --- NATS 用戶端設定 ---
nats_config:
  # NATS 伺服器位址和連接埠
  nats_server_host: 127.0.0.1
  nats_server_port: 4222

  # 連線認證（留空則不啟用認證）
  nats_user: webapi
  nats_password: your_nats_password_here

  # 用戶端識別名稱（在 NATS 伺服器端可見）
  nats_client_name: ApiNatsBridge

  # 重連策略
  nats_max_reconnects: 5 # 最大重連次數
  nats_reconnect_wait: 2 # 重連等待間隔（秒）
  nats_connect_timeout: 10 # 初次連線逾時（秒）

  # AES 對稱加密金鑰（長度必須為 16、24 或 32 位元組）
  # 留空則明文傳輸 NATS 訊息
  nats_encryption_key: "YOUR_32_CHAR_ENCRYPTION_KEY_HERE!"

  # 按 Subject 獨立設定加密金鑰（優先於全域金鑰）
  # 設為空字串表示該 Subject 使用明文傳輸
  nats_theme_keys:
    "sensitive.subject": "PER_SUBJECT_KEY_32_CHARS_LONG!!!"
    "public.subject": ""

# --- 橋接層設定 ---
bridge:
  # 多國語言設定（支援 en、zh、zh_Hant、ja）
  language:
    log: "zh_Hant" # 日誌輸出語言
    http: "en" # HTTP 回應錯誤訊息語言
    cli: "zh_Hant" # 命令列相關訊息語言

  # 日誌輸出設定
  log:
    stdout: true # 是否同時輸出到主控台，設為 false 則僅寫入日誌檔案
    debug: ["HTTP", "NATS", "LIMIT"] # 除錯模式：空陣列 [] = 不啟用除錯；可選值：HTTP、NATS、LIMIT
    overwrite: false # 是否使用覆蓋模式，設為 true 則啟動時清空現有日誌檔案，設為 false 或不提供則僅附加
    color: true # 是否使用彩色主控台輸出，設為 true 或不提供則使用彩色，設為 false 則純文字
    files:
      # 各模組獨立日誌檔案路徑，可分別設定
      # 留空或不填則該模組不寫入檔案
      main: "logs/main.log" # 主流程日誌
      bridge: "logs/bridge.log" # 橋接路由與轉送日誌
      http: "logs/http.log" # HTTP 請求日誌
      nats: "logs/nats.log" # NATS 用戶端事件日誌
      status: "logs/status.log" # HTTP+NATS 狀態統計日誌
      module: "logs/module.log" # 通用模組日誌

  # 時區，影響所有日誌時間戳記，支援 IANA 時區名稱（如 Asia/Shanghai）或小時偏移（如 8、-5）
  timezone: "Asia/Shanghai"

  # 日誌時間日期顯示格式；Go 參照時間格式：2006-01-02 15:04:05
  #   預設值："2006-01-02 15:04:05"（YYYY-MM-DD HH:mm:ss）
  #   設為空字串 ""：控制台不顯示時間日期，但日誌檔案仍依照預設格式記錄
  #   自訂格式如："15:04:05"（僅時間）、"01/02 15:04:05"（月/日 時:分:秒）
  #   可被路由層級 time_format 覆蓋
  # time_format: ""                    # 空字串：控制台不顯示時間
  time_format: "2006-01-02 15:04:05" # 預設值：YYYY-MM-DD HH:mm:ss

  # CDN 真實 IP 標頭清單（按優先級排列）
  # 用於從 CDN 代理請求中擷取用戶端真實 IP 位址
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
    - "Ali-Cdn-Real-Ip" # 阿里雲 CDN
    - "Ar-Real-IP" # ArvanCloud

  # 允許檢視詳細錯誤資訊的 IP 白名單（用於開發偵錯）
  # 不在此清單中的 IP 僅收到通用錯誤提示
  error_detail_ips:
    - "127.0.0.1"
    - "::1"

  # 微服務回傳 JSON 中 HTTP 狀態碼的全域預設鍵名（可按路由覆蓋）
  http_code_key: "status_code"
  # 微服務回傳 JSON 中錯誤碼的全域預設鍵名（可按路由覆蓋）
  error_code_key: "error_code"
  # 回應本文 JSON Schema 驗證的全域預設規則（可按路由覆蓋）
  # response_schema_body: {}
  # 錯誤回應本文 JSON Schema 驗證的全域預設規則（可按路由覆蓋）
  # response_error_schema_body: {}
  # 微服務錯誤資訊顯示模式的全域預設值（可按路由覆蓋）
  #   0=不記錄、1=記錄+輸出、2=記錄+輸出+白名單可見、3=記錄+輸出+全員可見、4=不記錄+白名單可見、5=不記錄+全員可見
  error_info_show: 1

  # 全域請求欄位長度限制（0 或省略表示不限制）
  limits:
    path:
      max_length: 2048 # 請求路徑最大位元組長度
    headers:
      max_count: 64 # 請求標頭最大數量
      max_key_length: 256 # 標頭名稱最大位元組長度
      max_value_length: 4096 # 標頭值最大位元組長度
    cookies:
      max_count: 32 # Cookie 最大數量
      max_key_length: 256 # Cookie 名稱最大位元組長度
      max_value_length: 4096 # Cookie 值最大位元組長度
    params:
      max_count: 64 # 參數最大數量
      max_key_length: 256 # 參數名稱最大位元組長度
      max_value_length: 4096 # 參數值最大位元組長度
    body:
      max_length: 1048576 # 請求主體最大位元組長度（1MB）

  # 全域回應欄位長度限制（結構同 limits，可被路由層級覆蓋）
  response_limits:
    body:
      max_length: 1048576 # 回應主體最大位元組長度（1MB）
    headers:
      max_count: 64 # 回應標頭最大數量
      max_key_length: 256 # 回應標頭名稱最大位元組長度
      max_value_length: 4096 # 回應標頭值最大位元組長度

# --- 路由轉送規則 ---
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
    # http_code_key: "status_code"  # 回應 JSON 中 HTTP 狀態碼的鍵名，不指定時預設 200（可選）
    # error_code_key: "error_code"  # 回應 JSON 中錯誤碼的鍵名，檢測到時觸發 response_error_schema_body 驗證（可選）
    # error_info_show: 2  # 錯誤資訊顯示模式（覆蓋 bridge 層級）；0=不記錄、1=記錄、2=記錄+白名單可見、3=記錄+全員可見、4=不記錄+白名單可見、5=不記錄+全員可見（可選）
```

### 設定項詳解

#### `httpapiserver_config` — HTTP 伺服器設定

| 設定項                            | 型別   | 說明                                         |
| --------------------------------- | ------ | -------------------------------------------- |
| `httpapiserver_host`              | string | 伺服器監聽位址，`0.0.0.0` 監聽所有網路介面卡 |
| `httpapiserver_port`              | int    | 監聽連接埠                                   |
| `httpapiserver_tls_cert_file`     | string | TLS 憑證檔案路徑，留空使用 HTTP              |
| `httpapiserver_tls_key_file`      | string | TLS 私密金鑰檔案路徑，留空使用 HTTP          |
| `httpapiserver_read_timeout`      | int    | 讀取請求逾時（秒）                           |
| `httpapiserver_write_timeout`     | int    | 寫入回應逾時（秒）                           |
| `httpapiserver_idle_timeout`      | int    | 閒置連線逾時（秒）                           |
| `httpapiserver_enable_rate_limit` | bool   | 是否啟用 IP 速率限制                         |
| `httpapiserver_limit_requests`    | int    | 時間視窗內最大請求數                         |
| `httpapiserver_limit_window`      | int    | 速率限制時間視窗（秒）                       |
| `httpapiserver_block_duration`    | int    | 超限後封鎖時長（秒）                         |

#### `nats_config` — NATS 用戶端設定

| 設定項                 | 型別   | 說明                                          |
| ---------------------- | ------ | --------------------------------------------- |
| `nats_server_host`     | string | NATS 伺服器位址                               |
| `nats_server_port`     | int    | NATS 伺服器連接埠                             |
| `nats_user`            | string | NATS 使用者名稱，留空不認證                   |
| `nats_password`        | string | NATS 密碼                                     |
| `nats_client_name`     | string | 連線識別名稱                                  |
| `nats_max_reconnects`  | int    | 最大重連次數                                  |
| `nats_reconnect_wait`  | int    | 重連間隔（秒）                                |
| `nats_connect_timeout` | int    | 連線逾時（秒）                                |
| `nats_encryption_key`  | string | AES 全域加密金鑰（16/24/32 位元組），留空明文 |
| `nats_theme_keys`      | map    | 按 Subject 獨立設定的加密金鑰                 |

#### `bridge` — 橋接層設定

| 設定項             | 型別     | 說明                                                      |
| ------------------ | -------- | --------------------------------------------------------- |
| `language`         | object   | 多國語言設定（詳見下方）                                  |
| `log`              | object   | 日誌輸出設定（詳見下方）                                  |
| `timezone`         | string   | 時區，影響所有日誌時間戳記，如 `"Asia/Shanghai"` 或 `"8"` |
| `time_format`      | string   | 日誌時間日期顯示格式；預設 `"2006-01-02 15:04:05"`（YYYY-MM-DD HH:mm:ss）；設為空字串 `""` 時控制台不顯示時間但日誌檔案仍使用預設格式；可按路由覆蓋 |
| `cdnheader`        | []string | CDN 真實 IP 標頭優先級清單                                |
| `error_detail_ips` | []string | 允許檢視詳細錯誤的 IP 白名單                              |
| `http_code_key`    | string   | 微服務回傳 JSON 中 HTTP 狀態碼的全域預設鍵名；可按路由覆蓋 |
| `error_code_key`   | string   | 微服務回傳 JSON 中錯誤碼的全域預設鍵名；可按路由覆蓋       |
| `response_schema_body`      | object | 回應本文 JSON Schema 驗證的全域預設規則；可按路由覆蓋   |
| `response_error_schema_body`| object | 錯誤回應本文 JSON Schema 驗證的全域預設規則；可按路由覆蓋 |
| `error_info_show`  | int      | 微服務錯誤資訊顯示模式的全域預設值；可按路由覆蓋（0=不記錄、1=記錄+輸出、2=記錄+輸出+白名單可見、3=記錄+輸出+全員可見、4=不記錄+白名單可見、5=不記錄+全員可見） |
| `limits`           | object   | 全域請求欄位長度限制                                       |
| `response_limits`  | object   | 全域回應欄位長度限制（結構同 limits）                     |
| `token`            | object   | 令牌驗證設定（詳見[令牌驗證](#令牌驗證)）                   |
| `max_concurrent` | int | `0`（不限制） | 全域最大並行 NATS 轉發請求數，超限時回傳 503 |
| `http_max_concurrent` | int | `0`（不限制） | HTTP 層最大並行請求處理數，超限時回傳 503 |

##### `bridge.language` — 多國語言設定

| 設定項 | 型別   | 預設值      | 說明                  |
| ------ | ------ | ----------- | --------------------- |
| `log`  | string | `"zh_Hant"` | 日誌輸出語言          |
| `http` | string | `"en"`      | HTTP 回應錯誤訊息語言 |
| `cli`  | string | `"zh_Hant"` | 命令列相關訊息語言    |

> 支援的語言代碼：`en`（英語）、`zh`（簡體中文）、`zh_Hant`（繁體中文）、`ja`（日語）。
>
> **重要：** 語言文字的修改應在翻譯原始檔 `l10n/app_*.arb` 中進行，**不要**直接編輯 `l10n/app_localizations_*.go` 產生檔案，它們會在重新產生時被覆蓋。
>
> 修改 `.arb` 檔案後需執行以下命令重新產生 Go 程式碼：
>
> ```bash
> # Windows
> .\go-gen-l10n.exe -dir .\l10n -pkg l10n -lang zh_Hant
>
> # 或使用 go generate
> go generate .\src\l10nGlobal.go
> ```
>
> #### 語言風格約定
>
> | 語言代碼  | 語言     | 風格                 |
> | --------- | -------- | -------------------- |
> | `zh`      | 簡體中文 | 大陸風格（大陸簡體） |
> | `zh_Hant` | 繁體中文 | 台灣風格（臺灣繁體） |
> | `en`      | 英語     | 標準                 |
> | `ja`      | 日語     | 標準                 |
>
> #### ARB 檔案
>
> | 檔案                   | 語言             |
> | ---------------------- | ---------------- |
> | `l10n/app_zh.arb`      | 簡體中文（大陸） |
> | `l10n/app_zh_Hant.arb` | 繁體中文（台灣） |
> | `l10n/app_en.arb`      | 英語             |
> | `l10n/app_ja.arb`      | 日語             |

##### `bridge.log` — 日誌輸出設定

| 設定項      | 型別   | 說明                                                                  |
| ----------- | ------ | --------------------------------------------------------------------- |
| `stdout`    | bool   | 是否同時輸出到主控台，`false` 則僅寫檔案                              |
| `debug`     | []string | 除錯模式旗標陣列；空 `[]` = 僅 Info+。可選：`"HTTP"`（完整HTTP流量）、`"NATS"`（完整NATS流量）、`"LIMIT"`（被拒絕請求的違規詳情） |
| `overwrite` | bool   | 是否覆蓋模式，`true` 則啟動時清空現有日誌檔案，`false` 或不提供僅附加 |
| `color`     | bool   | 是否彩色主控台輸出，`true` 或不提供則彩色，`false` 則純文字           |
| `status_interval_seconds` | int | STATUS 日誌輸出間隔秒數；預設 60 秒                                      |
| `files`     | object | 各模組獨立日誌檔案路徑（詳見下方）                                    |

##### `bridge.log.files` — 模組日誌檔案路徑

| 設定項     | 型別   | 說明                            |
| ---------- | ------ | ------------------------------- |
| `main`     | string | 主流程日誌檔案路徑              |
| `bridge`   | string | 橋接路由與轉送日誌檔案路徑      |
| `http`     | string | HTTP 請求日誌檔案路徑           |
| `nats`     | string | NATS 用戶端事件日誌檔案路徑     |
| `status`   | string | HTTP+NATS 狀態統計日誌檔案路徑  |
| `module`   | string | 通用模組日誌檔案路徑            |

> 日誌檔案路徑為相對或絕對路徑均可。目錄不存在時會自動建立。
> 路徑留空或不填則該模組不寫入檔案。若 `stdout: false` 且所有檔案路徑均為空，則該模組無日誌輸出。

#### `routes` — 路由規則

| 設定項                 | 型別     | 預設值         | 說明                                             |
| ---------------------- | -------- | -------------- | ------------------------------------------------ |
| `path`                 | string   | （必填）       | HTTP 請求路徑                                    |
| `nats_subject`         | string   | （必填）       | 轉送的 NATS Subject                              |
| `methods`              | []string | []（允許全部） | 允許的 HTTP 方法清單                             |
| `content_type`         | string   | ""（不驗證）   | 要求的 Content-Type 前綴                         |
| `timeout`              | int      | 30             | NATS 回應逾時（秒）                              |
| `return_fields`        | []string | []（回傳全部） | 轉送給微服務的欄位選擇                           |
| `limits`               | object   | -              | 路由層級長度限制（覆蓋全域）                     |
| `schema_body`          | object   | -              | 請求主體 JSON Schema 驗證                        |
| `response_limits`      | object   | -              | 路由層級回應長度限制（覆蓋全域 response_limits） |
| `response_schema_body` | object   | -              | 回應主體 JSON Schema 驗證（結構同 schema_body）  |
| `response_error_schema_body` | object   | -              | 錯誤回應主體 JSON Schema 驗證（檢測到 error_code_key 時使用，未設定則沿用 response_schema_body） |
| `http_code_key`    | string   | ""（預設 200）   | 微服務回傳 JSON 中表示 HTTP 狀態碼的鍵名（100-599）；指定後回傳用戶端時會從回應中移除此鍵 |
| `error_code_key`   | string   | ""（不啟用）    | 微服務回傳 JSON 中表示錯誤碼的鍵名（int32）；檢測到此鍵時觸發 response_error_schema_body 驗證 |
| `error_info_show`  | int      | 0               | 微服務錯誤資訊顯示模式（覆蓋 bridge 層級）；0=不記錄、1=記錄、2=記錄+白名單可見、3=記錄+全員可見、4=不記錄+白名單可見、5=不記錄+全員可見 |
| `time_format`      | string   | -               | 路由層級日誌時間日期顯示格式（覆蓋 bridge 層級）；語義同 bridge 層的 `time_format` |
| `max_concurrent` | int | `0`（沿用全域） | 路由層級最大並行 NATS 轉發請求數；0 表示沿用全域限制；超限時回傳 503 |
| `token_fields` | []string | - | **（向後相容，推薦改用 `return_fields`）** 舊版令牌 claims 注入設定。列出的欄位從驗證回覆中提取並以頂層欄位注入。新部署應將令牌 claim 名稱直接加入 `return_fields` |

#### `return_fields` 可選值

| 欄位名        | 來源   | 說明                             |
| ------------- | ------ | -------------------------------- |
| `method`      | HTTP   | HTTP 請求方法                    |
| `path`        | HTTP   | 請求路徑                         |
| `headers`     | HTTP   | 請求標頭（鍵值對）               |
| `cookies`     | HTTP   | Cookie（鍵值對）                 |
| `remote_addr` | HTTP   | 直連 TCP 位址（含連接埠）        |
| `ip`          | HTTP   | 解析後的真實用戶端 IP            |
| `params`      | HTTP   | URL 查詢參數和表單參數（鍵值對） |
| `body`        | HTTP   | 請求主體原始內容                 |
| *其他名稱*    | **令牌** | **令牌 claim 欄位** — 不屬於以上 8 個已知欄位的名稱，會從令牌驗證 claims 中解析並以頂層欄位注入。可用 claims：`username`、`app`、`sub`、`iss`、`iat`、`nbf`、`exp`、`jti` 及透過 UserValidator `custom_claims` 寫入的自訂 claims（如 `uuid`）。僅對不在 `token.path_whitelist` 中的路由生效 |

#### `return_fields` 中的令牌 Claims

啟用令牌驗證後，`return_fields` 中**不屬於** 8 個已知 BridgeRequest 欄位（`method`、`path`、`headers`、`cookies`、`remote_addr`、`ip`、`params`、`body`）的名稱，會自動從令牌驗證回覆的 claims 中解析並以**頂層欄位**注入轉發資料。

```yaml
routes:
  - path: "/api/user"
    nats_subject: "user.get"
    return_fields: ["method", "path", "body", "uuid", "username"]
    #                                         ^^^^   ^^^^^^^^
    #                              這些來自令牌 claims
```

轉發給下游的資料將包含：
```json
{
  "method": "GET",
  "path": "/api/user",
  "body": "...",
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "username": "admin"
}
```

下游微服務可直接使用 `uuid` 來識別已認證使用者，無需自行解密令牌。

令牌 claim 欄位僅在以下條件滿足時注入：
- 該路由不在 `token.path_whitelist` 中（令牌實際被驗證）
- 令牌驗證成功
- 該欄位在驗證回覆中存在

> **向後相容：** 舊版 `token_fields` 設定仍可使用。其中列出的欄位現在以頂層欄位注入（不再包裹在 `_token` 物件中）。新部署建議直接使用 `return_fields`。

#### `schema_body` JSON Schema 驗證

除標準 JSON Schema 欄位外，支援兩個控制鍵：

| 控制鍵      | 型別   | 說明                                             |
| ----------- | ------ | ------------------------------------------------ |
| `root_type` | string | 根節點預期型別（如 `object`、`array`）           |
| `strict`    | bool   | 嚴格模式，為 `true` 時拒絕 Schema 中未定義的欄位 |

其餘欄位遵循 [JSON Schema](https://json-schema.org/) 規範（如 `required`、`properties`、`type` 等）。

## 用途示範

### 情境一：基礎 JSON API 閘道

將前端 JSON 請求轉送至使用者微服務：

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

用戶端請求：

```bash
curl -X POST http://127.0.0.1:9080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"123456"}'
```

### 情境二：表單提交轉 JSON

將傳統 HTML 表單提交自動轉為 JSON 後轉送：

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

用戶端請求：

```bash
curl -X POST http://127.0.0.1:9080/api/feedback \
  -d "message=Great+service&rating=5"
```

表單中的 `rating=5` 會根據 Schema 定義的 `type: integer` 自動從字串轉換為整數 `5`。

### 情境三：僅轉送特定欄位

只將 HTTP 方法和路徑傳給微服務（適用於簡單的健康檢查類路由）：

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

### 情境四：使用 Ping 端點測量延遲

```bash
# 傳送帶時間戳記參數的 GET 請求
curl "http://127.0.0.1:9080/ping?timestamp=$(date +%s%3N)"

# 回傳範例: {"pong": 3, "ip": "127.0.0.1", "servertime": 1716000000042}  (單位: 毫秒)
```

`/ping` 路由透過 NATS 轉送至 `ApiNatsBridgeTemplate` 微服務，由其計算延遲並回傳客戶端 IP。

### 情境五：加密 NATS 通訊

為敏感的支付服務使用獨立加密金鑰：

```yaml
nats_config:
  nats_encryption_key: "DEFAULT_GLOBAL_KEY_32_CHARS_OK!!"
  nats_theme_keys:
    "payment.process": "PAYMENT_DEDICATED_KEY_32_CHARS!!"
    "public.notify": "" # 此 Subject 明文傳輸

routes:
  - path: "/api/pay"
    nats_subject: "payment.process"
    methods: ["POST"]
    content_type: "application/json"
    timeout: 60
```

## 令牌驗證

ApiNatsBridge 可選擇性地對傳入的 HTTP 請求執行驗證令牌校驗。啟用後，它會從可設定的 HTTP 標頭中提取令牌，發送至基於 NATS 的令牌驗證服務（如 [NyarukoLogin UserValidator](https://github.com/kagurazakayashi/NyarukoLogin)），僅允許持有有效令牌的請求繼續處理。

### 架構流程

```
┌──────────┐      HTTP        ┌───────────────────┐      NATS        ┌────────────────┐
│  HTTP    │  Authorization   │  ApiNatsBridge    │  UUID|2|token    │  UserValidator │
│  用戶端  │ ────────────────>│  (Gateway/Bridge) │ ────────────────>│  (NATS 微服務) │
│          │ <────────────────│                   │ <────────────────│                │
└──────────┘  HTTP 200/401    └───────────────────┘  UUID|{JSON}     └────────────────┘
```

**步驟說明：**

1. **用戶端 → ApiNatsBridge（HTTP）**：用戶端發送 HTTP 請求，並在指定的標頭（預設 `Authorization`）中攜帶令牌，如 `Authorization: v2.local.FcG...`

2. **ApiNatsBridge → UserValidator（NATS Request）**：橋接器產生一個 UUID tag，以層級 2 格式發送 `UUID|2|令牌`（層級 2：系統＋token claims）到設定的 NATS 主題（預設 `auth.token.verify`）

3. **UserValidator → ApiNatsBridge（NATS Reply）**：驗證器回傳 `UUID|{"success":bool,...}` — 其中 `success:true` 表示有效（含全部標準 PASETO claims：`username`/`app`/`sub`/`iss`/`iat`/`nbf`/`exp`/`jti`），`success:false` 表示無效（含 `message` 錯誤訊息）

4. **ApiNatsBridge → 用戶端（HTTP Response）**：
   - 令牌有效：請求繼續轉送至後端微服務
   - 令牌無效：回傳 HTTP 401，`{"error":"Unauthorized: invalid token"}`
   - NATS 錯誤：回傳 HTTP 502，`{"error":"Bad Gateway: token verification request failed"}`
   - 缺少令牌：回傳 HTTP 401，`{"error":"Unauthorized: missing authentication token"}`

### NATS 訊息格式

| 方向 | 格式 | 範例 |
|------|------|------|
| ApiNatsBridge → UserValidator | `UUID\|2\|令牌` | `550e8400-e29b-41d4-a716-446655440000\|2\|v2.local.FcG...` |
| UserValidator → ApiNatsBridge（有效） | `UUID\|{"success":true,...}` | `550e8400-...\|{"success":true,"username":"admin","app":"myapp","sub":"admin","iss":"/auth/login","iat":"...","nbf":"...","exp":"...","jti":"abc123..."}` |
| UserValidator → ApiNatsBridge（無效） | `UUID\|{"success":false,...}` | `550e8400-...\|{"success":false,"message":"token verification failed: ..."}` |

- `|` 為欄位分隔符
- 層級 `2` 可取得全部標準 PASETO claims（`username`/`app`/`sub`/`iss`/`iat`/`nbf`/`exp`/`jti`），無需額外 DB 查詢
- 完整的 NATS 層級通訊協定詳情，請參見 [NyarukoLogin UserValidator README](https://github.com/kagurazakayashi/NyarukoLogin/blob/master/UserValidator/README.md#authtokenverify--直接-nats-介面令牌核實)

### 設定

```yaml
bridge:
  token:
    nats_subject: "auth.token.verify"   # 驗證令牌用的 NATS 主題
    path_whitelist:                      # 不需要檢查令牌的路徑白名單
      - "/ping"
      - "/auth/login"
    header_name: "Authorization"         # 令牌所在的 HTTP 標頭名稱
    min_length: 10                       # 令牌最小位元組長度
    max_length: 4096                     # 令牌最大位元組長度
    tag_separator: "|"                   # tag 與結果之間的分隔符
    success_value: "0"                   # 令牌有效時的預期回傳值
    timeout: 5                           # 等待 NATS 回覆的超時秒數
    cache_max_entries: 1000              # 快取的驗證結果最大筆數
    max_concurrent: 256                  # 最大並行驗證數
```

| 欄位 | 型別 | 預設值 | 說明 |
|------|------|--------|------|
| `nats_subject` | string | `auth.token.verify` | 令牌驗證的 NATS 主題 |
| `path_whitelist` | []string | `[]` | 略過令牌檢查的 HTTP 路徑 |
| `header_name` | string | `Authorization` | 包含令牌的 HTTP 標頭名稱 |
| `min_length` | int | `10` | 令牌允許的最小位元組長度 |
| `max_length` | int | `4096` | 令牌允許的最大位元組長度 |
| `tag_separator` | string | `\|` | 回覆中分隔 tag 與結果的字元 |
| `success_value` | string | `0` | 表示令牌有效的回傳值 |
| `timeout` | int | `5` | NATS 請求超時秒數 |
| `cache_max_entries` | int | `1000` | 快取的驗證結果最大筆數，達上限時清空快取 |
| `max_concurrent` | int | `256` | 最大並行令牌驗證 NATS 請求數，超限時回傳 HTTP 503 |
| `paseto_secret_key` | any | - | 選填 PASETO 本地解密金鑰。支援十六進位字串或 `{時間戳: 金鑰}` 金鑰輪替字典（格式與 NyarukoLogin UserValidator 相同） |

> **注意：** 令牌驗證**預設為停用**。若要啟用，必須在設定檔中明確加入 `token` 區塊。
>
> tag 使用 **UUID v4**（如 `550e8400-e29b-41d4-a716-446655440000`），僅包含十六進位字元與連字號，保證不含 `?` 和 `!` 字元。
>
> 透過 UserValidator 的 `token_claims_mapping.custom_claims` 寫入令牌的自訂 claims（如 `uuid`）會自動包含在層級 2 驗證回覆中，可在路由的 `return_fields` 中加入對應名稱來提取（或透過舊版 `token_fields` 設定）。

### 令牌快取

為減少 NATS 流量，已驗證的令牌和最近失敗的令牌結果會快取在記憶體中：

- **快取鍵**：完整的令牌字串
- **快取命中（有效）**：請求直接放行，無需 NATS 呼叫
- **快取命中（無效）**：請求直接拒絕（HTTP 401）
- **快取未命中**：執行 NATS 驗證並將結果寫入快取
- **快取淘汰**：當達到 `cache_max_entries` 上限時，清空整個快取（冷重啟策略）

### 錯誤代碼

| HTTP 狀態碼 | 訊息 | 原因 |
|-------------|------|------|
| 400 | `invalid token length` | 令牌長度小於 `min_length` 或大於 `max_length` |
| 401 | `missing authentication token` | 令牌標頭不存在或為空 |
| 401 | `invalid token` | NATS 回傳結果不是 `0`（令牌過期、格式錯誤或已撤銷） |
| 502 | `token verification request failed` | NATS 請求逾時或連線錯誤 |
| 503 | `too many token verification requests` | 並行驗證數達到上限（max_concurrent） |

## 用戶端 IP 解析優先級

解析用戶端真實 IP 位址時按以下優先級：

1. CDN 專屬標頭（`cdnheader` 清單中的標頭，按清單順序依次尋找）
2. `X-Real-IP` 標頭
3. `X-Forwarded-For` 標頭中的第一個有效 IP
4. TCP 連線的遠端位址（`RemoteAddr`）

所有候選值均會驗證其是否為合法 IP 位址。

## 微服務端開發指南

後端微服務只需訂閱對應的 NATS Subject，接收 `BridgeRequest` JSON，處理後回傳 `BridgeResponse` JSON 即可。

### Go 範例

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

微服務處理邏輯：

1. 訂閱 NATS Subject（如 `user_service`）
2. 收到訊息後解析為 `BridgeRequest`
3. 執行業務邏輯
4. 建構 `BridgeResponse` JSON 並回傳

如果微服務回傳的不是合法的 `BridgeResponse` JSON，ApiNatsBridge 會將原始回應字串作為 HTTP 200 回應主體直接回傳給用戶端。

## 本地服務環境

專案包含完整的本地測試環境（位於 `test/` 目錄）及範本微服務（`ApiNatsBridgeTemplate/`）：

```bash
# Windows — 一鍵啟動（啟動 NATS Server、ApiNatsBridge、ApiNatsBridgeTemplate）
serve.bat

# Linux / macOS — 一鍵啟動
chmod +x serve.sh
./serve.sh
```

```bash
# 一鍵停止所有服務
serve_stop.bat   # Windows
./serve_stop.sh  # Linux / macOS
```

> **警告：** serve 系列指令碼使用範例設定檔的預設連接埠。與已執行的服務會產生衝突（NATS 連接埠 4222、HTTP 連接埠 9080）。請先停止既有服務。

啟動流程：

1. 啟動本地 NATS 伺服器（`test/nats-server/`）
2. 啟動 ApiNatsBridge 主程式
3. 啟動 ApiNatsBridgeTemplate 微服務（`ApiNatsBridgeTemplate/`）

啟動後 `serve.bat` 會在最後自動送出測試請求，您也可以手動送出：

```bash
# 傳送 ping 請求（ApiNatsBridgeTemplate 回傳 {pong, ip, servertime}）
curl "http://127.0.0.1:9080/ping?timestamp=0"

# Windows PowerShell
Invoke-RestMethod -Uri ("http://127.0.0.1:9080/ping?timestamp=" + [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())
```

### ApiNatsBridgeTemplate

範本微服務訂閱 `ping_req` NATS 主題，讀取 `timestamp` 參數，回傳 `{"pong": <延遲毫秒>, "ip": "<客戶端 IP>", "servertime": <伺服器時間戳毫秒>}`。

```bash
# 使用預設設定啟動
go run ./ApiNatsBridgeTemplate/ -c ApiNatsBridgeTemplate/config.yaml
```

詳情請參見 `ApiNatsBridgeTemplate/README.md`。

## 相依性項目

| 套件                                                                                                      | 用途                |
| --------------------------------------------------------------------------------------------------------- | ------------------- |
| [github.com/google/uuid](https://github.com/google/uuid)                                                  | UUID 產生           |
| [github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver](https://github.com/kagurazakayashi/libNyaruko_Go) | HTTP API 伺服器框架 |
| [github.com/kagurazakayashi/libNyaruko_Go/nyanats](https://github.com/kagurazakayashi/libNyaruko_Go)      | NATS 用戶端封裝     |
| [gopkg.in/yaml.v3](https://github.com/go-yaml/yaml)                                                       | YAML 設定解析       |
| [github.com/santhosh-tekuri/jsonschema/v6](https://github.com/santhosh-tekuri/jsonschema)                 | JSON Schema 驗證    |
| [github.com/nats-io/nats.go](https://github.com/nats-io/nats.go)                                          | NATS Go 用戶端      |

## 授權條款

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
