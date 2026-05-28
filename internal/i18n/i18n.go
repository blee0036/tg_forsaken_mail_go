// Package i18n provides internationalization support for user-visible text.
// It is a shared package used by both the telegram bot and the io module.
package i18n

// LangTexts stores the English and Chinese versions of a text string.
type LangTexts struct {
	EN string
	ZH string
}

// texts contains all user-visible text entries.
var texts = map[string]LangTexts{
	"msg_truncated": {
		EN: "Content truncated, view full email for details",
		ZH: "内容已截断，请查看完整邮件",
	},
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

// Get returns the localized text for the given key and language code.
// Falls back to English if the language is not "zh".
// Returns empty string if not found.
func Get(key string, lang string) string {
	entry, ok := texts[key]
	if !ok {
		return ""
	}
	if lang == "zh" {
		return entry.ZH
	}
	return entry.EN
}

// Register adds or overwrites a text entry. Used by other packages to register
// their own text entries at init time.
func Register(key string, entry LangTexts) {
	texts[key] = entry
}

// RegisterAll adds multiple text entries at once.
func RegisterAll(entries map[string]LangTexts) {
	for k, v := range entries {
		texts[k] = v
	}
}

// AllKeys returns all registered text keys. Useful for testing.
func AllKeys() []string {
	keys := make([]string, 0, len(texts))
	for k := range texts {
		keys = append(keys, k)
	}
	return keys
}
