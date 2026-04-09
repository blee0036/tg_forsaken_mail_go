TG-Forsaken-Mail
==============
A self-hosted forward mail to your telegram account via telegram bot.

Edit and Fork From [forsaken-mail](https://github.com/denghongcai/forsaken-mail)

### Installation

#### Set Up DNS Record

First, you need a domain for receive all mail which send to you `*@<example.com>` mail address. Setup a dns record on
you name server.

| Record Type | Name | IP Address            | If have any cdn proxy |
|-------------|------|-----------------------|-----------------------|
| A           | @    | <Your server IP addr> | DISABLE               |

#### Clone Project

```git clone https://github.com/blee0036/tg_forsaken_mail.git ```

#### Install NVM NodeJS

```
# Install NVM
touch ~/.profile
curl -sL https://raw.githubusercontent.com/nvm-sh/nvm/v0.37.2/install.sh -o install_nvm.sh && bash install_nvm.sh
source ~/.profile
# Install NJS
nvm install 15.10.0 && nvm use 15.10.0

# Check NodeJS version
# OutPut : v15.10.0
node -v 

# Install Package
cd tg_forsaken_mail && npm install
```

#### Edit Config

```
cp config-simple.json config.json
vim config.json
```

<b>Only edit</b> <code>mail_domain</code> to your and <code>telegram_bot_token</code>

#### Set Systemd

```
# mkdir
mkdir -p /usr/lib/systemd/system
# edit service
vim /usr/lib/systemd/system/tgbot.service
```

Edit <code>TG_MAIL_BOT_PATH</code> as your tg-forsaken-mail path

```
[Unit]
Description=TG-Forsaken-Mail
After=network.target
Wants=network.target

[Service]
WorkingDirectory=/TG_MAIL_BOT_PATH
ExecStart=/root/.nvm/versions/node/v15.10.0/bin/node /TG_MAIL_BOT_PATH/bin/bot
Restart=on-abnormal
RestartSec=5s
KillMode=mixed

StandardOutput=null
StandardError=syslog

[Install]
WantedBy=multi-user.target
```

#### Start Bot

```
# reload system daemon
systemctl daemon-reload

# start bot
systemctl start tgbot

# auto start bot when system startup
systemctl enable tgbot
```

### FAQ

Q: Why I can not get any mail with my telegram bot

A:

1. send command `/start` to your bot. Check if `telegram_bot_token` has settled correctly.

If bot can not reply, please search on google "how to setup a telegram bot" and get correct bot token.

2. if bot reply correctly, you can use `tcping` command to check if port of smtp protocol `25` is open. If is not
   responsed, check your server firewall and open the port. Some server seller also block port 25 for prevent them from
   email abuse. Just change another server seller.
