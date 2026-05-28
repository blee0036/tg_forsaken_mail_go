# TG-Forsaken-Mail (Go Version)

A self-hosted email-to-Telegram forwarding service written in Go. Receives emails via SMTP and forwards them to your Telegram account through a bot.

自托管的邮件转发服务（Go 版本），通过 SMTP 接收邮件并转发到你的 Telegram 账号。

---

## Features / 功能

- Receive emails at `*@yourdomain.com` and forward to Telegram / 接收 `*@你的域名` 的邮件并转发到 Telegram
- Bind multiple domains / 绑定多个域名
- Block senders, sender domains, or receivers / 屏蔽发件人、发件域名或收件人
- Optional HTML email upload / 可选的 HTML 邮件上传
- Admin broadcast / 管理员广播

## Requirements / 前置条件

- A domain with MX record pointing to your server / 一个 MX 记录指向你服务器的域名
- Port 25 open (SMTP) / 端口 25 开放
- A Telegram Bot token from [@BotFather](https://t.me/BotFather) / 从 @BotFather 获取 Bot Token

## DNS Setup / DNS 设置

| Type | Name | Value | TTL |
|------|------|-------|-----|
| A | `mail` | `<your server IP>` | 300 |
| MX | `@` | `mail.yourdomain.com` | 300 |

> Make sure port 25 is open and not blocked by your hosting provider.
>
> 确保端口 25 已开放，且未被主机商封禁。

## Configuration / 配置

Copy the example config and fill in your values / 复制示例配置并填入你的值：

```bash
cp config-simple.json config.json
```

```json
{
  "mailin": {
    "host": "0.0.0.0",
    "port": 25
  },
  "mail_domain": "yourdomain.com",
  "telegram_bot_token": "your-bot-token",
  "admin_tg_id": "your-telegram-id",
  "upload_url": "",
  "upload_token": ""
}
```

| Field | Description / 说明 |
|-------|-------------------|
| `mail_domain` | MX record domain / MX 记录域名 |
| `telegram_bot_token` | Bot token from @BotFather |
| `admin_tg_id` | Your Telegram user ID, enables `/send_all` / 你的 Telegram ID，启用广播功能 |
| `upload_url` | Optional: HTML upload endpoint / 可选：HTML 上传地址 |
| `upload_token` | Optional: Auth token for upload / 可选：上传认证 token |

## Running / 运行

### 1. Docker (Recommended / 推荐)

```bash
docker run -d \
  --name tg-mail-bot \
  --restart always \
  -p 25:25 \
  -v $(pwd)/config.json:/app/config.json \
  -v $(pwd)/mail.db:/app/mail.db \
  -v /etc/localtime:/etc/localtime:ro \
  ghcr.io/blee0036/tg_forsaken_main_go:latest
```

### 2. Build Docker Locally / 本地构建 Docker

```bash
cd go_version
docker build -t tg-mail-bot .
docker run -d \
  --restart always \
  -p 25:25 \
  -v $(pwd)/config.json:/app/config.json \
  -v $(pwd)/mail.db:/app/mail.db \
  -v /etc/localtime:/etc/localtime:ro \
  tg-mail-bot
```

### 3. systemd

Build the binary first / 先编译：

```bash
cd go_version
go build -o bot ./cmd/bot
```

Create a service file / 创建服务文件：

```ini
[Unit]
Description=TG-Forsaken-Mail (Go)
After=network.target

[Service]
WorkingDirectory=/path/to/tg_forsaken_mail/go_version
ExecStart=/path/to/tg_forsaken_mail/go_version/bot
Restart=on-abnormal
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now tg-mail-bot
```

## Bot Commands / Bot 命令

| Command | Description / 说明 |
|---------|-------------------|
| `/start` | Start the bot / 启动 Bot 并绑定默认域名 |
| `/help` | Show help / 显示帮助 |
| `/list` | List bound domains / 列出已绑定域名 |
| `/bind <domain>` | Bind a domain / 绑定域名 |
| `/dismiss <domain>` | Unbind a domain / 解绑域名 |
| `/unblock_domain <domain>` | Unblock a sender domain / 解封发件域名 |
| `/unblock_sender <email>` | Unblock a sender / 解封发件人 |
| `/unblock_receiver <email>` | Unblock a receiver / 解封收件人 |
| `/list_block_domain` | List blocked domains / 列出屏蔽的域名 |
| `/list_block_sender` | List blocked senders / 列出屏蔽的发件人 |
| `/list_block_receiver` | List blocked receivers / 列出屏蔽的收件人 |

## Bot Migration / Bot 迁移功能

When you need to switch users from an old Bot to a new Bot, the system supports a smooth migration process: old Bots continue to receive interactions and deliver emails during the transition, while sending migration alerts to guide users to the new Bot.

当你需要将用户从旧 Bot 迁移到新 Bot 时，系统支持平滑迁移：旧 Bot 在过渡期内继续接收交互和投递邮件，同时发送迁移提醒引导用户切换到新 Bot。

### Migration Config Fields / 迁移配置字段

Copy the example config to get started / 复制示例配置快速开始：

```bash
cp config-changebot-simple.json config.json
```

```json
{
  "telegram_bot_token": "YOUR_NEW_BOT_TOKEN",
  "old_telegram_bot_token": [
    "YOUR_OLD_BOT_TOKEN_1",
    "YOUR_OLD_BOT_TOKEN_2"
  ],
  "change_bot_alert_msg": {
    "en": "⚠️ This bot will be discontinued soon. Please switch to @YourNewBot for uninterrupted mail forwarding.",
    "zh": "⚠️ 此 Bot 即将停用，请尽快切换到 @YourNewBot 以继续接收邮件转发服务。"
  },
  "close_old_date": "2025-12-31"
}
```

| Field | Required | Description / 说明 |
|-------|----------|-------------------|
| `old_telegram_bot_token` | No | Array of old Bot tokens to migrate from. Empty strings, duplicates, and tokens matching the main `telegram_bot_token` are automatically filtered. / 旧 Bot Token 数组。空字符串、重复项和与主 Token 相同的条目会被自动过滤。 |
| `change_bot_alert_msg` | No | Map of language code → migration alert message (supports Markdown). At least `"en"` and `"zh"` recommended. Entries with blank keys or values are ignored. / 语言代码→迁移提醒消息的映射（支持 Markdown）。建议至少配置 `"en"` 和 `"zh"`。空白 key 或 value 的条目会被忽略。 |
| `close_old_date` | No | Date (`YYYY-MM-DD`) after which old Bots stop delivering emails and daily alerts. Interactions still trigger alerts. Invalid format causes startup failure. / 旧 Bot 停止投递邮件和每日推送的截止日期。到期后交互仍会触发提醒。格式不合法会导致启动失败。 |

### How It Works / 使用流程

1. **Create a new Bot** via [@BotFather](https://t.me/BotFather) and set its token as `telegram_bot_token` in config.json.

   通过 @BotFather 创建新 Bot，将其 Token 设为 config.json 中的 `telegram_bot_token`。

2. **Move old Bot token(s)** to the `old_telegram_bot_token` array.

   将旧 Bot Token 移入 `old_telegram_bot_token` 数组。

3. **Configure alert messages** in `change_bot_alert_msg` with your preferred languages. Messages support Markdown formatting. Language fallback order: exact match → prefix normalization (e.g. `zh-Hans` → `zh`) → `"en"` → `"zh"` → first key alphabetically.

   在 `change_bot_alert_msg` 中配置各语言的迁移提醒消息，支持 Markdown 格式。语言回退顺序：精确匹配 → 前缀归一化（如 `zh-Hans` → `zh`）→ `"en"` → `"zh"` → 按 key 字典序第一个。

4. **Optionally set `close_old_date`** to a future date. After this date, old Bots will stop delivering emails and daily push alerts, but will still send migration alerts when users interact with them.

   可选设置 `close_old_date` 为未来日期。到期后旧 Bot 停止投递邮件和每日推送，但用户与旧 Bot 交互时仍会收到迁移提醒。

5. **Restart the service**. The system will start listening on all old Bots and the new Bot simultaneously.

   重启服务。系统将同时监听所有旧 Bot 和新 Bot。

### Migration Behavior / 迁移行为

- **Email delivery / 邮件投递**: New emails are delivered through ALL Bots (new + old). Old Bot deliveries are followed by a migration alert. / 新邮件通过所有 Bot（新+旧）投递，旧 Bot 投递后附加迁移提醒。
- **User interaction / 用户交互**: When a user interacts with an old Bot, the interaction is handled normally, then a migration alert is sent. / 用户与旧 Bot 交互时，正常处理后发送迁移提醒。
- **Daily push / 每日推送**: At 00:00 server local time, all unique users receive a migration alert via old Bots (not the new Bot). / 每日 0:00（服务器本地时间），所有唯一用户通过旧 Bot 收到迁移提醒。
- **Close date / 截止日期**: After `close_old_date`, old Bots stop email delivery and daily push, but interaction alerts continue. / 到达 `close_old_date` 后，旧 Bot 停止邮件投递和每日推送，但交互提醒继续。

### Notes / 注意事项

- All migration fields are **optional**. Without them, the system runs exactly as before. / 所有迁移字段均为**可选**，不配置时系统行为与之前完全一致。
- All Bots (new and old) share the same database and user bindings. / 所有 Bot（新旧）共享同一数据库和用户绑定关系。
- If an old Bot token is invalid, it is skipped with a log warning; other Bots start normally. / 旧 Bot Token 无效时跳过并记录日志，不影响其他 Bot 启动。
- Alert send failures are handled gracefully (Markdown → plain text retry → log and continue). / Alert 发送失败时优雅处理（Markdown → 纯文本重试 → 记录日志并继续）。
- See `config-changebot-simple.json` for a complete example configuration. / 完整配置示例请参考 `config-changebot-simple.json`。

---

## Advanced: HTML Email Viewer via Cloudflare Worker / 进阶：通过 Cloudflare Worker 查看 HTML 邮件

When an email contains HTML content, the bot can upload it to a Cloudflare Worker so you can view the full rendered email in a browser. This requires configuring `upload_url` and `upload_token` in `config.json`, and deploying a Cloudflare Worker with a D1 database.

当邮件包含 HTML 内容时，Bot 可以将其上传到 Cloudflare Worker，让你在浏览器中查看完整渲染的邮件。需要在 `config.json` 中配置 `upload_url` 和 `upload_token`，并部署一个绑定了 D1 数据库的 Cloudflare Worker。

### Step 1: Create D1 Database / 创建 D1 数据库

1. Go to [Cloudflare Dashboard](https://dash.cloudflare.com/) → **Workers & Pages** → **D1 SQL Database**
2. Click **Create database**, name it (e.g. `mail-html-db`)
3. In the database console, run this SQL to create the table / 在数据库控制台执行以下 SQL 建表：

```sql
CREATE TABLE mail_data (
  id TEXT PRIMARY KEY,
  data TEXT NOT NULL,
  createTime INTEGER NOT NULL
);
```

进入 [Cloudflare 控制台](https://dash.cloudflare.com/) → **Workers & Pages** → **D1 SQL Database**，创建数据库后在控制台执行上述 SQL。

### Step 2: Create Worker / 创建 Worker

1. Go to **Workers & Pages** → **Create** → **Create Worker**
2. Name it (e.g. `mail-html-worker`), click **Deploy**
3. Click **Edit code**, replace all content with the code from `cloudflare_worker_script/mail-html.js`
4. Click **Deploy**

进入 **Workers & Pages** → **创建** → **创建 Worker**，命名后点击部署，然后点击编辑代码，将 `cloudflare_worker_script/mail-html.js` 的内容粘贴进去，再次部署。

### Step 3: Bind D1 Database to Worker / 将 D1 数据库绑定到 Worker

1. Go to your Worker → **Settings** → **Bindings**
2. Click **Add** → **D1 Database**
3. Variable name: `DB` (must be exactly `DB`)
4. Select the database you created in Step 1
5. Click **Deploy** to apply

进入 Worker → **设置** → **绑定**，添加 D1 数据库绑定，变量名必须填 `DB`，选择第一步创建的数据库，然后部署。

### Step 4: Set Worker Token / 设置 Worker Token

1. Go to your Worker → **Settings** → **Variables and Secrets**
2. Click **Add** → Type: **Secret**
3. Variable name: `TOKEN`, value: a random string you choose as your auth token
4. Click **Deploy**

进入 Worker → **设置** → **变量和机密**，添加一个 Secret，变量名 `TOKEN`，值为你自定义的认证密钥。

### Step 5: Update config.json / 更新配置

```json
{
  "upload_url": "https://mail-html-worker.your-subdomain.workers.dev",
  "upload_token": "your-token-from-step-4"
}
```

Now when the bot receives an HTML email, it will upload the content and include a "View in browser" link in the Telegram message.

配置完成后，Bot 收到 HTML 邮件时会自动上传内容，并在 Telegram 消息中附带"在浏览器中查看"链接。

### Auto Cleanup / 自动清理

The Worker includes a scheduled handler that deletes emails older than 7 days. To enable it:

Worker 内置了定时清理功能，自动删除 7 天前的邮件。启用方法：

1. Go to your Worker → **Settings** → **Triggers** → **Cron Triggers**
2. Add a cron expression, e.g. `0 0 * * *` (daily at midnight UTC)

进入 Worker → **设置** → **触发器** → **Cron 触发器**，添加 cron 表达式如 `0 0 * * *`（每天 UTC 0 点执行）。

---

## Development / 开发

```bash
cd go_version
go test ./...
```

Uses [gopter](https://github.com/leanovate/gopter) for property-based testing.

使用 gopter 进行属性测试。

## Project Structure / 项目结构

```
go_version/
├── cmd/bot/          # Entry point / 入口
├── internal/
│   ├── config/       # Config loading / 配置加载
│   ├── db/           # SQLite database / 数据库层
│   ├── io/           # Business logic / 业务逻辑
│   ├── migration/    # Bot migration manager / Bot 迁移管理
│   ├── smtp/         # SMTP server / SMTP 服务
│   ├── telegram/     # Telegram bot
│   ├── upload/       # HTML upload client / HTML 上传
│   └── lrucache/     # LRU cache / LRU 缓存
├── config-simple.json
├── config-changebot-simple.json  # Migration config example / 迁移配置示例
└── Dockerfile
```
