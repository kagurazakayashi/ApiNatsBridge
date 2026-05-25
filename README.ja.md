[English](README.md) | [简体中文](README.zh-Hans.md) | [繁體中文](README.zh-Hant.md) | [日本語](README.ja.md)

![ApiNatsBridge](ico/icon.png)

# ApiNatsBridge

軽量 HTTP-to-NATS ゲートウェイブリッジ。標準 HTTP REST リクエストを NATS メッセージに変換してバックエンドのマイクロサービスに転送し、マイクロサービスからのレスポンスを HTTP クライアントに返します。マイクロサービスアーキテクチャにおける API ゲートウェイ層としての利用に適しています。

## 機能

- **HTTP から NATS へのリクエスト転送** — HTTP リクエストを受信し、JSON 構造体にシリアライズして NATS Request/Reply モードでバックエンドのマイクロサービスに送信
- **YAML 宣言的ルーティング設定** — 設定ファイルで HTTP パスと NATS Subject のマッピングを定義
- **JSON Schema リクエストボディ検証** — リクエストボディの JSON Schema 検証をサポートし、不正なリクエストを拒否
- **フォームから JSON への自動変換** — `application/x-www-form-urlencoded` フォームデータを Schema に基づいて型変換し、JSON として転送
- **選択的フィールド転送** — `return_fields` 設定により必要なリクエスト情報のみをマイクロサービスに渡す
- **リクエストフィールド長制限** — グローバルおよびルートレベルでのパス、ヘッダー、Cookie、パラメータ、リクエストボディの長さ制限
- **AES 暗号化 NATS 通信** — グローバルキーおよび Subject ごとの個別キーによる AES 対称暗号化
- **CDN 実 IP 解決** — Cloudflare、Akamai、Fastly、AWS CloudFront、Alibaba Cloud CDN など主要 CDN の実際のクライアント IP 抽出をサポート
- **IP レート制限** — IP 単位のリクエスト頻度制限とブロック機構を内蔵
- **TLS/HTTPS サポート** — 証明書を設定することで HTTPS を有効化可能
- **`/ping` エンドポイント** — NATS マイクロサービス経由の遅延測定エンドポイント
- **トークン検証** — オプションの認証トークン検証、NATS マイクロサービス（例：NyarukoLogin UserValidator）経由で実行、パスホワイトリストとメモリキャッシュをサポート
- **IP ホワイトリストエラー詳細** — 指定された IP のみが詳細なエラー情報を閲覧可能、本番環境では一般的なエラーのみを返す
- **マイクロサービスエラー情報表示制御** — `error_info_show` パラメータで、マイクロサービスのエラー内容を記録・出力するか、およびその内容をホワイトリストの IP ユーザーまたは全ユーザーに返すかを制御
- **多言語サポート (i18n/l10n)** — ログ、HTTP レスポンス、CLI ヘルプテキストを多言語対応、設定ファイルで個別に設定可能（en、zh、zh_Hant、ja 対応）
- **グレースフルシャットダウン** — システムシグナルを捕捉後、HTTP サーバーを順序立てて停止し、NATS サブスクリプションを解除して接続を切断

## アーキテクチャ概要

```
┌──────────┐         ┌───────────────────┐         ┌────────────────┐
│  HTTP    │  HTTP   │  ApiNatsBridge    │  NATS   │  Microservice  │
│  Client  │ ──────> │  (Gateway/Bridge) │ ──────> │  (Backend)     │
│          │ <────── │                   │ <────── │                │
└──────────┘         └───────────────────┘         └────────────────┘
```

1. HTTP クライアントが ApiNatsBridge にリクエストを送信
2. ApiNatsBridge がルーティングマッチング、メソッド検証、Content-Type 検証、長さ制限検証、Schema 検証を実行
3. リクエストデータを `BridgeRequest` JSON にシリアライズし、NATS Request で対応する Subject に送信
4. バックエンドのマイクロサービスがリクエストを処理し、`BridgeResponse` JSON を返す
5. ApiNatsBridge がレスポンス内容を HTTP レスポンスとしてクライアントに返す

## ログモジュールプレフィックス

実行時ログ出力では、以下のプレフィックスで発生元モジュールを区別します：

| プレフィックス  | ソースファイル      | 色     | 用途                               |
| --------------- | ------------------- | ------ | ---------------------------------- |
| `[MAIN]`        | `src/logger.go`     | Cyan   | メインプロセスのライフサイクルログ |
| `[NATS]`        | `src/natsLogger.go` | Green  | NATS クライアントの接続とイベント  |
| `[BRIDGE]`      | `src/logger.go`     | Yellow | ブリッジルーティングと転送ログ     |
| `[HTTP]`        | `src/logger.go`     | Blue   | HTTP リクエストログ行              |
| `[HTTPINFO]`    | `src/logger.go`     | Blue   | HTTP ヘッダー、Cookie 詳細（デバッグ） |
| `[HTTPBODY]`    | `src/logger.go`     | Blue   | HTTP リクエスト/レスポンスボディ（デバッグ） |
| `[NATSBODY]`    | `src/logger.go`     | Green  | NATS リクエスト/リプライペイロード（デバッグ） |
| `[STATUS]`      | `src/logger.go`     | Purple | 定期的 HTTP+NATS 実行時統計             |
| `[MODULE]`      | `src/logger.go`     | Cyan   | 汎用モジュールログ                 |
| `[NATS][ERROR]` | `src/logger.go`     | Red    | NATS 接続エラー                    |
| `[HTTP][ERROR]` | `src/logger.go`     | Red    | HTTP サーバーエラー                |
| `[MAIN][ERROR]` | `src/logger.go`     | Red    | メインプロセスの致命的エラー       |

すべてのプレフィックスは、ローカルライブラリ `libNyaruko_Go/nyalog` の `LogCC()` 関数を通じて出力されます。

## データ構造

### BridgeRequest（マイクロサービスに送信）

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

### BridgeResponse（マイクロサービスが返す）

```json
{
  "status_code": 200,
  "headers": { "Content-Type": "application/json; charset=utf-8" },
  "body": "{\"result\":\"success\"}"
}
```

## インストールとビルド

### 前提条件

- Go 1.24.4 以上
- 本プロジェクトは Git サブモジュールで依存関係を管理しています。クローン後にサブモジュールの初期化が必要です（下記参照）

### Git サブモジュールの初期化

本プロジェクトには以下の Git サブモジュールが含まれます：

| サブモジュール                                                                 | パス                     | 説明                                                             |
| ------------------------------------------------------------------------------ | ------------------------ | ---------------------------------------------------------------- |
| [libNyaruko_Go](https://github.com/kagurazakayashi/libNyaruko_Go)              | `libNyaruko_Go/`         | 依存ライブラリ（`nyalog`、`nyanats`、`nyaapiserver` モジュール） |
| [ApiNatsBridgeTemplate](https://github.com/MasaeProject/ApiNatsBridgeTemplate) | `ApiNatsBridgeTemplate/` | マイクロサービステンプレートプロジェクト                         |

クローン時にサブモジュールも一緒に取得：

```bash
git clone --recursive <repo_url>
```

クローン済みのプロジェクトでサブモジュールを初期化：

```bash
git submodule init
git submodule update
```

または 1 つのコマンドにまとめて：

```bash
git submodule update --init
```

### サブモジュールの強制更新（ローカル変更を無視）

サブモジュールにローカルの変更があり通常の更新ができない場合、以下のコマンドを使用します：

```bash
# サブモジュール内のすべてのローカル変更を破棄し、強制更新
git submodule foreach --recursive "git reset --hard && git clean -fd"
git submodule update --init --recursive --force
```

### go-gen-l10n ツールのビルド

サブモジュール `libNyaruko_Go` にはローカライズコード生成ツール `go-gen-l10n` が含まれています。そのディレクトリに入り、リソースを生成してビルドします：

```bash
# サブモジュールのディレクトリに入る
cd libNyaruko_Go/go-gen-l10n

# Windows リソースを生成（該当する場合）、その後ビルド
go generate .
go build .

# 実行ファイルをプロジェクトルートにコピー（go generate ./src/l10nGlobal.go が見つけられるように）
# Linux / macOS
cd ../..
cp libNyaruko_Go/go-gen-l10n/go-gen-l10n .

# Windows
cd ..\..
copy libNyaruko_Go\go-gen-l10n\go-gen-l10n.exe .
```

ビルド後、プロジェクトのルートディレクトリに `go-gen-l10n`（または `go-gen-l10n.exe`）が生成されます。`l10n/app_*.arb` 言語ファイルを変更した後、以下のコマンドを実行してコードを再生成します：

```bash
# Linux / macOS
./go-gen-l10n -dir ./l10n -pkg l10n -lang zh_Hant

# Windows
.\go-gen-l10n.exe -dir .\l10n -pkg l10n -lang zh_Hant
```

または `go generate` を使用：

```bash
go generate ./src/l10nGlobal.go
```

### Windows 実行可能ファイルのアイコン埋め込み

まず `go-winres` ツールをインストールします：

```bash
go install github.com/tc-hib/go-winres@latest
```

次にリソースファイル（`.syso`）を生成すると、以降の `go build` で自動的にリンクされます：

```bash
# go generate で go-winres を自動呼び出し（推奨）
go generate ./...

# または手動実行
go-winres make
```

> リソース設定ファイルは `winres/winres.json`、アイコンソースファイルは `ico/icon.png` にあります。
> `.syso` ファイルは `.gitignore` で無視されているため、ビルド前に毎回再生成する必要があります。
> この手順はオプションです——`go run .` は `.syso` ファイルなしでも正常に動作します。

### 現在のプラットフォーム向けビルド

```bash
# まずアイコンリソースを生成（Windows プラットフォームで必要、その他のプラットフォームではスキップ可能）
go generate ./...

# ビルド（Windows の場合は .exe を付加）
go build -o ApiNatsBridge .
```

### マルチプラットフォームクロスコンパイル

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

> **ヒント：** Windows PowerShell で環境変数を設定する場合は `$env:GOOS="linux"; $env:GOARCH="amd64"` を実行してから `go build` を実行します。

### バッチマルチプラットフォームビルド

提供されているビルドスクリプトを使用して、サポートされているすべてのプラットフォーム向けに一括コンパイルできます：

**Windows：**

```bat
build.bat
```

**Linux / macOS：**

```bash
chmod +x build.sh
./build.sh
```

> **注意：** 出力に HTML 形式の README ファイルを含める場合は、Python の `markdown` パッケージを事前にインストールしてください：
>
> ```bash
> pip install markdown
> ```

## 使用方法

### コマンドラインパラメータ

| パラメータ  | 説明                                                                                                             |
| ----------- | ---------------------------------------------------------------------------------------------------------------- |
| `-c <パス>` | YAML 設定ファイルのパスを指定します。指定がない場合は、実行ファイルと同じ名前の `.yaml` ファイルがデフォルトです |
| `-o <パス>` | すべてのログを指定ファイルに出力します（コンソールと各モジュールのログファイルに加えて）                       |

### 起動例

```bash
# デフォルト設定ファイルを使用（実行ファイルと同じ名前の .yaml ファイル）
./ApiNatsBridge

# 設定ファイルを指定
./ApiNatsBridge -c /etc/apibridge/config.yaml

# すべてのログを統合ファイルに出力
./ApiNatsBridge -c config.yaml -o ../logs/all.log
```

## 設定ファイル詳細

設定ファイルは YAML 形式で、以下の 4 つの主要セクションで構成されます。

### 完全な設定例

```yaml
# ===========================================================
# ApiNatsBridge 設定ファイル
# ===========================================================

# --- HTTP API サーバー設定 ---
httpapiserver_config:
  # リッスンアドレス（0.0.0.0 に設定するとすべてのネットワークインターフェースでリッスン）
  httpapiserver_host: "127.0.0.1"
  # リッスンポート
  httpapiserver_port: 9080

  # TLS 証明書パス（両方指定時に HTTPS が有効、空の場合は HTTP を使用）
  httpapiserver_tls_cert_file: ""
  httpapiserver_tls_key_file: ""

  # タイムアウト設定（秒）
  httpapiserver_read_timeout: 5 # リクエスト読み取りタイムアウト
  httpapiserver_write_timeout: 30 # レスポンス書き込みタイムアウト
  httpapiserver_idle_timeout: 60 # アイドル接続タイムアウト

  # IP レート制限
  httpapiserver_enable_rate_limit: true # レート制限を有効にするか
  httpapiserver_limit_requests: 50 # 時間枠あたりの最大リクエスト数
  httpapiserver_limit_window: 1 # 時間枠の長さ（秒）
  httpapiserver_block_duration: 600 # 制限超過後のブロック時間（秒）

# --- NATS クライアント設定 ---
nats_config:
  # NATS サーバーのアドレスとポート
  nats_server_host: 127.0.0.1
  nats_server_port: 4222

  # 接続認証（空の場合は認証を無効化）
  nats_user: webapi
  nats_password: your_nats_password_here

  # クライアント識別名（NATS サーバー側で表示）
  nats_client_name: ApiNatsBridge

  # 再接続ポリシー
  nats_max_reconnects: 5 # 最大再接続回数
  nats_reconnect_wait: 2 # 再接続待機間隔（秒）
  nats_connect_timeout: 10 # 初回接続タイムアウト（秒）

  # AES 対称暗号化キー（長さは 16、24、または 32 バイトである必要があります）
  # 空の場合は NATS メッセージを平文で送信
  nats_encryption_key: "YOUR_32_CHAR_ENCRYPTION_KEY_HERE!"

  # Subject ごとに個別の暗号化キーを設定（グローバルキーより優先）
  # 空文字列に設定すると、その Subject は平文で送信
  nats_theme_keys:
    "sensitive.subject": "PER_SUBJECT_KEY_32_CHARS_LONG!!!"
    "public.subject": ""

# --- ブリッジ層設定 ---
bridge:
  # 多言語設定（en、zh、zh_Hant、ja 対応）
  language:
    log: "zh_Hant" # ログ出力言語
    http: "en" # HTTP レスポンスエラーメッセージ言語
    cli: "zh_Hant" # コマンドライン関連メッセージ言語

  # ログ出力設定
  log:
    stdout: true # コンソールにも同時出力するか、false の場合はログファイルのみに書き込み
    debug: ["HTTP", "NATS", "LIMIT"] # デバッグモード：空配列 [] = デバッグ無効；オプション：HTTP、NATS、LIMIT
    overwrite: false # 上書きモードを使用するか、true の場合は起動時に既存のログファイルをクリア、false または未指定の場合は追記のみ
    color: true # カラーコンソール出力を使用するか、true または未指定の場合はカラー、false の場合はプレーンテキスト
    files:
      # 各モジュールの個別ログファイルパス、それぞれ設定可能
      # 空または未指定の場合、そのモジュールはファイルに書き込まれません
      main: "logs/main.log" # メインプロセスログ
      bridge: "logs/bridge.log" # ブリッジルーティングと転送ログ
      http: "logs/http.log" # HTTP リクエストログ
      nats: "logs/nats.log" # NATS クライアントイベントログ
      status: "logs/status.log" # HTTP+NATS 状態統計ログ
      module: "logs/module.log" # 汎用モジュールログ

  # タイムゾーン、すべてのログタイムスタンプに影響、IANA タイムゾーン名（例：Asia/Tokyo）または時間オフセット（例：9、-5）をサポート
  timezone: "Asia/Shanghai"

  # ログタイムスタンプ表示形式；Go 参照時間形式：2006-01-02 15:04:05
  #   デフォルト値："2006-01-02 15:04:05"（YYYY-MM-DD HH:mm:ss）
  #   空文字列 "" の場合：コンソールにタイムスタンプを表示しないがログファイルはデフォルト形式で記録
  #   カスタム形式例："15:04:05"（時刻のみ）、"01/02 15:04:05"（MM/DD HH:mm:ss）
  #   ルートレベルの time_format で上書き可能
  # time_format: ""                    # 空文字列：コンソールにタイムスタンプなし
  time_format: "2006-01-02 15:04:05" # デフォルト値：YYYY-MM-DD HH:mm:ss

  # CDN 実 IP ヘッダーリスト（優先度順）
  # CDN プロキシリクエストからクライアントの実際の IP アドレスを抽出するために使用
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

  # 詳細なエラー情報の閲覧を許可する IP ホワイトリスト（開発デバッグ用）
  # このリストにない IP には一般的なエラーメッセージのみが返されます
  error_detail_ips:
    - "127.0.0.1"
    - "::1"

  # マイクロサービス応答 JSON 内の HTTP ステータスコードのグローバルデフォルトキー名（ルート単位で上書き可能）
  http_code_key: "status_code"
  # マイクロサービス応答 JSON 内のエラーコードのグローバルデフォルトキー名（ルート単位で上書き可能）
  error_code_key: "error_code"
  # 応答ボディ JSON Schema 検証のグローバルデフォルトルール（ルート単位で上書き可能）
  # response_schema_body: {}
  # エラー応答ボディ JSON Schema 検証のグローバルデフォルトルール（ルート単位で上書き可能）
  # response_error_schema_body: {}
  # マイクロサービスエラー情報表示モードのグローバルデフォルト値（ルート単位で上書き可能）
  #   0=記録しない、1=記録+出力、2=記録+出力+ホワイトリストに表示、3=記録+出力+全員に表示、4=記録しない+ホワイトリストに表示、5=記録しない+全員に表示
  error_info_show: 1

  # グローバルリクエストフィールド長制限（0 または省略は制限なし）
  limits:
    path:
      max_length: 2048 # リクエストパスの最大バイト長
    headers:
      max_count: 64 # リクエストヘッダーの最大数
      max_key_length: 256 # ヘッダー名の最大バイト長
      max_value_length: 4096 # ヘッダー値の最大バイト長
    cookies:
      max_count: 32 # Cookie の最大数
      max_key_length: 256 # Cookie 名の最大バイト長
      max_value_length: 4096 # Cookie 値の最大バイト長
    params:
      max_count: 64 # パラメータの最大数
      max_key_length: 256 # パラメータ名の最大バイト長
      max_value_length: 4096 # パラメータ値の最大バイト長
    body:
      max_length: 1048576 # リクエストボディの最大バイト長（1MB）

  # グローバルレスポンスフィールド長制限（limits と同じ構造、ルートレベルで上書き可能）
  response_limits:
    body:
      max_length: 1048576 # レスポンスボディ最大バイト長（1MB）
    headers:
      max_count: 64 # レスポンスヘッダー最大数
      max_key_length: 256 # レスポンスヘッダー名最大バイト長
      max_value_length: 4096 # レスポンスヘッダー値最大バイト長

# --- ルーティング転送ルール ---
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
    # http_code_key: "status_code"  # レスポンス JSON 内の HTTP ステータスコードキー名、未指定時はデフォルト 200（オプション）
    # error_code_key: "error_code"  # レスポンス JSON 内のエラーコードキー名、検出時に response_error_schema_body 検証をトリガー（オプション）
    # error_info_show: 2  # エラー情報表示モード（bridge レベルを上書き）；0=記録しない、1=記録、2=記録+ホワイトリストに表示、3=記録+全員に表示、4=記録しない+ホワイトリストに表示、5=記録しない+全員に表示（オプション）
```

### 設定項目詳細

#### `httpapiserver_config` — HTTP サーバー設定

| 設定項目                          | 型     | 説明                                                                                 |
| --------------------------------- | ------ | ------------------------------------------------------------------------------------ |
| `httpapiserver_host`              | string | サーバーリッスンアドレス、`0.0.0.0` はすべてのネットワークインターフェースでリッスン |
| `httpapiserver_port`              | int    | リッスンポート                                                                       |
| `httpapiserver_tls_cert_file`     | string | TLS 証明書ファイルパス、空の場合は HTTP を使用                                       |
| `httpapiserver_tls_key_file`      | string | TLS 秘密鍵ファイルパス、空の場合は HTTP を使用                                       |
| `httpapiserver_read_timeout`      | int    | リクエスト読み取りタイムアウト（秒）                                                 |
| `httpapiserver_write_timeout`     | int    | レスポンス書き込みタイムアウト（秒）                                                 |
| `httpapiserver_idle_timeout`      | int    | アイドル接続タイムアウト（秒）                                                       |
| `httpapiserver_enable_rate_limit` | bool   | IP レート制限を有効にするか                                                          |
| `httpapiserver_limit_requests`    | int    | 時間枠内の最大リクエスト数                                                           |
| `httpapiserver_limit_window`      | int    | レート制限時間枠（秒）                                                               |
| `httpapiserver_block_duration`    | int    | 制限超過後のブロック時間（秒）                                                       |

#### `nats_config` — NATS クライアント設定

| 設定項目               | 型     | 説明                                                        |
| ---------------------- | ------ | ----------------------------------------------------------- |
| `nats_server_host`     | string | NATS サーバーアドレス                                       |
| `nats_server_port`     | int    | NATS サーバーポート                                         |
| `nats_user`            | string | NATS ユーザー名、空の場合は認証なし                         |
| `nats_password`        | string | NATS パスワード                                             |
| `nats_client_name`     | string | 接続識別名                                                  |
| `nats_max_reconnects`  | int    | 最大再接続回数                                              |
| `nats_reconnect_wait`  | int    | 再接続間隔（秒）                                            |
| `nats_connect_timeout` | int    | 接続タイムアウト（秒）                                      |
| `nats_encryption_key`  | string | AES グローバル暗号化キー（16/24/32 バイト）、空の場合は平文 |
| `nats_theme_keys`      | map    | Subject ごとに個別設定する暗号化キー                        |

#### `bridge` — ブリッジ層設定

| 設定項目           | 型       | 説明                                                                            |
| ------------------ | -------- | ------------------------------------------------------------------------------- |
| `language`         | object   | 多言語設定（下記参照）                                                          |
| `log`              | object   | ログ出力設定（下記参照）                                                        |
| `timezone`         | string   | タイムゾーン、すべてのログタイムスタンプに影響、例：`"Asia/Tokyo"` または `"9"` |
| `time_format`      | string   | ログタイムスタンプ表示形式；デフォルト `"2006-01-02 15:04:05"`（YYYY-MM-DD HH:mm:ss）；空文字列 `""` の場合はコンソールにタイムスタンプを表示しないがログファイルはデフォルト形式で記録；ルート単位で上書き可能 |
| `cdnheader`        | []string | CDN 実 IP ヘッダー優先度リスト                                                  |
| `error_detail_ips` | []string | 詳細エラーの閲覧を許可する IP ホワイトリスト                                    |
| `http_code_key`    | string   | マイクロサービス応答 JSON 内の HTTP ステータスコードのグローバルデフォルトキー名；ルート単位で上書き可能 |
| `error_code_key`   | string   | マイクロサービス応答 JSON 内のエラーコードのグローバルデフォルトキー名；ルート単位で上書き可能 |
| `response_schema_body`      | object | 応答ボディ JSON Schema 検証のグローバルデフォルトルール；ルート単位で上書き可能 |
| `response_error_schema_body`| object | エラー応答ボディ JSON Schema 検証のグローバルデフォルトルール；ルート単位で上書き可能 |
| `error_info_show`  | int      | マイクロサービスエラー情報表示モードのグローバルデフォルト値；ルート単位で上書き可能（0=記録しない、1=記録+出力、2=記録+出力+ホワイトリストに表示、3=記録+出力+全員に表示、4=記録しない+ホワイトリストに表示、5=記録しない+全員に表示） |
| `limits`           | object   | グローバルリクエストフィールド長制限                         |
| `response_limits`  | object   | グローバルレスポンスフィールド長制限（構造は limits と同じ） |
| `token`            | object   | トークン検証設定（[トークン検証](#トークン検証)を参照）      |
| `max_concurrent` | int | `0`（無制限） | グローバル最大並行 NATS 転送リクエスト数、超過時に 503 |
| `http_max_concurrent` | int | `0`（無制限） | HTTP 層の最大並行リクエスト処理数、超過時に 503 |

##### `bridge.language` — 多言語設定

| 設定項目 | 型     | デフォルト値 | 説明                                |
| -------- | ------ | ------------ | ----------------------------------- |
| `log`    | string | `"zh_Hant"`  | ログ出力言語                        |
| `http`   | string | `"en"`       | HTTP レスポンスエラーメッセージ言語 |
| `cli`    | string | `"zh_Hant"`  | コマンドライン関連メッセージ言語    |

> サポートされる言語コード：`en`（英語）、`zh`（簡体字中国語）、`zh_Hant`（繁体字中国語）、`ja`（日本語）。
>
> **重要：** 言語テキストの変更は翻訳ソースファイル `l10n/app_*.arb` で行ってください。生成ファイル `l10n/app_localizations_*.go` を**直接編集しないでください**。これらは再生成時に上書きされます。
>
> `.arb` ファイルを変更した後、以下のコマンドを実行して Go コードを再生成する必要があります：
>
> ```bash
> # Windows
> .\go-gen-l10n.exe -dir .\l10n -pkg l10n -lang zh_Hant
>
> # または go generate を使用
> go generate .\src\l10nGlobal.go
> ```
>
> #### 言語スタイル規約
>
> | 言語コード | 言語         | スタイル                 |
> | ---------- | ------------ | ------------------------ |
> | `zh`       | 簡体字中国語 | 大陸スタイル（大陸簡体） |
> | `zh_Hant`  | 繁体字中国語 | 台湾スタイル（臺灣繁體） |
> | `en`       | 英語         | 標準                     |
> | `ja`       | 日本語       | 標準                     |
>
> #### ARB ファイル
>
> | ファイル               | 言語                 |
> | ---------------------- | -------------------- |
> | `l10n/app_zh.arb`      | 簡体字中国語（大陸） |
> | `l10n/app_zh_Hant.arb` | 繁体字中国語（台湾） |
> | `l10n/app_en.arb`      | 英語                 |
> | `l10n/app_ja.arb`      | 日本語               |

##### `bridge.log` — ログ出力設定

| 設定項目    | 型     | 説明                                                                                                          |
| ----------- | ------ | ------------------------------------------------------------------------------------------------------------- |
| `stdout`    | bool   | コンソールにも同時出力するか、`false` の場合はファイルのみ                                                    |
| `debug`     | []string | デバッグモードフラグ配列；空 `[]` = Info+ のみ。オプション：`"HTTP"`（HTTP通信全体）、`"NATS"`（NATS通信全体）、`"LIMIT"`（拒否リクエストの違反詳細） |
| `overwrite` | bool   | 上書きモードかどうか、`true` の場合は起動時に既存のログファイルをクリア、`false` または未指定の場合は追記のみ |
| `color`     | bool   | カラーコンソール出力かどうか、`true` または未指定の場合はカラー、`false` の場合はプレーンテキスト             |
| `status_interval_seconds` | int | STATUS ログ出力間隔秒数；デフォルト 60 秒                                                         |
| `files`     | object | 各モジュールの個別ログファイルパス（下記参照）                                                                |

##### `bridge.log.files` — モジュールログファイルパス

| 設定項目   | 型     | 説明                                       |
| ---------- | ------ | ------------------------------------------ |
| `main`     | string | メインプロセスログファイルパス             |
| `bridge`   | string | ブリッジルーティングと転送ログファイルパス |
| `http`     | string | HTTP リクエストログファイルパス            |
| `nats`     | string | NATS クライアントイベントログファイルパス  |
| `status`   | string | HTTP+NATS 状態統計ログファイルパス         |
| `module`   | string | 汎用モジュールログファイルパス             |

> ログファイルパスは相対パスでも絶対パスでも構いません。ディレクトリが存在しない場合は自動的に作成されます。
> パスが空または未指定の場合、そのモジュールはファイルに書き込まれません。`stdout: false` かつすべてのファイルパスが空の場合、そのモジュールのログ出力は行われません。

#### `routes` — ルーティングルール

| 設定項目               | 型       | デフォルト値     | 説明                                                                |
| ---------------------- | -------- | ---------------- | ------------------------------------------------------------------- |
| `path`                 | string   | （必須）         | HTTP リクエストパス                                                 |
| `nats_subject`         | string   | （必須）         | 転送先 NATS Subject                                                 |
| `methods`              | []string | []（すべて許可） | 許可する HTTP メソッドリスト                                        |
| `content_type`         | string   | ""（検証しない） | 要求する Content-Type プレフィックス                                |
| `timeout`              | int      | 30               | NATS レスポンスタイムアウト（秒）                                   |
| `return_fields`        | []string | []（すべて返す） | マイクロサービスに転送するフィールド選択                            |
| `limits`               | object   | -                | ルートレベル長制限（グローバル設定を上書き）                        |
| `schema_body`          | object   | -                | リクエストボディ JSON Schema 検証                                   |
| `response_limits`      | object   | -                | ルートレベルレスポンス長制限（グローバル response_limits を上書き） |
| `response_schema_body` | object   | -                | レスポンスボディ JSON Schema 検証（構造は schema_body と同じ）      |
| `response_error_schema_body` | object   | -                | エラーレスポンスボディ JSON Schema 検証（error_code_key 検出時に使用、未設定の場合は response_schema_body を使用） |
| `http_code_key`    | string   | ""（デフォルト200） | マイクロサービス応答 JSON 内の HTTP ステータスコードを示すキー名（100-599）；指定時はクライアントに返す前にこのキーを除去 |
| `error_code_key`   | string   | ""（無効）       | マイクロサービス応答 JSON 内のエラーコードを示すキー名（int32）；検出時に response_error_schema_body 検証をトリガー |
| `error_info_show`  | int      | 0                | マイクロサービスエラー情報表示モード（bridge レベルを上書き）；0=記録しない、1=記録、2=記録+ホワイトリストに表示、3=記録+全員に表示、4=記録しない+ホワイトリストに表示、5=記録しない+全員に表示 |
| `time_format`      | string   | -               | ルートレベルログタイムスタンプ表示形式（bridge レベルを上書き）；意味は bridge レベルの `time_format` と同じ |
| `max_concurrent` | int | `0`（グローバルに従う） | ルートレベルの最大並行 NATS 転送リクエスト数；0 はグローバル制限に従う；超過時に 503 |
| `token_fields` | []string | - | **（後方互換、`return_fields` の使用を推奨）** レガシーのトークン claims 注入設定。リストされたフィールドは検証応答から抽出されトップレベルフィールドとして注入されます。新規デプロイでは `return_fields` にトークン claim 名を直接追加してください |

#### `return_fields` 選択可能な値

| フィールド名  | ソース | 説明                                                       |
| ------------- | ------ | ---------------------------------------------------------- |
| `method`      | HTTP   | HTTP リクエストメソッド                                    |
| `path`        | HTTP   | リクエストパス                                             |
| `headers`     | HTTP   | リクエストヘッダー（キーと値のペア）                       |
| `cookies`     | HTTP   | Cookie（キーと値のペア）                                   |
| `remote_addr` | HTTP   | 直接 TCP アドレス（ポート含む）                            |
| `ip`          | HTTP   | 解決後の実際のクライアント IP                              |
| `params`      | HTTP   | URL クエリパラメータとフォームパラメータ（キーと値のペア） |
| `body`        | HTTP   | リクエストボディの生内容                                   |
| *その他*      | **トークン** | **トークン claim フィールド** — 上記 8 つの既知フィールドに含まれない名前は、トークン検証 claims から解決されトップレベルフィールドとして注入されます。利用可能な claims：`username`、`app`、`sub`、`iss`、`iat`、`nbf`、`exp`、`jti` および UserValidator の `custom_claims` で書き込まれたカスタム claims（例：`uuid`）。`token.path_whitelist` にないルートでのみ有効 |

#### `return_fields` でのトークン Claims

トークン検証が有効な場合、`return_fields` 内で 8 つの既知 BridgeRequest フィールド（`method`、`path`、`headers`、`cookies`、`remote_addr`、`ip`、`params`、`body`）に**含まれない**名前は、トークン検証応答の claims から自動的に解決され、転送データに**トップレベルフィールド**として注入されます。

```yaml
routes:
  - path: "/api/user"
    nats_subject: "user.get"
    return_fields: ["method", "path", "body", "uuid", "username"]
    #                                         ^^^^   ^^^^^^^^
    #                              トークン claims から取得
```

転送されるデータ：
```json
{
  "method": "GET",
  "path": "/api/user",
  "body": "...",
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "username": "admin"
}
```

これにより、ダウンストリームのマイクロサービスはトークンを自分でデコードすることなく、認証済みユーザーを識別できます。

トークン claim フィールドは以下の条件を満たす場合にのみ注入されます：
- ルートが `token.path_whitelist` にない（トークンが実際に検証される）
- トークン検証が成功
- フィールドが検証応答に存在

> **後方互換性：** レガシーの `token_fields` 設定は引き続きサポートされます。リストされたフィールドはトップレベルフィールドとして注入されます（`_token` オブジェクトにラップされなくなりました）。新規デプロイでは `return_fields` の使用を推奨します。

#### `schema_body` JSON Schema 検証

標準の JSON Schema フィールドに加え、以下の 2 つの制御キーをサポートします：

| 制御キー    | 型     | 説明                                                                  |
| ----------- | ------ | --------------------------------------------------------------------- |
| `root_type` | string | ルートノードの期待される型（例：`object`、`array`）                   |
| `strict`    | bool   | 厳密モード、`true` の場合は Schema で定義されていないフィールドを拒否 |

その他のフィールドは [JSON Schema](https://json-schema.org/) 仕様（`required`、`properties`、`type` など）に従います。

## 使用例

### シナリオ 1：基本的な JSON API ゲートウェイ

フロントエンドの JSON リクエストをユーザーマイクロサービスに転送：

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

クライアントリクエスト：

```bash
curl -X POST http://127.0.0.1:9080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"123456"}'
```

### シナリオ 2：フォーム送信の JSON 変換

従来の HTML フォーム送信を自動的に JSON に変換して転送：

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

クライアントリクエスト：

```bash
curl -X POST http://127.0.0.1:9080/api/feedback \
  -d "message=Great+service&rating=5"
```

フォーム内の `rating=5` は、Schema で定義された `type: integer` に基づいて、文字列から整数 `5` に自動変換されます。

### シナリオ 3：特定フィールドのみ転送

HTTP メソッドとパスのみをマイクロサービスに渡す（シンプルなヘルスチェック系ルートに適する）：

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

### シナリオ 4：Ping エンドポイントによる遅延測定

```bash
# タイムスタンプパラメータ付き GET リクエストを送信
curl "http://127.0.0.1:9080/ping?timestamp=$(date +%s%3N)"

# 戻り値の例: {"pong": 3, "ip": "127.0.0.1", "servertime": 1716000000042}  （単位: ミリ秒）
```

`/ping` ルートは NATS 経由で `ApiNatsBridgeTemplate` マイクロサービスに転送され、遅延を計算してクライアント IP を返します。

### シナリオ 5：NATS 通信の暗号化

機密性の高い決済サービスに個別の暗号化キーを使用：

```yaml
nats_config:
  nats_encryption_key: "DEFAULT_GLOBAL_KEY_32_CHARS_OK!!"
  nats_theme_keys:
    "payment.process": "PAYMENT_DEDICATED_KEY_32_CHARS!!"
    "public.notify": "" # この Subject は平文で送信

routes:
  - path: "/api/pay"
    nats_subject: "payment.process"
    methods: ["POST"]
    content_type: "application/json"
    timeout: 60
```

## トークン検証

ApiNatsBridge はオプションで、受信 HTTP リクエストの認証トークンを検証できます。有効にすると、設定可能な HTTP ヘッダーからトークンを抽出し、NATS ベースのトークン検証サービス（例：[NyarukoLogin UserValidator](https://github.com/kagurazakayashi/NyarukoLogin)）に送信し、有効なトークンを持つリクエストのみを通過させます。

### アーキテクチャフロー

```
┌──────────┐      HTTP        ┌───────────────────┐      NATS        ┌────────────────┐
│  HTTP    │  Authorization   │  ApiNatsBridge    │  UUID|2|token    │  UserValidator │
│  クライ  │ ────────────────>│  (Gateway/Bridge) │ ────────────────>│  (NATS マイク) │
│  アント  │ <────────────────│                   │ <────────────────│                │
└──────────┘  HTTP 200/401    └───────────────────┘  UUID|{JSON}     └────────────────┘
```

**手順説明：**

1. **クライアント → ApiNatsBridge（HTTP）**：クライアントが HTTP リクエストを送信し、指定されたヘッダー（デフォルト `Authorization`）にトークンを含めます（例：`Authorization: v2.local.FcG...`）

2. **ApiNatsBridge → UserValidator（NATS Request）**：ブリッジが UUID タグを生成し、レベル 2 形式で `UUID|2|トークン`（レベル 2：システム＋トークン claims）を設定された NATS サブジェクト（デフォルト `auth.token.verify`）に送信します

3. **UserValidator → ApiNatsBridge（NATS Reply）**：バリデーターが `UUID|{"success":bool,...}` を返します — `success:true` は有効（全標準 PASETO claims：`username`/`app`/`sub`/`iss`/`iat`/`nbf`/`exp`/`jti` を含む）、`success:false` は無効（`message` エラーメッセージを含む）

4. **ApiNatsBridge → クライアント（HTTP Response）**：
   - トークン有効：リクエストはバックエンドマイクロサービスへ転送されます
   - トークン無効：HTTP 401 を返却、`{"error":"Unauthorized: invalid token"}`
   - NATS エラー：HTTP 502 を返却、`{"error":"Bad Gateway: token verification request failed"}`
   - トークン不在：HTTP 401 を返却、`{"error":"Unauthorized: missing authentication token"}`

### NATS メッセージ形式

| 方向 | 形式 | 例 |
|------|------|-----|
| ApiNatsBridge → UserValidator | `UUID\|2\|トークン` | `550e8400-e29b-41d4-a716-446655440000\|2\|v2.local.FcG...` |
| UserValidator → ApiNatsBridge（有効） | `UUID\|{"success":true,...}` | `550e8400-...\|{"success":true,"username":"admin","app":"myapp","sub":"admin","iss":"/auth/login","iat":"...","nbf":"...","exp":"...","jti":"abc123..."}` |
| UserValidator → ApiNatsBridge（無効） | `UUID\|{"success":false,...}` | `550e8400-...\|{"success":false,"message":"token verification failed: ..."}` |

- `|` はフィールド区切り文字です
- レベル `2` は全標準 PASETO claims（`username`/`app`/`sub`/`iss`/`iat`/`nbf`/`exp`/`jti`）を返し、追加の DB クエリは不要です
- NATS レベルの完全なプロトコル詳細は [NyarukoLogin UserValidator README](https://github.com/kagurazakayashi/NyarukoLogin/blob/master/UserValidator/README.md#authtokenverify--直接-nats-介面令牌核實) を参照してください

### 設定

```yaml
bridge:
  token:
    nats_subject: "auth.token.verify"   # トークン検証用 NATS サブジェクト
    path_whitelist:                      # トークンチェックをスキップするパス
      - "/ping"
      - "/auth/login"
    header_name: "Authorization"         # トークンを含む HTTP ヘッダー名
    min_length: 10                       # トークンの最小バイト長
    max_length: 4096                     # トークンの最大バイト長
    tag_separator: "|"                   # タグと結果の区切り文字
    success_value: "0"                   # 有効なトークンの期待返却値
    timeout: 5                           # NATS 応答待ちタイムアウト秒数
    cache_max_entries: 1000              # キャッシュする検証結果の最大エントリ数
    max_concurrent: 256                  # 最大並行検証数
```

| フィールド | 型 | デフォルト | 説明 |
|------------|------|-----------|------|
| `nats_subject` | string | `auth.token.verify` | トークン検証の NATS サブジェクト |
| `path_whitelist` | []string | `[]` | トークンチェックをスキップする HTTP パス |
| `header_name` | string | `Authorization` | トークンを含む HTTP ヘッダー名 |
| `min_length` | int | `10` | トークンの最小許容バイト長 |
| `max_length` | int | `4096` | トークンの最大許容バイト長 |
| `tag_separator` | string | `\|` | 応答でタグと結果を区切る文字 |
| `success_value` | string | `0` | 有効なトークンを示す返却値 |
| `timeout` | int | `5` | NATS リクエストタイムアウト秒数 |
| `cache_max_entries` | int | `1000` | キャッシュする検証結果の最大エントリ数、上限到達時にキャッシュクリア |
| `max_concurrent` | int | `256` | 同時トークン検証 NATS リクエストの最大数、超過時に HTTP 503 を返却 |
| `paseto_secret_key` | any | - | オプションの PASETO ローカル復号キー（UserValidator と同じ形式をサポート） |

> **注意：** トークン検証は**デフォルトで無効**です。有効にするには、設定ファイルに `token` ブロックを明示的に記述する必要があります。
>
> タグは **UUID v4**（例：`550e8400-e29b-41d4-a716-446655440000`）を使用し、16進数文字とハイフンのみで構成されるため、`?` および `!` を含まないことが保証されます。
>
> UserValidator の `token_claims_mapping.custom_claims` でトークンに書き込まれたカスタム claims（例：`uuid`）は、レベル 2 検証応答に自動的に含まれ、ルートの `return_fields` に対応する名前を追加することで抽出できます（またはレガシーの `token_fields` 設定を使用）。

### トークンキャッシュ

NATS トラフィックを削減するため、検証済みトークンと最近失敗したトークンの結果はメモリにキャッシュされます：

- **キャッシュキー**：トークンの完全な文字列
- **キャッシュヒット（有効）**：NATS 呼び出しなしでリクエストを通過
- **キャッシュヒット（無効）**：即座にリクエストを拒否（HTTP 401）
- **キャッシュミス**：NATS 検証を実行し、結果をキャッシュに保存
- **キャッシュ削除**：`cache_max_entries` の上限に達すると、キャッシュ全体をクリア（コールドリスタート戦略）

### エラーコード

| HTTP ステータス | メッセージ | 原因 |
|----------------|------------|------|
| 400 | `invalid token length` | トークン長が `min_length` より短いか `max_length` より長い |
| 401 | `missing authentication token` | トークンヘッダーが存在しないか空 |
| 401 | `invalid token` | NATS 応答結果が `0` ではない（期限切れ、形式不正、失効） |
| 502 | `token verification request failed` | NATS リクエストがタイムアウトまたは接続エラー |
| 503 | `too many token verification requests` | 同時検証数が上限（max_concurrent）に到達 |

## クライアント IP 解決の優先順位

クライアントの実際の IP アドレスを解決する際の優先順位：

1. CDN 専用ヘッダー（`cdnheader` リスト内のヘッダー、リスト順に検索）
2. `X-Real-IP` ヘッダー
3. `X-Forwarded-For` ヘッダーの最初の有効な IP
4. TCP 接続のリモートアドレス（`RemoteAddr`）

すべての候補値は有効な IP アドレスかどうか検証されます。

## マイクロサービス開発ガイド

バックエンドのマイクロサービスは、対応する NATS Subject をサブスクライブし、`BridgeRequest` JSON を受信して処理後、`BridgeResponse` JSON を返すだけです。

### Go の例

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

マイクロサービス処理ロジック：

1. NATS Subject（例：`user_service`）をサブスクライブ
2. メッセージを受信したら `BridgeRequest` にパース
3. ビジネスロジックを実行
4. `BridgeResponse` JSON を構築して返す

マイクロサービスが有効な `BridgeResponse` JSON を返さない場合、ApiNatsBridge は生のレスポンス文字列を HTTP 200 のレスポンスボディとして直接クライアントに返します。

## ローカルサービス環境

プロジェクトには完全なローカルテスト環境（`test/` ディレクトリ）とテンプレートマイクロサービス（`ApiNatsBridgeTemplate/`）が含まれています：

```bash
# Windows — ワンクリック起動（NATS Server、ApiNatsBridge、ApiNatsBridgeTemplate を起動）
serve.bat

# Linux / macOS — ワンクリック起動
chmod +x serve.sh
./serve.sh
```

```bash
# すべてのサービスをワンクリック停止
serve_stop.bat   # Windows
./serve_stop.sh  # Linux / macOS
```

> **警告：** serve スクリプトはサンプル設定ファイルのデフォルトポートを使用します。既に実行中のサービスと競合します（NATS ポート 4222、HTTP ポート 9080）。既存のサービスを先に停止してください。

起動手順：

1. ローカル NATS サーバーを起動（`test/nats-server/`）
2. ApiNatsBridge メインプログラムを起動
3. ApiNatsBridgeTemplate マイクロサービスを起動（`ApiNatsBridgeTemplate/`）

起動後、`serve.bat` が最後に自動でテストリクエストを送信します。手動でも送信できます：

```bash
# ping リクエストを送信（ApiNatsBridgeTemplate が {pong, ip, servertime} を返す）
curl "http://127.0.0.1:9080/ping?timestamp=0"

# Windows PowerShell
Invoke-RestMethod -Uri ("http://127.0.0.1:9080/ping?timestamp=" + [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())
```

### ApiNatsBridgeTemplate

テンプレートマイクロサービスは `ping_req` NATS サブジェクトを購読し、`timestamp` パラメータを読み取り、`{"pong": <遅延ミリ秒>, "ip": "<クライアント IP>", "servertime": <サーバータイムスタンプミリ秒>}` を返します。

```bash
# デフォルト設定で起動
go run ./ApiNatsBridgeTemplate/ -c ApiNatsBridgeTemplate/config.yaml
```

詳細は `ApiNatsBridgeTemplate/README.md` を参照してください。

## 依存関係

| パッケージ                                                                                                | 用途                            |
| --------------------------------------------------------------------------------------------------------- | ------------------------------- |
| [github.com/google/uuid](https://github.com/google/uuid)                                                  | UUID 生成                       |
| [github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver](https://github.com/kagurazakayashi/libNyaruko_Go) | HTTP API サーバーフレームワーク |
| [github.com/kagurazakayashi/libNyaruko_Go/nyanats](https://github.com/kagurazakayashi/libNyaruko_Go)      | NATS クライアントラッパー       |
| [gopkg.in/yaml.v3](https://github.com/go-yaml/yaml)                                                       | YAML 設定解析                   |
| [github.com/santhosh-tekuri/jsonschema/v6](https://github.com/santhosh-tekuri/jsonschema)                 | JSON Schema 検証                |
| [github.com/nats-io/nats.go](https://github.com/nats-io/nats.go)                                          | NATS Go クライアント            |

## ライセンス

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
