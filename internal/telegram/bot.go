package telegram

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/io"
)

// DomainRegex matches valid domain names, equivalent to Node version:
// /((?=[a-z0-9-]{1,63}\.)(xn--)?[a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,63}/i
// Note: Go's RE2 engine does not support lookaheads (?=...). The lookahead
// (?=[a-z0-9-]{1,63}\.) constrains each label to 1-63 chars. We drop it here
// since the base pattern already matches valid domains correctly for practical use.
var DomainRegex = regexp.MustCompile(`(?i)((xn--)?[a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,63}`)

// EmailRegex matches email addresses, equivalent to Node version:
// /[\w\._\-\+]+@[\w\._\-\+]+/i
var EmailRegex = regexp.MustCompile(`(?i)[\w._\-+]+@[\w._\-+]+`)

// BotSender abstracts the Telegram Bot sending capability for testability.
type BotSender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

// LangTexts stores the English and Chinese versions of a text string.
type LangTexts struct {
	EN string
	ZH string
}

// ParsedCommand represents a parsed user command.
type ParsedCommand struct {
	Command string   // command name (e.g. "bind")
	Args    []string // argument list (trimmed)
	Raw     string   // original text
}

// CallbackData represents parsed callback data from an inline keyboard button.
type CallbackData struct {
	Action string   // action type (e.g. "dismiss_confirm", "block_sender")
	Params []string // parameter list
}

// Bot is the Telegram Bot command handler, corresponding to Node version modules/telegram.js.
type Bot struct {
	bot    *tgbotapi.BotAPI // used for Start() and GetBotAPI(); nil in tests
	sender BotSender        // used for all Send/Request calls
	io     *io.IO
	config *config.Config
	texts  map[string]LangTexts
}

// defaultTexts contains all user-visible text in English and Chinese.
var defaultTexts = map[string]LangTexts{
	"welcome_new": {
		EN: "👋 Welcome to Mail Bot!\nThis bot forwards emails to your Telegram.\nClick the button below to get started.",
		ZH: "👋 欢迎使用邮件转发 Bot！\n本 Bot 可将邮件转发到你的 Telegram。\n点击下方按钮快速开始。",
	},
	"welcome_back": {
		EN: "Welcome back! Choose an option:",
		ZH: "欢迎回来！请选择操作：",
	},
	"help_title": {
		EN: "📖 Help - Choose a category:",
		ZH: "📖 帮助 - 请选择分类：",
	},
	"help_cat_domain": {
		EN: "📬 Domain Management",
		ZH: "📬 域名管理",
	},
	"help_cat_block": {
		EN: "🚫 Block Management",
		ZH: "🚫 屏蔽管理",
	},
	"help_cat_other": {
		EN: "⚙️ Other",
		ZH: "⚙️ 其他",
	},
	"help_detail_domain": {
		EN: "<b>Domain Management</b>\n\n" +
			"<b>/bind</b> <code>example.com</code>\nBind a domain to receive emails.\n\n" +
			"<b>/dismiss</b> <code>example.com</code>\nUnbind a domain.\n\n" +
			"<b>/list</b>\nList all your bound domains.",
		ZH: "<b>域名管理</b>\n\n" +
			"<b>/bind</b> <code>example.com</code>\n绑定域名以接收邮件。\n\n" +
			"<b>/dismiss</b> <code>example.com</code>\n解绑域名。\n\n" +
			"<b>/list</b>\n列出所有已绑定的域名。",
	},
	"help_detail_block": {
		EN: "<b>Block Management</b>\n\n" +
			"<b>/unblock_domain</b> <code>example.com</code>\nUnblock a sender domain.\n\n" +
			"<b>/unblock_sender</b> <code>someone@example.com</code>\nUnblock a sender.\n\n" +
			"<b>/unblock_receiver</b> <code>someone@example.com</code>\nUnblock a receiver.",
		ZH: "<b>屏蔽管理</b>\n\n" +
			"<b>/unblock_domain</b> <code>example.com</code>\n解封发件者域名。\n\n" +
			"<b>/unblock_sender</b> <code>someone@example.com</code>\n解封发件者邮箱。\n\n" +
			"<b>/unblock_receiver</b> <code>someone@example.com</code>\n解封收件人邮箱。",
	},
	"help_detail_other": {
		EN: "<b>Other</b>\n\n" +
			"<b>/send_all</b> <code>message</code>\nBroadcast a message to all users (admin only).",
		ZH: "<b>其他</b>\n\n" +
			"<b>/send_all</b> <code>消息内容</code>\n向所有用户广播消息（仅管理员）。",
	},
	"btn_quick_start": {
		EN: "🚀 Quick Start",
		ZH: "🚀 快速开始",
	},
	"btn_my_domains": {
		EN: "📬 My Domains",
		ZH: "📬 我的域名",
	},
	"btn_block_mgmt": {
		EN: "🚫 Block Management",
		ZH: "🚫 屏蔽管理",
	},
	"btn_help": {
		EN: "❓ Help",
		ZH: "❓ 帮助",
	},
	"btn_dismiss": {
		EN: "🗑 Unbind",
		ZH: "🗑 解绑",
	},
	"btn_confirm": {
		EN: "✅ Confirm",
		ZH: "✅ 确认",
	},
	"btn_cancel": {
		EN: "❌ Cancel",
		ZH: "❌ 取消",
	},
	"btn_back": {
		EN: "⬅️ Back",
		ZH: "⬅️ 返回",
	},
	"btn_unblock": {
		EN: "🔓 Unblock",
		ZH: "🔓 解除屏蔽",
	},
	"btn_view_help": {
		EN: "📖 View Help",
		ZH: "📖 查看帮助",
	},
	"btn_block_sender": {
		EN: "Block Sender",
		ZH: "屏蔽发件人",
	},
	"btn_block_domain": {
		EN: "Block Domain",
		ZH: "屏蔽域名",
	},
	"btn_block_receiver": {
		EN: "Block Receiver",
		ZH: "屏蔽收件人",
	},
	"btn_blocked_domains": {
		EN: "🌐 Blocked Domains",
		ZH: "🌐 屏蔽的域名",
	},
	"btn_blocked_senders": {
		EN: "👤 Blocked Senders",
		ZH: "👤 屏蔽的发件人",
	},
	"btn_blocked_receivers": {
		EN: "📧 Blocked Receivers",
		ZH: "📧 屏蔽的收件人",
	},
	"err_unknown_cmd": {
		EN: "Unknown command. Tap the button below for help.",
		ZH: "未识别的命令。点击下方按钮查看帮助。",
	},
	"err_invalid_domain": {
		EN: "Invalid domain format. Example: <code>example.com</code>",
		ZH: "域名格式无效。示例：<code>example.com</code>",
	},
	"err_invalid_email": {
		EN: "Invalid email format. Example: <code>someone@example.com</code>",
		ZH: "邮箱格式无效。示例：<code>someone@example.com</code>",
	},
	"err_bind_default_fail": {
		EN: "Failed to bind a default domain. Please try /bind <domain> manually.",
		ZH: "自动绑定默认域名失败，请手动使用 /bind <域名> 绑定。",
	},
	"usage_bind": {
		EN: "Usage: <code>/bind example.com</code>",
		ZH: "用法：<code>/bind example.com</code>",
	},
	"usage_dismiss": {
		EN: "Usage: <code>/dismiss example.com</code>",
		ZH: "用法：<code>/dismiss example.com</code>",
	},
	"usage_unblock_domain": {
		EN: "Usage: <code>/unblock_domain example.com</code>",
		ZH: "用法：<code>/unblock_domain example.com</code>",
	},
	"usage_unblock_sender": {
		EN: "Usage: <code>/unblock_sender someone@example.com</code>",
		ZH: "用法：<code>/unblock_sender someone@example.com</code>",
	},
	"usage_unblock_receiver": {
		EN: "Usage: <code>/unblock_receiver someone@example.com</code>",
		ZH: "用法：<code>/unblock_receiver someone@example.com</code>",
	},
	"msg_bind_success": {
		EN: "✅ Domain <code>%s</code> bound successfully!\n\nPlease add an MX record pointing to <code>%s</code>\n\nExample: <code>someone@%s</code>",
		ZH: "✅ 域名 <code>%s</code> 绑定成功！\n\n请添加 MX 记录指向 <code>%s</code>\n\n示例：<code>someone@%s</code>",
	},
	"msg_dismiss_success": {
		EN: "✅ Domain unbound successfully.",
		ZH: "✅ 域名已成功解绑。",
	},
	"msg_dismiss_cancel": {
		EN: "Operation cancelled.",
		ZH: "操作已取消。",
	},
	"msg_dismiss_confirm_prompt": {
		EN: "Are you sure you want to unbind <code>%s</code>?",
		ZH: "确定要解绑 <code>%s</code> 吗？",
	},
	"msg_block_success": {
		EN: "✅ Blocked %s successfully.",
		ZH: "✅ 已成功屏蔽 %s。",
	},
	"msg_unblock_success": {
		EN: "✅ Unblocked %s successfully.",
		ZH: "✅ 已成功解除屏蔽 %s。",
	},
	"msg_no_blocks": {
		EN: "No blocked items.",
		ZH: "暂无屏蔽项。",
	},
	"msg_broadcast_done": {
		EN: "✅ Broadcast complete.",
		ZH: "✅ 广播已发送。",
	},
	"msg_domain_already_yours": {
		EN: "This domain is already bound to your account.",
		ZH: "该域名已绑定到你的账户。",
	},
	"msg_domain_already_other": {
		EN: "This domain is already bound to another account.",
		ZH: "该域名已被其他账户绑定。",
	},
	"msg_domain_not_yours": {
		EN: "This domain is not bound to your account.",
		ZH: "该域名未绑定到你的账户。",
	},
	"msg_no_domains": {
		EN: "You have no bound domains.\nUse /bind <code>example.com</code> to bind one.",
		ZH: "你还没有绑定任何域名。\n使用 /bind <code>example.com</code> 来绑定。",
	},
	"msg_list_title": {
		EN: "<b>Your domains:</b>",
		ZH: "<b>你的域名：</b>",
	},
	"btn_main_menu": {
		EN: "🏠 Main Menu",
		ZH: "🏠 主菜单",
	},
	"btn_lang": {
		EN: "🌐 Language",
		ZH: "🌐 语言",
	},
	"msg_lang_select": {
		EN: "Choose your language / 选择语言：",
		ZH: "Choose your language / 选择语言：",
	},
	"msg_lang_set": {
		EN: "✅ Language set to English.",
		ZH: "✅ 语言已设置为中文。",
	},
	"btn_prev": {
		EN: "⬅️ Prev",
		ZH: "⬅️ 上一页",
	},
	"btn_next": {
		EN: "➡️ Next",
		ZH: "➡️ 下一页",
	},
}

// GenerateHelpMessages generates the English and Chinese help messages with the given mail domain.
// The output is character-level identical to the Node version telegram.js helpMsg and helpMsgCN.
func GenerateHelpMessages(mailDomain string) (helpMsg, helpMsgCN string) {
	helpMsg = "<b>list_domain :</b> \n" +
		"\n <code>/list</code> \n\n" +
		"list all your binded domain. \n\n" +
		"<b>bind_domain :</b> \n" +
		"\n <code>/bind example.com</code>\n\n" +
		"1. prepare one domain to catch all mails. \n" +
		"2. add the MX record to [<code>" + mailDomain + "</code>] \n" +
		"3. use this command to bind your domain. \n\n" +
		"<b>dismiss_domain :</b> \n " +
		"\n<code>/dismiss example.com</code> \n\n" +
		"dismiss the domain you have binded \n \n" +
		"<b>unblock_domain :</b> \n " +
		"\n<code>/unblock_domain example.com</code> \n\n" +
		"unblock the domain of sender \n \n" +
		"<b>unblock_sender :</b> \n " +
		"\n<code>/unblock_sender someone@example.com</code> \n\n" +
		"unblock sender \n \n" +
		"<b>unblock_receiver :</b> \n " +
		"\n<code>/unblock_receiver someone@example.com</code> \n\n" +
		"unblock receiver \n \n" +
		"<b>list_block_domain :</b> \n " +
		"\n<code>/list_block_domain</code> \n\n" +
		"list of blocked sender domain \n \n" +
		"<b>list_blocked_sender :</b> \n " +
		"\n<code>/list_block_sender</code> \n\n" +
		"list of blocked sender \n \n" +
		"<b>list_blocked_receiver :</b> \n " +
		"\n<code>/list_block_receiver</code> \n\n" +
		"list of blocked receiver"

	helpMsgCN = "<b>显示所有绑定域名 :</b> \n" +
		"\n <code>/list</code> \n\n" +
		"<b>绑定域名 :</b> \n" +
		"\n <code>/bind example.com</code>\n\n" +
		"1. 准备一个用于接收所有邮件的域名. \n" +
		"2. 添加MX记录解析到 [<code>" + mailDomain + "</code>] \n" +
		"3. 使用此命令绑定你的域名到你的TG账号. \n\n" +
		"<b>释放域名 :</b> \n " +
		"\n<code>/dismiss example.com</code> \n\n" +
		"释放你绑定过的域名 \n \n" +
		"<b>解封发件者域名 :</b> \n " +
		"\n<code>/unblock_domain example.com</code> \n\n" +
		"<b>解封发件者邮箱 :</b> \n " +
		"\n<code>/unblock_sender someone@example.com</code> \n\n" +
		"<b>解封收件者邮箱 :</b> \n " +
		"\n<code>/unblock_receiver someone@example.com</code> \n\n" +
		"<b>显示你所有屏蔽的发件者域名 :</b> \n " +
		"\n<code>/list_block_domain</code> \n\n" +
		"<b>显示你所有屏蔽的发件者邮箱 :</b> \n " +
		"\n<code>/list_block_sender</code> \n\n" +
		"<b>显示你所有屏蔽的收件人邮箱 :</b> \n " +
		"\n<code>/list_block_receiver</code>"

	return helpMsg, helpMsgCN
}

// New creates a new Bot instance. Returns an error if the token is empty or invalid.
func New(cfg *config.Config, ioModule *io.IO) (*Bot, error) {
	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("telegram bot token is empty")
	}

	botAPI, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	// Copy defaultTexts into the instance
	texts := make(map[string]LangTexts, len(defaultTexts))
	for k, v := range defaultTexts {
		texts[k] = v
	}

	return &Bot{
		bot:    botAPI,
		sender: botAPI, // *tgbotapi.BotAPI implements BotSender
		io:     ioModule,
		config: cfg,
		texts:  texts,
	}, nil
}

// NewForTest creates a Bot instance for testing with a mock BotSender.
// The underlying bot field is nil, so Start() and GetBotAPI() should not be called in tests.
func NewForTest(cfg *config.Config, ioModule *io.IO, sender BotSender) *Bot {
	texts := make(map[string]LangTexts, len(defaultTexts))
	for k, v := range defaultTexts {
		texts[k] = v
	}

	return &Bot{
		sender: sender,
		io:     ioModule,
		config: cfg,
		texts:  texts,
	}
}

// GetBotAPI returns the underlying tgbotapi.BotAPI instance.
func (b *Bot) GetBotAPI() *tgbotapi.BotAPI {
	return b.bot
}

// GetSender returns the BotSender interface used for sending messages.
func (b *Bot) GetSender() BotSender {
	return b.sender
}

// getText returns the text for the given key and language.
// Falls back to EN if the language is not "zh", or if the key is not found.
func (b *Bot) getText(key string, lang string) string {
	lt, ok := b.texts[key]
	if !ok {
		return ""
	}
	if lang == "zh" {
		return lt.ZH
	}
	return lt.EN
}

// registerCommands registers all bot commands via the SetMyCommands API.
// Returns an error if the API call fails.
func (b *Bot) registerCommands() error {
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Start the bot"},
		tgbotapi.BotCommand{Command: "help", Description: "Show help information"},
		tgbotapi.BotCommand{Command: "list", Description: "List your domains"},
		tgbotapi.BotCommand{Command: "bind", Description: "Bind a domain"},
		tgbotapi.BotCommand{Command: "dismiss", Description: "Unbind a domain"},
		tgbotapi.BotCommand{Command: "unblock_domain", Description: "Unblock a sender domain"},
		tgbotapi.BotCommand{Command: "unblock_sender", Description: "Unblock a sender"},
		tgbotapi.BotCommand{Command: "unblock_receiver", Description: "Unblock a receiver"},
		tgbotapi.BotCommand{Command: "list_block_domain", Description: "List blocked domains"},
		tgbotapi.BotCommand{Command: "list_block_sender", Description: "List blocked senders"},
		tgbotapi.BotCommand{Command: "list_block_receiver", Description: "List blocked receivers"},
		tgbotapi.BotCommand{Command: "lang", Description: "Set language / 设置语言"},
	)
	_, err := b.sender.Request(commands)
	if err != nil {
		return fmt.Errorf("failed to register commands: %w", err)
	}
	return nil
}

// Start begins listening for Telegram messages and callback queries.
// This method blocks indefinitely.
func (b *Bot) Start() {
	if err := b.registerCommands(); err != nil {
		log.Printf("warning: %v", err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	for {
		updates := b.bot.GetUpdatesChan(u)

		for update := range updates {
			if update.CallbackQuery != nil {
				b.handleCallbackQuery(update.CallbackQuery)
				continue
			}

			if update.Message != nil {
				b.handleMessage(update.Message)
			}
		}

		log.Println("Telegram update channel closed, reconnecting in 5s...")
		time.Sleep(5 * time.Second)
	}
}

// handleCallbackQuery processes callback queries from inline keyboard buttons.
// Uses decodeCallback to parse callback data and routes to specific handlers.
func (b *Bot) handleCallbackQuery(callbackQuery *tgbotapi.CallbackQuery) {
	cb := b.decodeCallback(callbackQuery.Data)
	lang := b.detectLanguage(callbackQuery.From)

	switch cb.Action {
	case "quick_start":
		b.handleQuickStart(callbackQuery, lang)
	case "main_menu":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleMainMenu(callbackQuery, lang, param)
	case "help_cat":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleHelpCategory(callbackQuery, lang, param)
	case "help_back":
		b.handleHelpBack(callbackQuery, lang)
	case "dismiss_ask":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleDismissAsk(callbackQuery, lang, param)
	case "dismiss_yes":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleDismissConfirm(callbackQuery, lang, param)
	case "dismiss_no":
		b.handleDismissCancel(callbackQuery, lang)
	case "block_sender":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleBlockAction(callbackQuery, lang, "sender", param)
	case "block_domain":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleBlockAction(callbackQuery, lang, "domain", param)
	case "block_receiver":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleBlockAction(callbackQuery, lang, "receiver", param)
	case "unblock_sender":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleUnblockAction(callbackQuery, lang, "sender", param)
	case "unblock_domain":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleUnblockAction(callbackQuery, lang, "domain", param)
	case "unblock_receiver":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleUnblockAction(callbackQuery, lang, "receiver", param)
	case "block_cat":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleBlockCategory(callbackQuery, lang, param)
	case "set_lang":
		param := ""
		if len(cb.Params) > 0 {
			param = cb.Params[0]
		}
		b.handleSetLang(callbackQuery, param)
	case "go_main":
		b.handleGoMain(callbackQuery, lang)
	case "list_page":
		page := 0
		if len(cb.Params) > 0 {
			fmt.Sscanf(cb.Params[0], "%d", &page)
		}
		b.handleListPage(callbackQuery, lang, page)
	case "block_page":
		cat := ""
		page := 0
		if len(cb.Params) > 0 {
			cat = cb.Params[0]
		}
		if len(cb.Params) > 1 {
			fmt.Sscanf(cb.Params[1], "%d", &page)
		}
		b.handleBlockCategoryPage(callbackQuery, lang, cat, page)
	case "noop":
		b.answerCallbackQuery(callbackQuery.ID, "", false)
	default:
		b.answerCallbackQuery(callbackQuery.ID, b.getText("err_unknown_cmd", lang), true)
	}
}

// handleMessage processes incoming text messages and routes commands.
// Uses parseCommand for parsing and detectLanguage for i18n.
func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	if msg.Text == "" {
		return
	}

	cmd := parseCommand(msg.Text)
	lang := b.detectLanguage(msg.From)

	switch cmd.Command {
	case "start":
		b.handleStart(msg, lang)
	case "help":
		b.handleHelp(msg, lang)
	case "list":
		b.handleList(msg, lang)
	case "bind":
		b.handleBind(msg, lang, cmd.Args)
	case "dismiss":
		b.handleDismiss(msg, lang, cmd.Args)
	case "unblock_domain":
		b.handleUnblock(msg, lang, "domain", cmd.Args)
	case "unblock_sender":
		b.handleUnblock(msg, lang, "sender", cmd.Args)
	case "unblock_receiver":
		b.handleUnblock(msg, lang, "receiver", cmd.Args)
	case "list_block_domain":
		b.handleListBlock(msg, lang, "domain")
	case "list_block_sender":
		b.handleListBlock(msg, lang, "sender")
	case "list_block_receiver":
		b.handleListBlock(msg, lang, "receiver")
	case "send_all":
		b.handleSendAll(msg, cmd.Args)
	case "lang":
		b.handleLang(msg)
	default:
		// Unknown command: send friendly prompt with "View Help" button
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					b.getText("btn_view_help", lang),
					b.encodeCallback("help"),
				),
			),
		)
		b.sendHTMLWithKeyboard(msg.Chat.ID, b.getText("err_unknown_cmd", lang), keyboard)
	}
}

// sendMessage sends a plain text message via bot.
func (b *Bot) sendMessage(chatID int64, text string) {
	if b.sender == nil {
		return
	}
	msgCfg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.sender.Send(msgCfg); err != nil {
		log.Printf("failed to send message: %v", err)
	}
}

// sendHTMLWithKeyboard sends an HTML-formatted message with an inline keyboard via bot.
func (b *Bot) sendHTMLWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	if b.sender == nil {
		return
	}
	msgCfg := tgbotapi.NewMessage(chatID, text)
	msgCfg.ParseMode = "HTML"
	msgCfg.ReplyMarkup = keyboard
	if _, err := b.sender.Send(msgCfg); err != nil {
		log.Printf("failed to send HTML message with keyboard: %v", err)
	}
}

// handleStart handles the /start command.
// New users (no domains) get a welcome message with a "Quick Start" button.
// Existing users get a welcome back message with the main menu.
func (b *Bot) handleStart(msg *tgbotapi.Message, lang string) {
	if b.io.GetAllDomainCount(msg.Chat.ID) == 0 {
		// New user: welcome + quick start button
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					b.getText("btn_quick_start", lang),
					b.encodeCallback("quick_start"),
				),
			),
		)
		b.sendHTMLWithKeyboard(msg.Chat.ID, b.getText("welcome_new", lang), keyboard)
	} else {
		// Existing user: welcome back + main menu
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					b.getText("btn_my_domains", lang),
					b.encodeCallback("main_menu", "domains"),
				),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					b.getText("btn_block_mgmt", lang),
					b.encodeCallback("main_menu", "blocks"),
				),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					b.getText("btn_help", lang),
					b.encodeCallback("main_menu", "help"),
				),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					b.getText("btn_lang", lang),
					b.encodeCallback("main_menu", "lang"),
				),
			),
		)
		b.sendHTMLWithKeyboard(msg.Chat.ID, b.getText("welcome_back", lang), keyboard)
	}
}

// handleHelp handles the /help command.
// Sends help title with category inline keyboard buttons.
func (b *Bot) handleHelp(msg *tgbotapi.Message, lang string) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("help_cat_domain", lang),
				b.encodeCallback("help_cat", "domain"),
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("help_cat_block", lang),
				b.encodeCallback("help_cat", "block"),
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("help_cat_other", lang),
				b.encodeCallback("help_cat", "other"),
			),
		),
		b.mainMenuBackRow(lang),
	)
	b.sendHTMLWithKeyboard(msg.Chat.ID, b.getText("help_title", lang), keyboard)
}

// pageSize is the number of items per page in list views.
const pageSize = 5

// handleList handles the /list command.
// Lists all domains bound to the user with an "Unbind" button for each, paginated.
func (b *Bot) handleList(msg *tgbotapi.Message, lang string) {
	domains := b.io.GetUserDomains(msg.Chat.ID)

	if len(domains) == 0 {
		b.sendHTMLMessage(msg.Chat.ID, b.getText("msg_no_domains", lang))
		return
	}

	text, rows := b.buildDomainPage(domains, 0, lang)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendHTMLWithKeyboard(msg.Chat.ID, text, keyboard)
}

// buildDomainPage builds the text and keyboard rows for a page of domains.
func (b *Bot) buildDomainPage(domains []string, page int, lang string) (string, [][]tgbotapi.InlineKeyboardButton) {
	total := len(domains)
	start := page * pageSize
	if start >= total {
		start = 0
		page = 0
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	text := b.getText("msg_list_title", lang) + "\n\n"
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := start; i < end; i++ {
		text += fmt.Sprintf("%d. <code>%s</code>\n", i+1, domains[i])
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_dismiss", lang)+" "+domains[i],
				b.encodeCallback("dismiss_ask", domains[i]),
			),
		))
	}

	// Pagination row
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages > 1 {
		var navBtns []tgbotapi.InlineKeyboardButton
		if page > 0 {
			navBtns = append(navBtns, tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_prev", lang), b.encodeCallback("list_page", fmt.Sprintf("%d", page-1)),
			))
		}
		navBtns = append(navBtns, tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%d/%d", page+1, totalPages), b.encodeCallback("noop"),
		))
		if page < totalPages-1 {
			navBtns = append(navBtns, tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_next", lang), b.encodeCallback("list_page", fmt.Sprintf("%d", page+1)),
			))
		}
		rows = append(rows, navBtns)
	}

	// Back to main menu
	rows = append(rows, b.mainMenuBackRow(lang))
	return text, rows
}

// handleBind handles the /bind command.
// Validates domain format, calls IO to bind, and sends success message with MX hint.
// If no args, sends usage hint.
func (b *Bot) handleBind(msg *tgbotapi.Message, lang string, args []string) {
	if len(args) == 0 {
		b.sendHTMLMessage(msg.Chat.ID, b.getText("usage_bind", lang))
		return
	}

	domain := args[0]
	if !DomainRegex.MatchString(domain) {
		b.sendHTMLMessage(msg.Chat.ID, b.getText("err_invalid_domain", lang))
		return
	}

	b.io.BindDomain(msg.Chat.ID, domain)
	successText := fmt.Sprintf(b.getText("msg_bind_success", lang), domain, b.config.MailDomain, domain)
	b.sendHTMLMessage(msg.Chat.ID, successText)
}

// handleDismiss handles the /dismiss command.
// Validates domain format, calls IO to unbind. If no args, sends usage hint.
func (b *Bot) handleDismiss(msg *tgbotapi.Message, lang string, args []string) {
	if len(args) == 0 {
		b.sendHTMLMessage(msg.Chat.ID, b.getText("usage_dismiss", lang))
		return
	}

	domain := args[0]
	if !DomainRegex.MatchString(domain) {
		b.sendHTMLMessage(msg.Chat.ID, b.getText("err_invalid_domain", lang))
		return
	}

	b.io.RemoveDomain(msg.Chat.ID, domain)
}

// handleUnblock handles the /unblock_domain, /unblock_sender, /unblock_receiver commands.
// Validates format based on blockType and calls the corresponding IO method.
// If no args, sends usage hint for the specific blockType.
func (b *Bot) handleUnblock(msg *tgbotapi.Message, lang string, blockType string, args []string) {
	if len(args) == 0 {
		switch blockType {
		case "domain":
			b.sendHTMLMessage(msg.Chat.ID, b.getText("usage_unblock_domain", lang))
		case "sender":
			b.sendHTMLMessage(msg.Chat.ID, b.getText("usage_unblock_sender", lang))
		case "receiver":
			b.sendHTMLMessage(msg.Chat.ID, b.getText("usage_unblock_receiver", lang))
		}
		return
	}

	value := args[0]

	switch blockType {
	case "domain":
		if !DomainRegex.MatchString(value) {
			b.sendHTMLMessage(msg.Chat.ID, b.getText("err_invalid_domain", lang))
			return
		}
		b.io.RemoveBlockDomain(msg.Chat.ID, value)
	case "sender":
		if !EmailRegex.MatchString(value) {
			b.sendHTMLMessage(msg.Chat.ID, b.getText("err_invalid_email", lang))
			return
		}
		b.io.RemoveBlockSender(msg.Chat.ID, value)
	case "receiver":
		if !EmailRegex.MatchString(value) {
			b.sendHTMLMessage(msg.Chat.ID, b.getText("err_invalid_email", lang))
			return
		}
		b.io.RemoveBlockReceiver(msg.Chat.ID, value)
	}
}

// handleSendAll handles the /send_all command (admin only).
// Verifies sender is admin, joins args into a message, broadcasts via IO, and sends confirmation.
func (b *Bot) handleSendAll(msg *tgbotapi.Message, args []string) {
	if msg.Chat.ID != b.config.AdminTgID {
		return
	}

	if len(args) == 0 {
		return
	}

	message := strings.Join(args, " ")
	b.io.SendAll(message)

	lang := b.detectLanguage(msg.From)
	b.sendHTMLMessage(msg.Chat.ID, b.getText("msg_broadcast_done", lang))
}

// handleListBlock handles the /list_block_domain, /list_block_sender, /list_block_receiver commands.
// Calls the corresponding IO list method based on blockType.
func (b *Bot) handleListBlock(msg *tgbotapi.Message, lang string, blockType string) {
	switch blockType {
	case "domain":
		b.io.ListBlockDomain(msg.Chat.ID)
	case "sender":
		b.io.ListBlockSender(msg.Chat.ID)
	case "receiver":
		b.io.ListBlockReceiver(msg.Chat.ID)
	}
}

// sendHTMLMessage sends an HTML-formatted message via bot.
func (b *Bot) sendHTMLMessage(chatID int64, text string) {
	if b.sender == nil {
		return
	}
	msgCfg := tgbotapi.NewMessage(chatID, text)
	msgCfg.ParseMode = "HTML"
	if _, err := b.sender.Send(msgCfg); err != nil {
		log.Printf("failed to send HTML message: %v", err)
	}
}

// detectLanguage detects the user's language preference.
// Priority: 1) DB-stored preference, 2) Telegram LanguageCode, 3) default "en".
func (b *Bot) detectLanguage(user *tgbotapi.User) string {
	if user == nil {
		return "en"
	}
	// Check DB-stored preference first
	if b.io != nil {
		if stored := b.io.GetUserLang(user.ID); stored != "" {
			return stored
		}
	}
	if strings.HasPrefix(strings.ToLower(user.LanguageCode), "zh") {
		return "zh"
	}
	return "en"
}

// detectLanguageStatic detects language from Telegram User without DB lookup (for package-level use).
func detectLanguageStatic(user *tgbotapi.User) string {
	if user == nil {
		return "en"
	}
	if strings.HasPrefix(strings.ToLower(user.LanguageCode), "zh") {
		return "zh"
	}
	return "en"
}

// parseCommand parses user input text into a structured command.
// It trims whitespace, splits by whitespace (supporting multiple spaces),
// strips the leading "/" from the command name, and collects remaining tokens as args.
func parseCommand(text string) ParsedCommand {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ParsedCommand{}
	}
	fields := strings.Fields(trimmed)
	cmd := strings.TrimPrefix(fields[0], "/")
	var args []string
	if len(fields) > 1 {
		args = fields[1:]
	}
	return ParsedCommand{
		Command: cmd,
		Args:    args,
		Raw:     trimmed,
	}
}

// callbackCachePrefix is prepended to snowflake IDs stored in the cache
// to distinguish them from regular colon-separated callback data.
const callbackCachePrefix = "cb:"

// encodeCallback encodes an action and optional params into a colon-separated
// callback data string. If the result exceeds 64 bytes (Telegram's limit),
// it stores the full string in IO's blockCache via a Snowflake ID and returns
// "cb:<snowflakeID>" instead.
func (b *Bot) encodeCallback(action string, params ...string) string {
	parts := make([]string, 0, 1+len(params))
	parts = append(parts, action)
	parts = append(parts, params...)
	result := strings.Join(parts, ":")

	if len(result) <= 64 {
		return result
	}

	// Data exceeds 64 bytes — cache it and return a short key
	id := b.io.StoreCallbackData(result)
	return callbackCachePrefix + id
}

// decodeCallback decodes callback data back into a CallbackData struct.
// If the data starts with "cb:", it retrieves the real data from IO's blockCache.
// Then splits by colon: first element is Action, rest are Params.
func (b *Bot) decodeCallback(data string) CallbackData {
	actual := data

	// Check if this is a cached callback (starts with "cb:")
	if strings.HasPrefix(data, callbackCachePrefix) {
		id := strings.TrimPrefix(data, callbackCachePrefix)
		if retrieved, ok := b.io.RetrieveCallbackData(id); ok {
			actual = retrieved
		}
	}

	parts := strings.Split(actual, ":")
	if len(parts) == 0 {
		return CallbackData{}
	}

	cd := CallbackData{
		Action: parts[0],
	}
	if len(parts) > 1 {
		cd.Params = parts[1:]
	}
	return cd
}

// --- Helper methods for callback handling ---

// editMessageWithKeyboard edits an existing message's text and inline keyboard.
func (b *Bot) editMessageWithKeyboard(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	if b.sender == nil {
		return
	}
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "HTML"
	editMsg.ReplyMarkup = &keyboard
	if _, err := b.sender.Send(editMsg); err != nil {
		log.Printf("failed to edit message with keyboard: %v", err)
	}
}

// editMessageNoKeyboard edits a message's text and removes the inline keyboard.
func (b *Bot) editMessageNoKeyboard(chatID int64, messageID int, text string) {
	if b.sender == nil {
		return
	}
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "HTML"
	if _, err := b.sender.Send(editMsg); err != nil {
		log.Printf("failed to edit message: %v", err)
	}
}

// answerCallbackQuery sends an answer to a callback query, optionally showing an alert popup.
func (b *Bot) answerCallbackQuery(queryID string, text string, showAlert bool) {
	if b.sender == nil {
		return
	}
	callback := tgbotapi.NewCallback(queryID, text)
	callback.ShowAlert = showAlert
	if _, err := b.sender.Request(callback); err != nil {
		log.Printf("failed to answer callback query: %v", err)
	}
}

// --- Task 6.1: Help-related Callback Handlers ---

// handleHelpCategory edits the message to show category details + "Back" button.
// Maps category to text key: "domain" → help_detail_domain, "block" → help_detail_block, "other" → help_detail_other.
func (b *Bot) handleHelpCategory(query *tgbotapi.CallbackQuery, lang string, category string) {
	msg := query.Message
	textKey := "help_detail_" + category
	detailText := b.getText(textKey, lang)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_back", lang),
				b.encodeCallback("help_back"),
			),
		),
	)

	b.editMessageWithKeyboard(msg.Chat.ID, msg.MessageID, detailText, keyboard)
	b.answerCallbackQuery(query.ID, "", false)
}

// handleHelpBack edits the message back to the help category list.
func (b *Bot) handleHelpBack(query *tgbotapi.CallbackQuery, lang string) {
	msg := query.Message

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("help_cat_domain", lang),
				b.encodeCallback("help_cat", "domain"),
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("help_cat_block", lang),
				b.encodeCallback("help_cat", "block"),
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("help_cat_other", lang),
				b.encodeCallback("help_cat", "other"),
			),
		),
		b.mainMenuBackRow(lang),
	)

	b.editMessageWithKeyboard(msg.Chat.ID, msg.MessageID, b.getText("help_title", lang), keyboard)
	b.answerCallbackQuery(query.ID, "", false)
}

// --- Task 6.2: Welcome Flow & Main Menu Callback Handlers ---

// handleQuickStart handles the "Quick Start" callback: binds a default domain for the user.
func (b *Bot) handleQuickStart(query *tgbotapi.CallbackQuery, lang string) {
	chatID := query.Message.Chat.ID
	domain := b.io.BindDefaultDomain(chatID)

	if domain == "" {
		b.sendHTMLMessage(chatID, b.getText("err_bind_default_fail", lang))
	} else {
		successText := fmt.Sprintf(b.getText("msg_bind_success", lang), domain, b.config.MailDomain, domain)
		b.sendHTMLMessage(chatID, successText)
	}

	b.answerCallbackQuery(query.ID, "", false)
}

// handleMainMenu routes main menu actions to the appropriate handler.
func (b *Bot) handleMainMenu(query *tgbotapi.CallbackQuery, lang string, action string) {
	chatID := query.Message.Chat.ID

	switch action {
	case "domains":
		// Create a fake message from the callback query to reuse handleList
		fakeMsg := &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: chatID},
			From: query.From,
		}
		b.handleList(fakeMsg, lang)
	case "blocks":
		// Send block management menu with 3 category buttons + back
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					b.getText("btn_blocked_domains", lang),
					b.encodeCallback("block_cat", "domain"),
				),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					b.getText("btn_blocked_senders", lang),
					b.encodeCallback("block_cat", "sender"),
				),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					b.getText("btn_blocked_receivers", lang),
					b.encodeCallback("block_cat", "receiver"),
				),
			),
			b.mainMenuBackRow(lang),
		)
		b.sendHTMLWithKeyboard(chatID, b.getText("btn_block_mgmt", lang), keyboard)
	case "help":
		// Create a fake message from the callback query to reuse handleHelp
		fakeMsg := &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: chatID},
			From: query.From,
		}
		b.handleHelp(fakeMsg, lang)
	case "lang":
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("English", b.encodeCallback("set_lang", "en")),
				tgbotapi.NewInlineKeyboardButtonData("中文", b.encodeCallback("set_lang", "zh")),
			),
			b.mainMenuBackRow(lang),
		)
		b.sendHTMLWithKeyboard(chatID, b.getText("msg_lang_select", lang), keyboard)
	}

	b.answerCallbackQuery(query.ID, "", false)
}

// --- Task 6.3: Domain Dismiss Confirmation Callback Handlers ---

// handleDismissAsk edits the message to show a confirmation prompt with Confirm and Cancel buttons.
func (b *Bot) handleDismissAsk(query *tgbotapi.CallbackQuery, lang string, domain string) {
	msg := query.Message
	promptText := fmt.Sprintf(b.getText("msg_dismiss_confirm_prompt", lang), domain)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_confirm", lang),
				b.encodeCallback("dismiss_yes", domain),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_cancel", lang),
				b.encodeCallback("dismiss_no"),
			),
		),
	)

	b.editMessageWithKeyboard(msg.Chat.ID, msg.MessageID, promptText, keyboard)
	b.answerCallbackQuery(query.ID, "", false)
}

// handleDismissConfirm calls IO to remove the domain and edits the message to show success.
func (b *Bot) handleDismissConfirm(query *tgbotapi.CallbackQuery, lang string, domain string) {
	msg := query.Message
	chatID := msg.Chat.ID

	b.io.RemoveDomain(chatID, domain)

	// Edit message to show success (remove keyboard)
	successText := b.getText("msg_dismiss_success", lang)
	b.editMessageNoKeyboard(chatID, msg.MessageID, successText)
	b.answerCallbackQuery(query.ID, "", false)
}

// handleDismissCancel edits the message to show "Operation cancelled".
func (b *Bot) handleDismissCancel(query *tgbotapi.CallbackQuery, lang string) {
	msg := query.Message

	cancelText := b.getText("msg_dismiss_cancel", lang)
	b.editMessageNoKeyboard(msg.Chat.ID, msg.MessageID, cancelText)
	b.answerCallbackQuery(query.ID, "", false)
}

// --- Task 6.4: Block Management Callback Handlers ---

// handleBlockAction calls the corresponding IO block method and answers with a success popup.
func (b *Bot) handleBlockAction(query *tgbotapi.CallbackQuery, lang string, blockType string, target string) {
	chatID := query.Message.Chat.ID

	switch blockType {
	case "sender":
		b.io.BlockSender(chatID, target)
	case "domain":
		b.io.BlockDomain(chatID, target)
	case "receiver":
		b.io.BlockReceiver(chatID, target)
	}

	successText := fmt.Sprintf(b.getText("msg_block_success", lang), target)
	b.answerCallbackQuery(query.ID, successText, false)
}

// handleUnblockAction calls the corresponding IO unblock method and re-renders the block category list.
func (b *Bot) handleUnblockAction(query *tgbotapi.CallbackQuery, lang string, blockType string, target string) {
	chatID := query.Message.Chat.ID

	switch blockType {
	case "sender":
		b.io.RemoveBlockSender(chatID, target)
	case "domain":
		b.io.RemoveBlockDomain(chatID, target)
	case "receiver":
		b.io.RemoveBlockReceiver(chatID, target)
	}

	// Re-render the block category list by calling handleBlockCategory
	b.handleBlockCategory(query, lang, blockType)
}

// handleBlockCategory edits the message to show all blocked items in a category with "Unblock" buttons, paginated.
func (b *Bot) handleBlockCategory(query *tgbotapi.CallbackQuery, lang string, category string) {
	b.handleBlockCategoryPage(query, lang, category, 0)
}

// handleBlockCategoryPage renders a specific page of blocked items.
func (b *Bot) handleBlockCategoryPage(query *tgbotapi.CallbackQuery, lang string, category string, page int) {
	msg := query.Message
	chatID := msg.Chat.ID

	var items []string
	switch category {
	case "sender":
		items = b.io.GetBlockedSenders(chatID)
	case "domain":
		items = b.io.GetBlockedDomains(chatID)
	case "receiver":
		items = b.io.GetBlockedReceivers(chatID)
	}

	if len(items) == 0 {
		// Empty list with back button to block management
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					b.getText("btn_back", lang),
					b.encodeCallback("main_menu", "blocks"),
				),
			),
		)
		b.editMessageWithKeyboard(chatID, msg.MessageID, b.getText("msg_no_blocks", lang), keyboard)
		b.answerCallbackQuery(query.ID, "", false)
		return
	}

	total := len(items)
	start := page * pageSize
	if start >= total {
		start = 0
		page = 0
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	text := ""
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := start; i < end; i++ {
		text += fmt.Sprintf("%d. <code>%s</code>\n", i+1, items[i])
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_unblock", lang)+" "+items[i],
				b.encodeCallback("unblock_"+category, items[i]),
			),
		))
	}

	// Pagination row
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages > 1 {
		var navBtns []tgbotapi.InlineKeyboardButton
		if page > 0 {
			navBtns = append(navBtns, tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_prev", lang), b.encodeCallback("block_page", category, fmt.Sprintf("%d", page-1)),
			))
		}
		navBtns = append(navBtns, tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%d/%d", page+1, totalPages), b.encodeCallback("noop"),
		))
		if page < totalPages-1 {
			navBtns = append(navBtns, tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_next", lang), b.encodeCallback("block_page", category, fmt.Sprintf("%d", page+1)),
			))
		}
		rows = append(rows, navBtns)
	}

	// Back to block management
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(
			b.getText("btn_back", lang),
			b.encodeCallback("main_menu", "blocks"),
		),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.editMessageWithKeyboard(chatID, msg.MessageID, text, keyboard)
	b.answerCallbackQuery(query.ID, "", false)
}

// handleLang sends a language selection menu.
func (b *Bot) handleLang(msg *tgbotapi.Message) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("English", b.encodeCallback("set_lang", "en")),
			tgbotapi.NewInlineKeyboardButtonData("中文", b.encodeCallback("set_lang", "zh")),
		),
	)
	b.sendHTMLWithKeyboard(msg.Chat.ID, "Choose your language / 选择语言：", keyboard)
}

// handleSetLang saves the user's language preference and confirms.
func (b *Bot) handleSetLang(query *tgbotapi.CallbackQuery, lang string) {
	if lang != "zh" && lang != "en" {
		lang = "en"
	}
	if err := b.io.SetUserLang(query.From.ID, lang); err != nil {
		log.Printf("failed to set user lang: %v", err)
	}
	b.editMessageNoKeyboard(query.Message.Chat.ID, query.Message.MessageID, b.getText("msg_lang_set", lang))
	b.answerCallbackQuery(query.ID, "", false)
}

// handleGoMain sends the main menu (same as /start for existing users).
func (b *Bot) handleGoMain(query *tgbotapi.CallbackQuery, lang string) {
	chatID := query.Message.Chat.ID
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_my_domains", lang),
				b.encodeCallback("main_menu", "domains"),
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_block_mgmt", lang),
				b.encodeCallback("main_menu", "blocks"),
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_help", lang),
				b.encodeCallback("main_menu", "help"),
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.getText("btn_lang", lang),
				b.encodeCallback("main_menu", "lang"),
			),
		),
	)
	b.editMessageWithKeyboard(chatID, query.Message.MessageID, b.getText("welcome_back", lang), keyboard)
	b.answerCallbackQuery(query.ID, "", false)
}

// mainMenuBackRow returns a keyboard row with a "Main Menu" button.
func (b *Bot) mainMenuBackRow(lang string) []tgbotapi.InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(
			b.getText("btn_main_menu", lang),
			b.encodeCallback("go_main"),
		),
	)
}

// handleListPage handles pagination for the domain list via callback.
func (b *Bot) handleListPage(query *tgbotapi.CallbackQuery, lang string, page int) {
	chatID := query.Message.Chat.ID
	domains := b.io.GetUserDomains(chatID)

	if len(domains) == 0 {
		b.editMessageNoKeyboard(chatID, query.Message.MessageID, b.getText("msg_no_domains", lang))
		b.answerCallbackQuery(query.ID, "", false)
		return
	}

	text, rows := b.buildDomainPage(domains, page, lang)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.editMessageWithKeyboard(chatID, query.Message.MessageID, text, keyboard)
	b.answerCallbackQuery(query.ID, "", false)
}
