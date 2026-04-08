# TG-Forsaken-Mail (Go Version)

A self-hosted email-to-Telegram forwarding service written in Go. Receives emails via SMTP and forwards them to your Telegram account through a bot.

Rewritten from the original Node.js version with improved performance, type safety, and property-based testing.

## Features

- Receive emails at `*@yourdomain.com` and forward to Telegram
- Bind multiple domains to your Telegram account
- Block senders, sender domains, or receivers
- Optional HTML email upload (for rich content)
- Admin broadcast to all users

## Requirements

- Go 1.21+
- A domain with MX record pointing to your server
- Port 25 open (SMTP)
- A Telegram Bot token (from [@BotFather](https://t.me/BotFather))

## Configuration

Copy the example config and fill in your values:

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

| Field | Description |
|-------|-------------|
| `mail_domain` | The domain used for MX record (e.g. `mail.yourdomain.com`) |
| `telegram_bot_token` | Bot token from @BotFather |
| `admin_tg_id` | Your Telegram user ID — enables `/send_all` broadcast |
| `upload_url` | Optional: URL to upload HTML email content |
| `upload_token` | Optional: Auth token for the upload endpoint |

## DNS Setup

Add an MX record pointing to your server's IP:

| Type | Name | Value | TTL |
|------|------|-------|-----|
| A | `mail` | `<your server IP>` | 300 |
| MX | `@` | `mail.yourdomain.com` | 300 |

> Make sure port 25 is open and not blocked by your hosting provider.

## Running

### Binary

```bash
cd go_version
go build -o bot ./cmd/bot
./bot
```

### Docker

```bash
cd go_version
docker build -t tg-mail-bot .
docker run -d \
  -p 25:25 \
  -v $(pwd)/config.json:/app/config.json \
  -v $(pwd)/mail.db:/app/mail.db \
  tg-mail-bot
```

### systemd

```ini
[Unit]
Description=TG-Forsaken-Mail (Go)
After=network.target

[Service]
WorkingDirectory=/path/to/go_version
ExecStart=/path/to/go_version/bot
Restart=on-abnormal
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now tg-mail-bot
```

## Bot Commands

| Command | Description |
|---------|-------------|
| `/start` | Start the bot and bind a default domain |
| `/help` | Show help |
| `/list` | List your bound domains |
| `/bind <domain>` | Bind a domain |
| `/dismiss <domain>` | Unbind a domain |
| `/unblock_domain <domain>` | Unblock a sender domain |
| `/unblock_sender <email>` | Unblock a sender |
| `/unblock_receiver <email>` | Unblock a receiver |
| `/list_block_domain` | List blocked sender domains |
| `/list_block_sender` | List blocked senders |
| `/list_block_receiver` | List blocked receivers |

## Development

```bash
cd go_version

# Run all tests
go test ./...

# Run with verbose output
go test -v ./...
```

The project uses [gopter](https://github.com/leanovate/gopter) for property-based testing alongside standard unit tests.

## Project Structure

```
go_version/
├── cmd/bot/          # Entry point
├── internal/
│   ├── config/       # Config loading
│   ├── db/           # SQLite database layer
│   ├── io/           # Business logic (mail handling, domain/block management)
│   ├── smtp/         # SMTP server
│   ├── telegram/     # Telegram bot
│   ├── upload/       # HTML upload client
│   └── lrucache/     # LRU cache
├── config-simple.json
└── Dockerfile
```
