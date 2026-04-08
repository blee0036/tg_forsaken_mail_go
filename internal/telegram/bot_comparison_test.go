package telegram

import (
	"testing"

	"go-version-rewrite/internal/config"
)

// Task 8.3: 对比验证：Telegram Bot 模块
// Validates: Requirements 6.16, 6.17
// Verifies Go version helpMsg, helpMsgCN, command reply messages, domain regex,
// and email regex are character-level identical to Node version telegram.js.

const testMailDomain = "tgmail.party"

// nodeHelpMsg is the exact string produced by Node version telegram.js helpMsg
// with mail_domain = "tgmail.party", after resolving all string concatenation.
const nodeHelpMsg = "<b>list_domain :</b> \n" +
	"\n <code>/list</code> \n\n" +
	"list all your binded domain. \n\n" +
	"<b>bind_domain :</b> \n" +
	"\n <code>/bind example.com</code>\n\n" +
	"1. prepare one domain to catch all mails. \n" +
	"2. add the MX record to [<code>tgmail.party</code>] \n" +
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

// nodeHelpMsgCN is the exact string produced by Node version telegram.js helpMsgCN
// with mail_domain = "tgmail.party", after resolving all string concatenation.
const nodeHelpMsgCN = "<b>显示所有绑定域名 :</b> \n" +
	"\n <code>/list</code> \n\n" +
	"<b>绑定域名 :</b> \n" +
	"\n <code>/bind example.com</code>\n\n" +
	"1. 准备一个用于接收所有邮件的域名. \n" +
	"2. 添加MX记录解析到 [<code>tgmail.party</code>] \n" +
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


// TestComparison_HelpMsgIdentical verifies Go version helpMsg is character-level
// identical to Node version telegram.js helpMsg.
func TestComparison_HelpMsgIdentical(t *testing.T) {
	goHelpMsg, _ := GenerateHelpMessages(testMailDomain)

	if goHelpMsg != nodeHelpMsg {
		// Find first difference for debugging
		minLen := len(goHelpMsg)
		if len(nodeHelpMsg) < minLen {
			minLen = len(nodeHelpMsg)
		}
		for i := 0; i < minLen; i++ {
			if goHelpMsg[i] != nodeHelpMsg[i] {
				t.Fatalf("helpMsg differs at byte %d: Go=0x%02x(%q) Node=0x%02x(%q)\nGo context:   ...%q...\nNode context: ...%q...",
					i, goHelpMsg[i], string(goHelpMsg[i]),
					nodeHelpMsg[i], string(nodeHelpMsg[i]),
					safeSlice(goHelpMsg, i-20, i+20),
					safeSlice(nodeHelpMsg, i-20, i+20))
				return
			}
		}
		t.Fatalf("helpMsg length differs: Go=%d Node=%d", len(goHelpMsg), len(nodeHelpMsg))
	}
}

// TestComparison_HelpMsgCNIdentical verifies Go version helpMsgCN is character-level
// identical to Node version telegram.js helpMsgCN.
func TestComparison_HelpMsgCNIdentical(t *testing.T) {
	_, goHelpMsgCN := GenerateHelpMessages(testMailDomain)

	if goHelpMsgCN != nodeHelpMsgCN {
		minLen := len(goHelpMsgCN)
		if len(nodeHelpMsgCN) < minLen {
			minLen = len(nodeHelpMsgCN)
		}
		for i := 0; i < minLen; i++ {
			if goHelpMsgCN[i] != nodeHelpMsgCN[i] {
				t.Fatalf("helpMsgCN differs at byte %d: Go=0x%02x(%q) Node=0x%02x(%q)\nGo context:   ...%q...\nNode context: ...%q...",
					i, goHelpMsgCN[i], string(goHelpMsgCN[i]),
					nodeHelpMsgCN[i], string(nodeHelpMsgCN[i]),
					safeSlice(goHelpMsgCN, i-20, i+20),
					safeSlice(nodeHelpMsgCN, i-20, i+20))
				return
			}
		}
		t.Fatalf("helpMsgCN length differs: Go=%d Node=%d", len(goHelpMsgCN), len(nodeHelpMsgCN))
	}
}

// TestComparison_DomainRegexMatches verifies Go domain regex matches the same
// test strings as Node version /((?=[a-z0-9-]{1,63}\.)(xn--)?[a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,63}/i
func TestComparison_DomainRegexMatches(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"invalid", false},
		{"a.b", false}, // tld too short (1 char)
	}

	for _, tc := range tests {
		got := DomainRegex.MatchString(tc.input)
		if got != tc.want {
			t.Errorf("DomainRegex.MatchString(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// TestComparison_EmailRegexMatches verifies Go email regex matches the same
// test strings as Node version /[\w\._\-\+]+@[\w\._\-\+]+/i
func TestComparison_EmailRegexMatches(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"user@example.com", true},
		{"user+tag@domain.com", true},
		{"invalid", false},
		{"@domain", false},
	}

	for _, tc := range tests {
		got := EmailRegex.MatchString(tc.input)
		if got != tc.want {
			t.Errorf("EmailRegex.MatchString(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// TestComparison_InvalidCommandMessage verifies the unknown command reply message
// uses the new friendly prompt from the redesigned interaction.
func TestComparison_InvalidCommandMessage(t *testing.T) {
	// After the telegram-interaction-redesign, the old "Invalid method!" message
	// has been replaced with a localized "err_unknown_cmd" text + "View Help" button.
	// Verify the new text keys exist and are non-empty.
	b := NewForTest(&config.Config{MailDomain: "test.example.com"}, nil, nil)
	enMsg := b.getText("err_unknown_cmd", "en")
	zhMsg := b.getText("err_unknown_cmd", "zh")
	if enMsg == "" {
		t.Error("err_unknown_cmd EN text is empty")
	}
	if zhMsg == "" {
		t.Error("err_unknown_cmd ZH text is empty")
	}
	if enMsg == zhMsg {
		t.Error("err_unknown_cmd EN and ZH should differ")
	}
}

// TestComparison_InvalidDomainMessage verifies the invalid domain reply message
// uses the new localized error text from the redesigned interaction.
func TestComparison_InvalidDomainMessage(t *testing.T) {
	b := NewForTest(&config.Config{MailDomain: "test.example.com"}, nil, nil)
	enMsg := b.getText("err_invalid_domain", "en")
	zhMsg := b.getText("err_invalid_domain", "zh")
	if enMsg == "" {
		t.Error("err_invalid_domain EN text is empty")
	}
	if zhMsg == "" {
		t.Error("err_invalid_domain ZH text is empty")
	}
}

// TestComparison_InvalidEmailMessage verifies the invalid email reply message
// uses the new localized error text from the redesigned interaction.
func TestComparison_InvalidEmailMessage(t *testing.T) {
	b := NewForTest(&config.Config{MailDomain: "test.example.com"}, nil, nil)
	enMsg := b.getText("err_invalid_email", "en")
	zhMsg := b.getText("err_invalid_email", "zh")
	if enMsg == "" {
		t.Error("err_invalid_email EN text is empty")
	}
	if zhMsg == "" {
		t.Error("err_invalid_email ZH text is empty")
	}
}

// safeSlice returns a safe substring of s from start to end, clamped to bounds.
func safeSlice(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(s) {
		end = len(s)
	}
	if start >= end {
		return ""
	}
	return s[start:end]
}
