[English](README.md) | [简体中文](README.zh-Hans.md) | [繁體中文](README.zh-Hant.md) | [日本語](README.ja.md)

![ApiNatsBridge](ico/icon.png)

# ApiNatsBridge

A lightweight HTTP-to-NATS gateway bridge that converts standard HTTP REST requests into NATS messages, forwards them to backend microservices, and returns microservice responses to HTTP clients. Suitable for use as an API gateway layer in microservice architectures.

## Features

- **HTTP to NATS Request Forwarding** — Receives HTTP requests, serializes them into JSON structures, and sends them to backend microservices via the NATS Request/Reply pattern
- **YAML Declarative Route Configuration** — Defines mappings between HTTP paths and NATS subjects through a configuration file
- **JSON Schema Request Body Validation** — Supports validating request bodies against JSON Schema, rejecting non-compliant requests
- **Form to JSON Auto-Conversion** — `application/x-www-form-urlencoded` form data can be type-coerced and forwarded as JSON based on the Schema
- **Selective Field Forwarding** — Use the `return_fields` option to pass only the required request information to microservices
- **Request Field Length Limits** — Global and per-route length limits for paths, headers, cookies, parameters, and request bodies
- **AES Encrypted NATS Communication** — Supports AES symmetric encryption with a global key or per-subject keys
- **CDN Real IP Resolution** — Supports extracting the real client IP from major CDN providers such as Cloudflare, Akamai, Fastly, AWS CloudFront, and Alibaba Cloud CDN
- **IP Rate Limiting** — Built-in per-IP request rate limiting and banning mechanism
- **TLS/HTTPS Support** — Enable HTTPS by configuring a certificate
- **`/ping` Endpoint** — Latency measurement endpoint via NATS microservice
- **IP Whitelist Error Details** — Only allows specified IPs to view detailed error information; production environments return generic errors only
- **Microservice Error Info Display Control** — Configurable `error_info_show` parameter controls whether microservice error content is logged and returned to whitelist IP users or all users
- **Multi-Language Support (i18n/l10n)** — Logs, HTTP responses, and CLI help text all support multiple languages, configurable independently in the config file (supports en, zh, zh_Hant, ja)
- **Graceful Shutdown** — Gracefully shuts down the HTTP server, cancels NATS subscriptions, and disconnects upon receiving system signals

## Architecture Overview

```
┌──────────┐         ┌───────────────────┐         ┌────────────────┐
│  HTTP    │  HTTP   │  ApiNatsBridge    │  NATS   │  Microservice  │
│  Client  │ ──────> │  (Gateway/Bridge) │ ──────> │  (Backend)     │
│          │ <────── │                   │ <────── │                │
└──────────┘         └───────────────────┘         └────────────────┘
```

1. HTTP client sends a request to ApiNatsBridge
2. ApiNatsBridge performs route matching, method validation, Content-Type validation, length limit checks, and Schema validation
3. The request data is serialized into `BridgeRequest` JSON and sent to the corresponding subject via NATS Request
4. The backend microservice processes the request and returns `BridgeResponse` JSON
5. ApiNatsBridge returns the response content to the client as an HTTP response

## Log Module Prefixes

Runtime log output uses the following prefixes to distinguish source modules:

| Prefix          | Source File         | Color  | Purpose                            |
| --------------- | ------------------- | ------ | ---------------------------------- |
| `[MAIN]`        | `src/logger.go`     | Cyan   | Main process lifecycle logs        |
| `[NATS]`        | `src/natsLogger.go` | Green  | NATS client connection and events  |
| `[BRIDGE]`      | `src/logger.go`     | Yellow | Bridge routing and forwarding logs |
| `[HTTP]`        | `src/logger.go`     | Blue   | HTTP request log lines             |
| `[HTTPSTAT]`    | `src/logger.go`     | Purple | HTTP server runtime statistics     |
| `[MODULE]`      | `src/logger.go`     | Cyan   | Generic module logs                |
| `[NATS][ERROR]` | `src/logger.go`     | Red    | NATS connection errors             |
| `[HTTP][ERROR]` | `src/logger.go`     | Red    | HTTP server errors                 |
| `[MAIN][ERROR]` | `src/logger.go`     | Red    | Fatal errors in the main process   |

All prefixes are output via the `LogCC()` function from the local library `libNyaruko_Go/nyalog`.

## Data Structures

### BridgeRequest (sent to microservice)

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

### BridgeResponse (returned by microservice)

```json
{
  "status_code": 200,
  "headers": { "Content-Type": "application/json; charset=utf-8" },
  "body": "{\"result\":\"success\"}"
}
```

## Installation and Compilation

### Prerequisites

- Go 1.24.4 or higher
- This project uses Git submodules for dependency management; after cloning, submodules must be initialized (see below)

### Initializing Git Submodules

This project includes the following Git submodules:

| Submodule                                                                      | Path                     | Description                                                        |
| ------------------------------------------------------------------------------ | ------------------------ | ------------------------------------------------------------------ |
| [libNyaruko_Go](https://github.com/kagurazakayashi/libNyaruko_Go)              | `libNyaruko_Go/`         | Dependency libraries (`nyalog`, `nyanats`, `nyaapiserver` modules) |
| [ApiNatsBridgeTemplate](https://github.com/MasaeProject/ApiNatsBridgeTemplate) | `ApiNatsBridgeTemplate/` | Microservice template project                                      |

Clone with submodules:

```bash
git clone --recursive <repo_url>
```

Initialize submodules in an already-cloned project:

```bash
git submodule init
git submodule update
```

Or combine into a single command:

```bash
git submodule update --init
```

### Building the go-gen-l10n Tool

The submodule `libNyaruko_Go` includes the localization code generation tool `go-gen-l10n`. Enter its directory, generate resources, and build:

```bash
# Enter the submodule directory
cd libNyaruko_Go/go-gen-l10n

# Generate Windows resource (if applicable), then build
go generate .
go build .

# Copy the binary to the project root (so go generate ./src/l10nGlobal.go can find it)
# Linux / macOS
cd ../..
cp libNyaruko_Go/go-gen-l10n/go-gen-l10n .  # (so go generate ./src/l10nGlobal.go can find it)

# Windows
cd ..\..
copy libNyaruko_Go\go-gen-l10n\go-gen-l10n.exe .  &rem ; (so go generate .\src\l10nGlobal.go can find it)
```

After building, `go-gen-l10n` (or `go-gen-l10n.exe`) will be available in the project root. After modifying `l10n/app_*.arb` language files, run the following command to regenerate code:

```bash
# Linux / macOS
./go-gen-l10n -dir ./l10n -pkg l10n -lang zh_Hant

# Windows
.\go-gen-l10n.exe -dir .\l10n -pkg l10n -lang zh_Hant
```

Or use `go generate`:

```bash
go generate ./src/l10nGlobal.go
```

### Embedding an Icon in the Windows Executable

First, install the `go-winres` tool:

```bash
go install github.com/tc-hib/go-winres@latest
```

Then generate the resource file (`.syso`), which `go build` will automatically link:

```bash
# Using go generate to invoke go-winres automatically (recommended)
go generate ./...

# Or manually run
go-winres make
```

> The resource configuration file is located at `winres/winres.json`, and the icon source file is at `ico/icon.png`.
> The `.syso` file is ignored in `.gitignore` and must be regenerated before each build.
> This step is optional — `go run .` works without the `.syso` file.

### Building for the Current Platform

```bash
# Generate the icon resource first (required on Windows, can be skipped on other platforms)
go generate ./...

# Build (add .exe on Windows)
go build -o ApiNatsBridge .
```

### Cross-Platform Compilation

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

> **Tip:** In Windows PowerShell, set environment variables using `$env:GOOS="linux"; $env:GOARCH="amd64"` before running `go build`.

### Batch Multi-Platform Build

Use the provided build scripts to compile for all supported platforms at once:

**Windows:**

```bat
build.bat
```

**Linux / macOS:**

```bash
chmod +x build.sh
./build.sh
```

> **Note:** If you want the README files included in the output to be in HTML format, install the `markdown` Python package first:
>
> ```bash
> pip install markdown
> ```

## Usage

### Command-Line Arguments

| Argument    | Description                                                                                                                   |
| ----------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `-c <path>` | Specifies the YAML configuration file path. If not specified, defaults to a `.yaml` file with the same name as the executable |
| `-v`        | Verbose mode. Outputs complete request/response data (headers, parameters, cookies, Schema validation errors, etc.)           |
| `-o <path>` | Outputs all logs to the specified file (in addition to console and per-module log files)                                      |

### Startup Examples

```bash
# Use default config file (a .yaml file with the same name as the executable)
./ApiNatsBridge

# Specify a config file
./ApiNatsBridge -c /etc/apibridge/config.yaml

# Verbose mode
./ApiNatsBridge -c config.yaml -v

# Output all logs to a unified file
./ApiNatsBridge -c config.yaml -o ../logs/all.log
```

## Configuration File Details

The configuration file uses YAML format and contains four main sections.

### Complete Configuration Example

```yaml
# ===========================================================
# ApiNatsBridge Configuration File
# ===========================================================

# --- HTTP API Server Configuration ---
httpapiserver_config:
  # Listen address (set to 0.0.0.0 to listen on all interfaces)
  httpapiserver_host: "127.0.0.1"
  # Listen port
  httpapiserver_port: 9080

  # TLS certificate paths (HTTPS is enabled when both fields are provided; leave empty for HTTP)
  httpapiserver_tls_cert_file: ""
  httpapiserver_tls_key_file: ""

  # Timeout settings (seconds)
  httpapiserver_read_timeout: 5 # Read request timeout
  httpapiserver_write_timeout: 30 # Write response timeout
  httpapiserver_idle_timeout: 60 # Idle connection timeout

  # IP rate limiting
  httpapiserver_enable_rate_limit: true # Whether to enable rate limiting
  httpapiserver_limit_requests: 50 # Maximum number of requests allowed per time window
  httpapiserver_limit_window: 1 # Time window length (seconds)
  httpapiserver_block_duration: 600 # Ban duration after exceeding the limit (seconds)

# --- NATS Client Configuration ---
nats_config:
  # NATS server address and port
  nats_server_host: 127.0.0.1
  nats_server_port: 4222

  # Connection authentication (leave empty to disable authentication)
  nats_user: webapi
  nats_password: your_nats_password_here

  # Client identification name (visible on the NATS server side)
  nats_client_name: ApiNatsBridge

  # Reconnection strategy
  nats_max_reconnects: 5 # Maximum number of reconnection attempts
  nats_reconnect_wait: 2 # Reconnection wait interval (seconds)
  nats_connect_timeout: 10 # Initial connection timeout (seconds)

  # AES symmetric encryption key (length must be 16, 24, or 32 bytes)
  # Leave empty to transmit NATS messages in plain text
  nats_encryption_key: "YOUR_32_CHAR_ENCRYPTION_KEY_HERE!"

  # Per-subject encryption keys (takes precedence over the global key)
  # Set to an empty string to use plain text for that subject
  nats_theme_keys:
    "sensitive.subject": "PER_SUBJECT_KEY_32_CHARS_LONG!!!"
    "public.subject": ""

# --- Bridge Layer Configuration ---
bridge:
  # Multi-language configuration (valid values: en, zh, zh_Hant, ja)
  language:
    log: "zh_Hant" # Log output language
    http: "en" # HTTP response error message language
    cli: "zh_Hant" # CLI-related message language

  # Log output configuration
  log:
    stdout: true # Whether to also output to the console; set to false to write to log files only
    debug: true # Whether to enable debug-level logging; set to false to output Info level and above only
    overwrite: false # Whether to use overwrite mode; set to true to clear existing log files on startup; set to false or omit to append only
    color: true # Whether to use colored console output; set to true or omit for color; set to false for plain text
    files:
      # Independent log file paths for each module; may be configured individually
      # Leave empty or omit to disable file writing for that module
      main: "logs/main.log" # Main process log
      bridge: "logs/bridge.log" # Bridge routing and forwarding log
      http: "logs/http.log" # HTTP request log
      nats: "logs/nats.log" # NATS client event log
      httpstat: "logs/httpstat.log" # HTTP server runtime statistics log
      module: "logs/module.log" # Generic module log

  # Timezone, affects all log timestamps; supports IANA timezone names (e.g., Asia/Shanghai) or hour offsets (e.g., 8, -5)
  timezone: "Asia/Shanghai"

  # Log timestamp format; Go reference time: 2006-01-02 15:04:05
  #   Default: "2006-01-02 15:04:05" (YYYY-MM-DD HH:mm:ss)
  #   Empty "": console output omits timestamps, but log files still use the default format
  #   Custom: e.g., "15:04:05" (time only), "01/02 15:04:05" (MM/DD HH:mm:ss)
  #   Overridable per route by route-level time_format
  # time_format: ""                    # Empty: console without timestamps
  time_format: "2006-01-02 15:04:05" # Default: YYYY-MM-DD HH:mm:ss

  # CDN real IP header list (ordered by priority)
  # Used to extract the client's real IP address from CDN proxy requests
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
    - "Ali-Cdn-Real-Ip" # Alibaba Cloud CDN
    - "Ar-Real-IP" # ArvanCloud

  # IP whitelist for viewing detailed error information (for development and debugging)
  # IPs not in this list will only receive generic error messages
  error_detail_ips:
    - "127.0.0.1"
    - "::1"

  # Global default key for HTTP status code in microservice response JSON (overridable per route)
  http_code_key: "status_code"
  # Global default key for error code in microservice response JSON (overridable per route)
  error_code_key: "error_code"
  # Global default JSON Schema for response body validation (overridable per route)
  # response_schema_body: {}
  # Global default JSON Schema for error response body validation (overridable per route)
  # response_error_schema_body: {}
  # Global default error info display mode (overridable per route)
  #   0=off, 1=log+output, 2=log+output+whitelist, 3=log+output+all, 4=whitelist only, 5=all users
  error_info_show: 1

  # Global request field length limits (0 or omitted means no limit)
  limits:
    path:
      max_length: 2048 # Maximum request path byte length
    headers:
      max_count: 64 # Maximum number of request headers
      max_key_length: 256 # Maximum header name byte length
      max_value_length: 4096 # Maximum header value byte length
    cookies:
      max_count: 32 # Maximum number of cookies
      max_key_length: 256 # Maximum cookie name byte length
      max_value_length: 4096 # Maximum cookie value byte length
    params:
      max_count: 64 # Maximum number of parameters
      max_key_length: 256 # Maximum parameter name byte length
      max_value_length: 4096 # Maximum parameter value byte length
    body:
      max_length: 1048576 # Maximum request body byte length (1MB)

  # Global response field length limits (same structure as limits, can be overridden at route level)
  response_limits:
    body:
      max_length: 1048576 # Maximum response body byte length (1MB)
    headers:
      max_count: 64 # Maximum number of response headers
      max_key_length: 256 # Maximum response header name byte length
      max_value_length: 4096 # Maximum response header value byte length

# --- Route Forwarding Rules ---
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
    # http_code_key: "status_code"  # Response JSON key for HTTP status code; defaults to 200 if not set (optional)
    # error_code_key: "error_code"  # Response JSON key for error code (triggers response_error_schema_body, optional)
    # error_info_show: 2  # Error info display mode (overrides bridge level); 0=off, 1=log, 2=log+whitelist, 3=log+all, 4=whitelist, 5=all (optional)
```

### Configuration Items in Detail

#### `httpapiserver_config` — HTTP Server Configuration

| Item                              | Type   | Description                                                |
| --------------------------------- | ------ | ---------------------------------------------------------- |
| `httpapiserver_host`              | string | Server listen address; `0.0.0.0` listens on all interfaces |
| `httpapiserver_port`              | int    | Listen port                                                |
| `httpapiserver_tls_cert_file`     | string | TLS certificate file path; leave empty for HTTP            |
| `httpapiserver_tls_key_file`      | string | TLS private key file path; leave empty for HTTP            |
| `httpapiserver_read_timeout`      | int    | Read request timeout (seconds)                             |
| `httpapiserver_write_timeout`     | int    | Write response timeout (seconds)                           |
| `httpapiserver_idle_timeout`      | int    | Idle connection timeout (seconds)                          |
| `httpapiserver_enable_rate_limit` | bool   | Whether to enable IP rate limiting                         |
| `httpapiserver_limit_requests`    | int    | Maximum number of requests per time window                 |
| `httpapiserver_limit_window`      | int    | Rate limit time window (seconds)                           |
| `httpapiserver_block_duration`    | int    | Ban duration after exceeding the limit (seconds)           |

#### `nats_config` — NATS Client Configuration

| Item                   | Type   | Description                                                            |
| ---------------------- | ------ | ---------------------------------------------------------------------- |
| `nats_server_host`     | string | NATS server address                                                    |
| `nats_server_port`     | int    | NATS server port                                                       |
| `nats_user`            | string | NATS username; leave empty for no authentication                       |
| `nats_password`        | string | NATS password                                                          |
| `nats_client_name`     | string | Connection identification name                                         |
| `nats_max_reconnects`  | int    | Maximum number of reconnection attempts                                |
| `nats_reconnect_wait`  | int    | Reconnection interval (seconds)                                        |
| `nats_connect_timeout` | int    | Connection timeout (seconds)                                           |
| `nats_encryption_key`  | string | AES global encryption key (16/24/32 bytes); leave empty for plain text |
| `nats_theme_keys`      | map    | Per-subject encryption keys                                            |

#### `bridge` — Bridge Layer Configuration

| Item               | Type     | Description                                                            |
| ------------------ | -------- | ---------------------------------------------------------------------- |
| `language`         | object   | Multi-language configuration (see below)                               |
| `log`              | object   | Log output configuration (see below)                                   |
| `timezone`         | string   | Timezone, affects all log timestamps, e.g., `"Asia/Shanghai"` or `"8"` |
| `time_format`      | string   | Log timestamp format; default `"2006-01-02 15:04:05"` (YYYY-MM-DD HH:mm:ss); empty `""` hides console timestamps but log files still use the default format; overridable per route |
| `cdnheader`        | []string | CDN real IP header priority list                                       |
| `error_detail_ips` | []string | IP whitelist allowed to view detailed errors                           |
| `http_code_key`    | string   | Global default key for HTTP status code in microservice response JSON; overridable per route |
| `error_code_key`   | string   | Global default key for error code in microservice response JSON; overridable per route |
| `response_schema_body`      | object | Global default JSON Schema for response body validation; overridable per route |
| `response_error_schema_body`| object | Global default JSON Schema for error response body validation; overridable per route |
| `error_info_show`  | int      | Global default error info display mode; overridable per route (0=off, 1=log+output, 2=log+output+whitelist, 3=log+output+all, 4=whitelist only, 5=all users) |
| `limits`           | object   | Global request field length limits                                     |
| `response_limits`  | object   | Global response field length limits (same structure as limits)         |

##### `bridge.language` — Multi-Language Configuration

| Item   | Type   | Default     | Description                          |
| ------ | ------ | ----------- | ------------------------------------ |
| `log`  | string | `"zh_Hant"` | Log output language                  |
| `http` | string | `"en"`      | HTTP response error message language |
| `cli`  | string | `"zh_Hant"` | CLI-related message language         |

> Supported language codes: `en`, `zh`, `zh_Hant`, `ja`.
>
> **Important:** Language text modifications should be made in the translation source files `l10n/app_*.arb`. Do **not** directly edit the `l10n/app_localizations_*.go` generated files, as they will be overwritten during regeneration.
>
> After modifying `.arb` files, run the following command to regenerate the Go code:
>
> ```bash
> # Windows
> .\go-gen-l10n.exe -dir .\l10n -pkg l10n -lang zh_Hant
>
> # Or use go generate
> go generate .\src\l10nGlobal.go
> ```
>
> #### Language Style Conventions
>
> | Language Code | Language            | Style                     |
> | ------------- | ------------------- | ------------------------- |
> | `zh`          | Simplified Chinese  | Mainland China (大陆简体) |
> | `zh_Hant`     | Traditional Chinese | Taiwan (臺灣繁體)         |
> | `en`          | English             | Standard                  |
> | `ja`          | Japanese            | Standard                  |
>
> #### ARB Files
>
> | File                   | Language                      |
> | ---------------------- | ----------------------------- |
> | `l10n/app_zh.arb`      | Simplified Chinese (Mainland) |
> | `l10n/app_zh_Hant.arb` | Traditional Chinese (Taiwan)  |
> | `l10n/app_en.arb`      | English                       |
> | `l10n/app_ja.arb`      | Japanese                      |
>
> #### Documentation Files (README)
>
> | File                | Language                      |
> | ------------------- | ----------------------------- |
> | `README.md`         | English                       |
> | `README.zh-Hans.md` | Simplified Chinese (Mainland) |
> | `README.zh-Hant.md` | Traditional Chinese (Taiwan)  |
> | `README.ja.md`      | Japanese                      |
>
> #### Multi-Language Editing Rules
>
> When modifying ARB language files or README documentation files, **all language versions must be updated simultaneously**.
>
> - **ARB files:** After modifying any `.arb` file, run `go-gen-l10n` to regenerate Go code:
>   ```bash
>   # Windows
>   .\go-gen-l10n.exe -dir .\l10n -pkg l10n -lang zh_Hant
>   # Linux / macOS
>   ./go-gen-l10n -dir ./l10n -pkg l10n -lang zh_Hant
>   ```
> - **README files:** When updating any README, apply the same changes to all four language versions (`README.md`, `README.zh-Hans.md`, `README.zh-Hant.md`, `README.ja.md`).
>
> ##### `bridge.log` — Log Output Configuration

| Item        | Type   | Description                                                                                                  |
| ----------- | ------ | ------------------------------------------------------------------------------------------------------------ |
| `stdout`    | bool   | Whether to also output to the console; `false` writes to files only                                          |
| `debug`     | bool   | Whether to enable debug-level logging; `false` enables Info+ only                                            |
| `overwrite` | bool   | Whether to use overwrite mode; `true` clears existing log files on startup; `false` or omission appends only |
| `color`     | bool   | Whether to use colored console output; `true` or omission enables color; `false` uses plain text             |
| `files`     | object | Independent log file paths for each module (see below)                                                       |

##### `bridge.log.files` — Module Log File Paths

| Item       | Type   | Description                                  |
| ---------- | ------ | -------------------------------------------- |
| `main`     | string | Main process log file path                   |
| `bridge`   | string | Bridge routing and forwarding log file path  |
| `http`     | string | HTTP request log file path                   |
| `nats`     | string | NATS client event log file path              |
| `httpstat` | string | HTTP server runtime statistics log file path |
| `module`   | string | Generic module log file path                 |

> Log file paths can be relative or absolute. Directories are created automatically if they do not exist.
> Leave empty or omit a path to disable file writing for that module. If `stdout: false` and all file paths are empty, that module produces no log output.

#### `routes` — Route Rules

| Item                   | Type     | Default         | Description                                                           |
| ---------------------- | -------- | --------------- | --------------------------------------------------------------------- |
| `path`                 | string   | (required)      | HTTP request path                                                     |
| `nats_subject`         | string   | (required)      | Forwarding NATS subject                                               |
| `methods`              | []string | [] (allow all)  | List of allowed HTTP methods                                          |
| `content_type`         | string   | "" (no check)   | Required Content-Type prefix                                          |
| `timeout`              | int      | 30              | NATS response timeout (seconds)                                       |
| `return_fields`        | []string | [] (return all) | Field selection for forwarding to microservice                        |
| `limits`               | object   | -               | Route-level length limits (overrides global)                          |
| `schema_body`          | object   | -               | Request body JSON Schema validation                                   |
| `response_limits`      | object   | -               | Route-level response length limits (overrides global response_limits) |
| `response_schema_body` | object   | -               | Response body JSON Schema validation (same structure as schema_body)  |
| `response_error_schema_body` | object   | -               | Error response body JSON Schema (used when error_code_key is detected; falls back to response_schema_body) |
| `http_code_key`    | string   | "" (defaults to 200) | Microservice response JSON key for HTTP status code (100-599); when specified, this key is removed from the response before returning to client |
| `error_code_key`   | string   | "" (disable)    | Microservice response JSON key for error code (int32); triggers response_error_schema_body when detected |
| `error_info_show`  | int      | 0               | Microservice error info display mode (overrides bridge level); 0=off, 1=log, 2=log+whitelist, 3=log+all, 4=whitelist, 5=all |
| `time_format`      | string   | -               | Route-level log timestamp format (overrides bridge level); same semantics as bridge-level `time_format` |

#### `return_fields` Options

| Field         | Description                                                |
| ------------- | ---------------------------------------------------------- |
| `method`      | HTTP request method                                        |
| `path`        | Request path                                               |
| `headers`     | Request headers (key-value pairs)                          |
| `cookies`     | Cookies (key-value pairs)                                  |
| `remote_addr` | Direct TCP address (including port)                        |
| `ip`          | Resolved real client IP                                    |
| `params`      | URL query parameters and form parameters (key-value pairs) |
| `body`        | Raw request body content                                   |

#### `schema_body` JSON Schema Validation

In addition to standard JSON Schema fields, two control keys are supported:

| Control Key | Type   | Description                                                        |
| ----------- | ------ | ------------------------------------------------------------------ |
| `root_type` | string | Expected root node type (e.g., `object`, `array`)                  |
| `strict`    | bool   | Strict mode; when `true`, rejects fields not defined in the Schema |

Other fields follow the [JSON Schema](https://json-schema.org/) specification (e.g., `required`, `properties`, `type`, etc.).

## Usage Examples

### Scenario 1: Basic JSON API Gateway

Forward frontend JSON requests to a user microservice:

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

Client request:

```bash
curl -X POST http://127.0.0.1:9080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"123456"}'
```

### Scenario 2: Form Submission to JSON

Automatically convert traditional HTML form submissions to JSON before forwarding:

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

Client request:

```bash
curl -X POST http://127.0.0.1:9080/api/feedback \
  -d "message=Great+service&rating=5"
```

The form field `rating=5` is automatically converted from string to integer `5` based on the Schema's `type: integer` definition.

### Scenario 3: Forward Only Specific Fields

Send only the HTTP method and path to the microservice (suitable for simple health-check routes):

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

### Scenario 4: Measuring Latency with the Ping Endpoint

```bash
# Send a GET request with a timestamp query parameter
curl "http://127.0.0.1:9080/ping?timestamp=$(date +%s%3N)"

# Example response: {"pong": 3, "ip": "127.0.0.1", "servertime": 1716000000042}  (unit: milliseconds)
```

The `/ping` route is forwarded via NATS to the `ApiNatsBridgeTemplate` microservice, which calculates the delay and returns the client IP.

### Scenario 5: Encrypted NATS Communication

Use a dedicated encryption key for a sensitive payment service:

```yaml
nats_config:
  nats_encryption_key: "DEFAULT_GLOBAL_KEY_32_CHARS_OK!!"
  nats_theme_keys:
    "payment.process": "PAYMENT_DEDICATED_KEY_32_CHARS!!"
    "public.notify": "" # This subject uses plain text transmission

routes:
  - path: "/api/pay"
    nats_subject: "payment.process"
    methods: ["POST"]
    content_type: "application/json"
    timeout: 60
```

## Client IP Resolution Priority

When resolving the client's real IP address, the following priority order is used:

1. CDN-specific headers (the headers listed in `cdnheader`, searched in list order)
2. `X-Real-IP` header
3. The first valid IP in the `X-Forwarded-For` header
4. The TCP connection's remote address (`RemoteAddr`)

All candidate values are validated as legal IP addresses.

## Microservice Development Guide

Backend microservices simply need to subscribe to the corresponding NATS subject, receive `BridgeRequest` JSON, process it, and return `BridgeResponse` JSON.

### Go Example

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

Microservice processing logic:

1. Subscribe to a NATS subject (e.g., `user_service`)
2. Upon receiving a message, parse it as `BridgeRequest`
3. Execute business logic
4. Construct `BridgeResponse` JSON and return it

If the microservice returns something other than valid `BridgeResponse` JSON, ApiNatsBridge will return the raw response string directly to the client as an HTTP 200 response body.

## Local Service Environment

The project includes a complete local testing environment (located in the `test/` directory) and a template microservice (`ApiNatsBridgeTemplate/`):

```bash
# Windows — one-click start (starts NATS Server, ApiNatsBridge, ApiNatsBridgeTemplate)
serve.bat

# Linux / macOS — one-click start
chmod +x serve.sh
./serve.sh
```

```bash
# One-click stop of all services
serve_stop.bat   # Windows
./serve_stop.sh  # Linux / macOS
```

> **Warning:** The serve scripts use the default ports from the example configuration files. Running them will conflict with any services already listening on those ports (NATS port 4222, HTTP port 9080). Stop any existing services first.

```bash
# One-click stop of all services
serve_stop.bat
```

Startup process:

1. Start the local NATS server (`test/nats-server/`)
2. Start the ApiNatsBridge main program
3. Start the ApiNatsBridgeTemplate microservice (`ApiNatsBridgeTemplate/`)

After startup, `serve.bat` will automatically send a test request. You can also send one manually:

```bash
# Send a ping request (ApiNatsBridgeTemplate responds with {pong, ip, servertime})
curl "http://127.0.0.1:9080/ping?timestamp=0"

# Windows PowerShell
Invoke-RestMethod -Uri ("http://127.0.0.1:9080/ping?timestamp=" + [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())
```

### ApiNatsBridgeTemplate

The template microservice subscribes to the `ping_req` NATS subject, reads the `timestamp` parameter, and returns `{"pong": <delay_ms>, "ip": "<client_ip>", "servertime": <server_timestamp_ms>}`.

```bash
# Start with default config
go run ./ApiNatsBridgeTemplate/ -c ApiNatsBridgeTemplate/config.yaml
```

For more details, see `ApiNatsBridgeTemplate/README.md`.

## Dependencies

| Package                                                                                                   | Purpose                    |
| --------------------------------------------------------------------------------------------------------- | -------------------------- |
| [github.com/google/uuid](https://github.com/google/uuid)                                                  | UUID generation            |
| [github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver](https://github.com/kagurazakayashi/libNyaruko_Go) | HTTP API server framework  |
| [github.com/kagurazakayashi/libNyaruko_Go/nyanats](https://github.com/kagurazakayashi/libNyaruko_Go)      | NATS client wrapper        |
| [gopkg.in/yaml.v3](https://github.com/go-yaml/yaml)                                                       | YAML configuration parsing |
| [github.com/santhosh-tekuri/jsonschema/v6](https://github.com/santhosh-tekuri/jsonschema)                 | JSON Schema validation     |
| [github.com/nats-io/nats.go](https://github.com/nats-io/nats.go)                                          | NATS Go client             |

## License

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
