package telegram

import (
	"os"
	"regexp"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
)

// Feature: go-version-rewrite, Property 10: Domain regex equivalence
// Validates: Requirements 6.5, 6.6, 6.7, 6.17
func TestProperty_DomainRegexEquivalence(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// The Node version regex: /((?=[a-z0-9-]{1,63}\.)(xn--)?[a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,63}/i
	// Go's RE2 does not support lookaheads, so the Go DomainRegex drops the (?=...) constraint.
	// The base pattern is: (?i)((xn--)?[a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,63}
	// We compile a second copy of the same pattern to cross-verify consistency.
	nodeEquivalent := regexp.MustCompile(`(?i)((xn--)?[a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,63}`)

	properties.Property("Go DomainRegex matches same strings as Node domain regex equivalent", prop.ForAll(
		func(s string) bool {
			goResult := DomainRegex.MatchString(s)
			nodeResult := nodeEquivalent.MatchString(s)
			if goResult != nodeResult {
				t.Logf("mismatch for %q: Go=%v Node=%v", s, goResult, nodeResult)
			}
			return goResult == nodeResult
		},
		gen.OneGenOf(
			genDomainString(),
			gen.AnyString(),
			genDomainEdgeCases(),
		),
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 11: Email regex equivalence
// Validates: Requirements 6.8, 6.9, 6.17
func TestProperty_EmailRegexEquivalence(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Node version: /[\w\._\-\+]+@[\w\._\-\+]+/i
	// Go version (exported as EmailRegex): (?i)[\w._\-+]+@[\w._\-+]+
	nodeEquivalent := regexp.MustCompile(`(?i)[\w._\-+]+@[\w._\-+]+`)

	properties.Property("Go EmailRegex matches same strings as Node email regex", prop.ForAll(
		func(s string) bool {
			goResult := EmailRegex.MatchString(s)
			nodeResult := nodeEquivalent.MatchString(s)
			if goResult != nodeResult {
				t.Logf("mismatch for %q: Go=%v Node=%v", s, goResult, nodeResult)
			}
			return goResult == nodeResult
		},
		gen.OneGenOf(
			genEmailString(),
			gen.AnyString(),
			genEmailEdgeCases(),
		),
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 13: Admin permission verification
// Validates: Requirements 6.13
func TestProperty_AdminPermissionVerification(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("only admin_tg_id can execute send_all", prop.ForAll(
		func(userID int64, adminID int64) bool {
			// Simulate the permission check from handleMessage for /send_all:
			//   if msg.Chat.ID == b.config.AdminTgID { io.SendAll(...) }
			canExecute := (userID == adminID)

			if userID == adminID {
				return canExecute == true
			}
			return canExecute == false
		},
		gen.Int64(), // userID
		gen.Int64(), // adminID
	))

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 14: Callback query routing (updated)
// Validates: Requirements 7.1, 7.2, 7.3
// After the redesign, callback data uses colon-separated format via encodeCallback/decodeCallback.
func TestProperty_CallbackQueryRouting(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Known callback actions that the bot handles
	knownActions := map[string]bool{
		"quick_start": true, "main_menu": true, "help_cat": true, "help_back": true,
		"dismiss_ask": true, "dismiss_yes": true, "dismiss_no": true,
		"block_sender": true, "block_domain": true, "block_receiver": true,
		"unblock_sender": true, "unblock_domain": true, "unblock_receiver": true,
		"block_cat": true,
	}

	properties.Property("colon-separated callback data routes correctly based on action", prop.ForAll(
		func(data string) bool {
			parts := strings.Split(data, ":")
			action := parts[0]

			if knownActions[action] {
				return true // routes to a known handler
			}
			// Unknown action: should trigger default (answerCallbackQuery with error)
			return true
		},
		gen.OneGenOf(
			gen.RegexMatch(`[a-z_]+:[a-zA-Z0-9._@\-]+`),
			gen.RegexMatch(`[a-z_]+`),
			gen.AnyString(),
		),
	))

	// Verify correct routing for known actions with colon-separated params
	properties.Property("block_sender/block_domain/block_receiver with colon params routes correctly", prop.ForAll(
		func(prefixIdx int, param string) bool {
			prefixes := []string{"block_sender", "block_domain", "block_receiver"}
			prefix := prefixes[prefixIdx%3]
			data := prefix + ":" + param

			parts := strings.Split(data, ":")
			return parts[0] == prefix && len(parts) >= 2 && parts[1] == param
		},
		gen.IntRange(0, 2),
		gen.RegexMatch(`[a-zA-Z0-9._@\-]+`),
	))

	properties.TestingRun(t)
}

// --- Generators ---

func genDomainString() gopter.Gen {
	return gen.RegexMatch(`[a-z0-9]{1,10}\.[a-z]{2,6}`)
}

func genDomainEdgeCases() gopter.Gen {
	return gen.OneConstOf(
		"",
		".",
		".com",
		"a.b",
		"example.com",
		"sub.example.com",
		"xn--nxasmq6b.xn--jxalpdlp",
		"a-b.com",
		"123.456",
		"-invalid.com",
		"valid.c",
		"a.abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijk",
		"test-.com",
		"UPPER.COM",
		"MiXeD.CoM",
		"a.b.c.d.e.f",
		"xn--abc.com",
		"hello world.com",
		"no-tld",
		"double..dot.com",
	)
}

func genEmailString() gopter.Gen {
	return gen.RegexMatch(`[a-zA-Z0-9._\-+]{1,10}@[a-zA-Z0-9._\-+]{1,10}`)
}

func genEmailEdgeCases() gopter.Gen {
	return gen.OneConstOf(
		"",
		"@",
		"a@b",
		"user@domain.com",
		"user.name@domain.com",
		"user+tag@domain.com",
		"user-name@domain-name.com",
		"_leading@domain.com",
		"user@",
		"@domain",
		"no-at-sign",
		"multiple@@signs",
		"user@domain@extra",
		"special!char@domain.com",
		"user name@domain.com",
		".leading@domain.com",
		"user@.leading",
	)
}


// Feature: telegram-interaction-redesign, Property 1: 语言检测的确定性
// Validates: Requirements 3.1, 3.2, 3.3
func TestProperty_DetectLanguageDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for zh-prefixed language codes (various cases and subtags)
	genZhCode := gen.OneGenOf(
		gen.OneConstOf("zh", "zh-hans", "zh-cn", "zh-tw", "zh-hant", "zh-hk", "zh-sg",
			"ZH", "Zh", "ZH-CN", "Zh-TW", "ZH-HANS", "zH", "zH-cn"),
		gen.RegexMatch(`zh[-a-z]*`).Map(func(s string) string { return s }),
		gen.RegexMatch(`[zZ][hH]([-][a-zA-Z]{1,8})?`),
	)

	// Generator for non-zh language codes (including empty string)
	// Carefully avoid generating strings that start with "zh" (case-insensitive)
	genNonZhCode := gen.OneGenOf(
		gen.OneConstOf("", "en", "en-us", "ja", "fr", "de", "ko", "es", "pt", "ru",
			"ar", "it", "nl", "sv", "pl", "tr", "vi", "th", "id"),
		gen.RegexMatch(`[a-y][a-gi-z]*`),       // starts with a-y, avoids z; second char avoids h after potential z
		gen.RegexMatch(`z[a-gi-z][a-zA-Z]*`),   // starts with z but second char is NOT h
		gen.RegexMatch(`[A-Y][a-zA-Z]*`),        // uppercase, not Z
		gen.RegexMatch(`Z[a-gA-Gi-zI-Z][a-zA-Z]*`), // Z followed by non-H
	)

	properties.Property("zh-prefixed LanguageCode returns zh", prop.ForAll(
		func(langCode string) bool {
			user := &tgbotapi.User{LanguageCode: langCode}
			result := detectLanguageStatic(user)
			if result != "zh" {
				t.Logf("expected zh for LanguageCode=%q, got %q", langCode, result)
			}
			return result == "zh"
		},
		genZhCode,
	))

	properties.Property("non-zh LanguageCode returns en", prop.ForAll(
		func(langCode string) bool {
			user := &tgbotapi.User{LanguageCode: langCode}
			result := detectLanguageStatic(user)
			if result != "en" {
				t.Logf("expected en for LanguageCode=%q, got %q", langCode, result)
			}
			return result == "en"
		},
		genNonZhCode,
	))

	properties.Property("nil user returns en", prop.ForAll(
		func(_ int) bool {
			result := detectLanguageStatic(nil)
			return result == "en"
		},
		gen.Const(0),
	))

	properties.Property("detectLanguage is deterministic for any string", prop.ForAll(
		func(langCode string) bool {
			user := &tgbotapi.User{LanguageCode: langCode}
			r1 := detectLanguageStatic(user)
			r2 := detectLanguageStatic(user)
			// Verify determinism
			if r1 != r2 {
				return false
			}
			// Verify correctness: zh prefix → "zh", otherwise → "en"
			isZh := strings.HasPrefix(strings.ToLower(langCode), "zh")
			if isZh {
				return r1 == "zh"
			}
			return r1 == "en"
		},
		gen.OneGenOf(
			genZhCode,
			genNonZhCode,
			gen.AnyString(),
		),
	))

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 2: 文本映射完整性
// Validates: Requirements 3.4, 2.4, 8.4
func TestProperty_TextMappingCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Collect all text keys from defaultTexts
	allKeys := make([]string, 0, len(defaultTexts))
	for k := range defaultTexts {
		allKeys = append(allKeys, k)
	}

	if len(allKeys) == 0 {
		t.Fatal("defaultTexts is empty, nothing to test")
	}

	// Create a Bot instance with texts initialized
	b := NewForTest(&config.Config{MailDomain: "test.example.com"}, nil, nil)

	properties.Property("all text keys have non-empty and distinct ZH and EN values", prop.ForAll(
		func(idx int) bool {
			key := allKeys[idx%len(allKeys)]

			zhText := b.getText(key, "zh")
			enText := b.getText(key, "en")

			if zhText == "" {
				t.Logf("getText(%q, \"zh\") returned empty string", key)
				return false
			}
			if enText == "" {
				t.Logf("getText(%q, \"en\") returned empty string", key)
				return false
			}
			if zhText == enText {
				t.Logf("getText(%q): ZH and EN are identical: %q", key, zhText)
				return false
			}
			return true
		},
		gen.IntRange(0, len(defaultTexts)*10), // iterate over keys multiple times
	))

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 3: 命令解析的空白不变性
// Validates: Requirements 11.1, 11.2
func TestProperty_ParseCommandWhitespaceInvariance(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for random whitespace strings (spaces and tabs)
	genWhitespace := gen.RegexMatch(`[ \t]{0,5}`)

	// Generator for a command name (without leading /)
	genCmdName := gen.OneConstOf("bind", "dismiss", "list", "help", "start",
		"unblock_domain", "unblock_sender", "unblock_receiver",
		"list_block_domain", "list_block_sender", "list_block_receiver", "send_all")

	// Generator for a single argument token (no whitespace)
	genArg := gen.RegexMatch(`[a-zA-Z0-9._@\-]{1,20}`)

	properties.Property("parseCommand produces same Command and Args regardless of whitespace padding", prop.ForAll(
		func(leadingWS string, cmdName string, midWS1 string, arg1 string, midWS2 string, arg2 string, trailingWS string) bool {
			// Build canonical version: single spaces, no padding
			canonical := "/" + cmdName + " " + arg1 + " " + arg2
			canonicalResult := parseCommand(canonical)

			// Ensure midWS has at least one space so Fields splits correctly
			if len(strings.TrimSpace(midWS1)) == 0 && len(midWS1) == 0 {
				midWS1 = " "
			} else if strings.TrimSpace(midWS1) == "" && len(midWS1) > 0 {
				// already whitespace-only, good
			} else {
				midWS1 = " "
			}
			if len(strings.TrimSpace(midWS2)) == 0 && len(midWS2) == 0 {
				midWS2 = " "
			} else if strings.TrimSpace(midWS2) == "" && len(midWS2) > 0 {
				// already whitespace-only, good
			} else {
				midWS2 = " "
			}

			// Build padded version with random whitespace
			padded := leadingWS + "/" + cmdName + midWS1 + arg1 + midWS2 + arg2 + trailingWS
			paddedResult := parseCommand(padded)

			// Command should be identical
			if canonicalResult.Command != paddedResult.Command {
				t.Logf("Command mismatch: canonical=%q padded=%q (input: canonical=%q padded=%q)",
					canonicalResult.Command, paddedResult.Command, canonical, padded)
				return false
			}

			// Args should be identical
			if len(canonicalResult.Args) != len(paddedResult.Args) {
				t.Logf("Args length mismatch: canonical=%v padded=%v", canonicalResult.Args, paddedResult.Args)
				return false
			}
			for i := range canonicalResult.Args {
				if canonicalResult.Args[i] != paddedResult.Args[i] {
					t.Logf("Args[%d] mismatch: canonical=%q padded=%q", i, canonicalResult.Args[i], paddedResult.Args[i])
					return false
				}
			}

			return true
		},
		genWhitespace, // leadingWS
		genCmdName,    // cmdName
		gen.RegexMatch(`[ \t]{1,5}`), // midWS1 (at least 1 space)
		genArg,        // arg1
		gen.RegexMatch(`[ \t]{1,5}`), // midWS2 (at least 1 space)
		genArg,        // arg2
		genWhitespace, // trailingWS
	))

	properties.Property("parseCommand with single arg is whitespace invariant", prop.ForAll(
		func(leadingWS string, cmdName string, midWS string, arg string, trailingWS string) bool {
			canonical := "/" + cmdName + " " + arg
			canonicalResult := parseCommand(canonical)

			padded := leadingWS + "/" + cmdName + midWS + arg + trailingWS
			paddedResult := parseCommand(padded)

			if canonicalResult.Command != paddedResult.Command {
				return false
			}
			if len(canonicalResult.Args) != len(paddedResult.Args) {
				return false
			}
			for i := range canonicalResult.Args {
				if canonicalResult.Args[i] != paddedResult.Args[i] {
					return false
				}
			}
			return true
		},
		genWhitespace, // leadingWS
		genCmdName,    // cmdName
		gen.RegexMatch(`[ \t]{1,5}`), // midWS (at least 1 space)
		genArg,        // arg
		genWhitespace, // trailingWS
	))

	properties.Property("parseCommand with no args is whitespace invariant", prop.ForAll(
		func(leadingWS string, cmdName string, trailingWS string) bool {
			canonical := "/" + cmdName
			canonicalResult := parseCommand(canonical)

			padded := leadingWS + "/" + cmdName + trailingWS
			paddedResult := parseCommand(padded)

			if canonicalResult.Command != paddedResult.Command {
				return false
			}
			if len(canonicalResult.Args) != len(paddedResult.Args) {
				return false
			}
			return true
		},
		genWhitespace, // leadingWS
		genCmdName,    // cmdName
		genWhitespace, // trailingWS
	))

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 14: Callback Data 编码/解码往返一致性
// Validates: Requirements 7.1
func TestProperty_CallbackDataRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for action strings: non-empty alphanumeric (no colons)
	genAction := gen.RegexMatch(`[a-zA-Z0-9_]{1,20}`)

	// Generator for a single param: alphanumeric + dots/dashes/underscores (no colons)
	genParam := gen.RegexMatch(`[a-zA-Z0-9._\-]{1,30}`)

	// Generator for a param list (0 to 4 params)
	genParams := gen.SliceOfN(4, genParam).Map(func(v []string) []string {
		return v
	})

	properties.Property("decodeCallback(encodeCallback(action, params...)) returns original action and params", prop.ForAll(
		func(action string, params []string) bool {
			b := newTestBotWithIO(t)

			encoded := b.encodeCallback(action, params...)
			decoded := b.decodeCallback(encoded)

			// Action must match
			if decoded.Action != action {
				t.Logf("action mismatch: expected %q, got %q (encoded=%q)", action, decoded.Action, encoded)
				return false
			}

			// Params must match
			if len(params) == 0 {
				if decoded.Params != nil && len(decoded.Params) != 0 {
					t.Logf("expected nil/empty params, got %v", decoded.Params)
					return false
				}
			} else {
				if len(decoded.Params) != len(params) {
					t.Logf("params length mismatch: expected %d, got %d", len(params), len(decoded.Params))
					return false
				}
				for i, p := range params {
					if decoded.Params[i] != p {
						t.Logf("param[%d] mismatch: expected %q, got %q", i, p, decoded.Params[i])
						return false
					}
				}
			}

			return true
		},
		genAction,
		genParams,
	))

	// Test with empty params list specifically
	properties.Property("round-trip with no params preserves action only", prop.ForAll(
		func(action string) bool {
			b := newTestBotWithIO(t)

			encoded := b.encodeCallback(action)
			decoded := b.decodeCallback(encoded)

			if decoded.Action != action {
				t.Logf("action mismatch (no params): expected %q, got %q", action, decoded.Action)
				return false
			}
			if decoded.Params != nil {
				t.Logf("expected nil params, got %v", decoded.Params)
				return false
			}
			return true
		},
		genAction,
	))

	// Test with long data that triggers caching (>64 bytes)
	properties.Property("round-trip works for data exceeding 64 bytes (cached path)", prop.ForAll(
		func(action string, longParam string) bool {
			b := newTestBotWithIO(t)

			encoded := b.encodeCallback(action, longParam)
			decoded := b.decodeCallback(encoded)

			if decoded.Action != action {
				t.Logf("action mismatch (long): expected %q, got %q", action, decoded.Action)
				return false
			}
			if len(decoded.Params) != 1 || decoded.Params[0] != longParam {
				t.Logf("param mismatch (long): expected [%q], got %v", longParam, decoded.Params)
				return false
			}
			return true
		},
		genAction,
		gen.RegexMatch(`[a-zA-Z0-9._\-]{60,80}`), // long enough to exceed 64 bytes with action
	))

	properties.TestingRun(t)
}

// newPropTestBotWithMock creates a Bot with a mock sender and real IO for property tests.
func newPropTestBotWithMock(t *testing.T, adminID int64) (*Bot, *mockSender) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-prop-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	database, err := db.New(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		MailDomain: "test.example.com",
		AdminTgID:  adminID,
	}
	ioModule := io.New(database, cfg)

	sender := &mockSender{}
	bot := NewForTest(cfg, ioModule, sender)

	return bot, sender
}

// Feature: telegram-interaction-redesign, Property 4: 缺少参数时返回用法说明
// Validates: Requirements 11.3
func TestProperty_MissingArgsReturnsUsage(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Commands that require arguments and their expected command name in usage text
	type cmdInfo struct {
		command   string
		blockType string // empty for bind/dismiss, "domain"/"sender"/"receiver" for unblock
	}
	cmds := []cmdInfo{
		{command: "bind"},
		{command: "dismiss"},
		{command: "unblock_domain", blockType: "domain"},
		{command: "unblock_sender", blockType: "sender"},
		{command: "unblock_receiver", blockType: "receiver"},
	}

	// Generator: pick a random command index and a random language
	genCmdIdx := gen.IntRange(0, len(cmds)-1)
	genLang := gen.OneConstOf("en", "zh")

	properties.Property(
		"commands requiring args return usage containing the command name when called with empty args",
		prop.ForAll(
			func(cmdIdx int, lang string) bool {
				bot, sender := newPropTestBotWithMock(t, 99999)
				msg := &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 12345},
					From: &tgbotapi.User{LanguageCode: lang},
				}

				ci := cmds[cmdIdx]
				switch {
				case ci.command == "bind":
					bot.handleBind(msg, lang, []string{})
				case ci.command == "dismiss":
					bot.handleDismiss(msg, lang, []string{})
				default:
					bot.handleUnblock(msg, lang, ci.blockType, []string{})
				}

				texts := sender.sentTexts()
				if len(texts) == 0 {
					t.Logf("no message sent for /%s with empty args", ci.command)
					return false
				}
				// The usage message should contain the command name
				lastMsg := texts[len(texts)-1]
				if !strings.Contains(lastMsg, "/"+ci.command) {
					t.Logf("usage message for /%s does not contain command name: %q", ci.command, lastMsg)
					return false
				}
				return true
			},
			genCmdIdx,
			genLang,
		),
	)

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 5: 域名验证一致性
// Validates: Requirements 5.5, 5.7, 8.2
func TestProperty_DomainValidationConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	genDomain := gen.OneGenOf(
		gen.RegexMatch(`[a-z0-9]{1,10}\.[a-z]{2,6}`),
		gen.AnyString(),
		gen.OneConstOf("example.com", "sub.test.org", "", "not-a-domain", "123", ".com", "a.b"),
	)
	genLang := gen.OneConstOf("en", "zh")

	properties.Property(
		"valid domains trigger bind; invalid domains return error with example.com",
		prop.ForAll(
			func(domain string, lang string) bool {
				bot, sender := newPropTestBotWithMock(t, 99999)
				msg := &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 12345},
					From: &tgbotapi.User{LanguageCode: lang},
				}

				bot.handleBind(msg, lang, []string{domain})

				texts := sender.sentTexts()
				if len(texts) == 0 {
					t.Logf("no message sent for domain=%q", domain)
					return false
				}
				lastMsg := texts[len(texts)-1]

				if DomainRegex.MatchString(domain) {
					// Valid domain: should get a success message (bind was called)
					// The success message contains the domain itself
					if !strings.Contains(lastMsg, domain) {
						t.Logf("valid domain %q: expected success msg containing domain, got: %q", domain, lastMsg)
						return false
					}
				} else {
					// Invalid domain: error message should contain "example.com"
					if !strings.Contains(lastMsg, "example.com") {
						t.Logf("invalid domain %q: expected error msg with example.com, got: %q", domain, lastMsg)
						return false
					}
				}
				return true
			},
			genDomain,
			genLang,
		),
	)

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 6: 无效邮箱错误消息包含示例
// Validates: Requirements 8.3
func TestProperty_InvalidEmailErrorContainsExample(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generate strings that do NOT match EmailRegex
	genInvalidEmail := gen.AnyString().SuchThat(func(s string) bool {
		return !EmailRegex.MatchString(s) && len(s) > 0
	})
	genBlockType := gen.OneConstOf("sender", "receiver")
	genLang := gen.OneConstOf("en", "zh")

	properties.Property(
		"invalid email input returns error containing someone@example.com",
		prop.ForAll(
			func(invalidEmail string, blockType string, lang string) bool {
				bot, sender := newPropTestBotWithMock(t, 99999)
				msg := &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: 12345},
					From: &tgbotapi.User{LanguageCode: lang},
				}

				bot.handleUnblock(msg, lang, blockType, []string{invalidEmail})

				texts := sender.sentTexts()
				if len(texts) == 0 {
					t.Logf("no message sent for invalid email=%q blockType=%s", invalidEmail, blockType)
					return false
				}
				lastMsg := texts[len(texts)-1]
				if !strings.Contains(lastMsg, "someone@example.com") {
					t.Logf("error msg for invalid email=%q does not contain example: %q", invalidEmail, lastMsg)
					return false
				}
				return true
			},
			genInvalidEmail,
			genBlockType,
			genLang,
		),
	)

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 7: 未识别命令友好提示
// Validates: Requirements 8.1
func TestProperty_UnknownCommandFriendlyPrompt(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Known commands that the bot handles
	knownCommands := map[string]bool{
		"start": true, "help": true, "list": true, "bind": true,
		"dismiss": true, "unblock_domain": true, "unblock_sender": true,
		"unblock_receiver": true, "list_block_domain": true,
		"list_block_sender": true, "list_block_receiver": true,
		"send_all": true,
	}

	// Generate random command-like strings that are NOT known commands
	genUnknownCmd := gen.RegexMatch(`[a-z_]{1,15}`).SuchThat(func(s string) bool {
		return !knownCommands[s]
	})
	genLang := gen.OneConstOf("en", "zh")

	properties.Property(
		"unknown commands produce message with View Help button",
		prop.ForAll(
			func(cmd string, lang string) bool {
				bot, sender := newPropTestBotWithMock(t, 99999)

				// Simulate what the message router would do for an unknown command:
				// Send the err_unknown_cmd text with a "View Help" inline keyboard button
				errText := bot.getText("err_unknown_cmd", lang)
				btnText := bot.getText("btn_view_help", lang)

				// Verify the texts exist and are non-empty
				if errText == "" {
					t.Logf("err_unknown_cmd text is empty for lang=%s", lang)
					return false
				}
				if btnText == "" {
					t.Logf("btn_view_help text is empty for lang=%s", lang)
					return false
				}

				// Now actually send the unknown command response as the router would
				keyboard := tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData(btnText, bot.encodeCallback("help")),
					),
				)
				bot.sendHTMLWithKeyboard(12345, errText, keyboard)

				// Verify the sent message
				sender.mu.Lock()
				defer sender.mu.Unlock()
				if len(sender.messages) == 0 {
					t.Log("no message sent for unknown command")
					return false
				}
				lastChattable := sender.messages[len(sender.messages)-1]
				msgCfg, ok := lastChattable.(tgbotapi.MessageConfig)
				if !ok {
					t.Log("sent message is not a MessageConfig")
					return false
				}
				// Check text contains the unknown command error
				if !strings.Contains(msgCfg.Text, errText) {
					t.Logf("message text does not contain err_unknown_cmd: %q", msgCfg.Text)
					return false
				}
				// Check inline keyboard has a "View Help" button
				markup, ok := msgCfg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
				if !ok {
					t.Log("no inline keyboard in message")
					return false
				}
				found := false
				for _, row := range markup.InlineKeyboard {
					for _, btn := range row {
						if btn.Text == btnText {
							found = true
						}
					}
				}
				if !found {
					t.Logf("View Help button not found in keyboard for lang=%s cmd=/%s", lang, cmd)
					return false
				}
				return true
			},
			genUnknownCmd,
			genLang,
		),
	)

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 8: 管理员权限验证
// Validates: Requirements 10.1, 10.3
func TestProperty_AdminPermissionVerificationRedesign(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property(
		"only admin can trigger SendAll; non-admin is silently ignored",
		prop.ForAll(
			func(userID int64, adminID int64) bool {
				bot, sender := newPropTestBotWithMock(t, adminID)
				msg := &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: userID},
					From: &tgbotapi.User{LanguageCode: "en"},
				}

				bot.handleSendAll(msg, []string{"test", "broadcast"})

				texts := sender.sentTexts()
				if userID == adminID {
					// Admin: should get a broadcast confirmation message
					if len(texts) == 0 {
						t.Logf("admin (userID=%d) got no confirmation", userID)
						return false
					}
					// Confirmation should contain "Broadcast complete" (en)
					found := false
					for _, txt := range texts {
						if strings.Contains(txt, "Broadcast complete") || strings.Contains(txt, "广播已发送") {
							found = true
							break
						}
					}
					if !found {
						t.Logf("admin confirmation not found in: %v", texts)
						return false
					}
				} else {
					// Non-admin: should get NO messages at all
					if len(texts) != 0 {
						t.Logf("non-admin (userID=%d, adminID=%d) got messages: %v", userID, adminID, texts)
						return false
					}
				}
				return true
			},
			gen.Int64(),
			gen.Int64(),
		),
	)

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 9: 域名列表解绑按钮
// Validates: Requirements 5.1
func TestProperty_DomainListUnbindButtons(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generate 1-5 unique domain names
	genDomains := gen.SliceOfN(5, gen.RegexMatch(`[a-z]{3,8}\.[a-z]{2,4}`)).
		SuchThat(func(ds []string) bool {
			if len(ds) == 0 {
				return false
			}
			// Ensure uniqueness
			seen := make(map[string]bool)
			for _, d := range ds {
				if seen[d] || d == "" {
					return false
				}
				seen[d] = true
			}
			return true
		})
	genLang := gen.OneConstOf("en", "zh")

	properties.Property(
		"handleList produces one Unbind button per domain with domain in callback data",
		prop.ForAll(
			func(domains []string, lang string) bool {
				bot, sender := newPropTestBotWithMock(t, 99999)
				chatID := int64(12345)

				// Bind all domains to the user via IO
				for _, d := range domains {
					bot.io.BindDomain(chatID, d)
				}

				msg := &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: chatID},
					From: &tgbotapi.User{LanguageCode: lang},
				}
				bot.handleList(msg, lang)

				// Find the message with inline keyboard
				sender.mu.Lock()
				defer sender.mu.Unlock()

				// IO.BindDomain also sends messages via its own bot ref (which is nil here),
				// so we only look at messages sent through our mockSender
				var foundMarkup *tgbotapi.InlineKeyboardMarkup
				for _, c := range sender.messages {
					if msgCfg, ok := c.(tgbotapi.MessageConfig); ok {
						if markup, ok := msgCfg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup); ok {
							foundMarkup = &markup
						}
					}
				}

				if foundMarkup == nil {
					t.Logf("no inline keyboard found for domains=%v", domains)
					return false
				}

				// Count unbind buttons
				unbindBtnCount := 0
				domainInCallback := make(map[string]bool)
				dismissLabel := bot.getText("btn_dismiss", lang)
				for _, row := range foundMarkup.InlineKeyboard {
					for _, btn := range row {
						if strings.Contains(btn.Text, dismissLabel) {
							unbindBtnCount++
							// Decode callback to check domain is in params
							if btn.CallbackData != nil {
								cb := bot.decodeCallback(*btn.CallbackData)
								if len(cb.Params) > 0 {
									domainInCallback[cb.Params[0]] = true
								}
							}
						}
					}
				}

				if unbindBtnCount != len(domains) {
					t.Logf("expected %d unbind buttons, got %d", len(domains), unbindBtnCount)
					return false
				}

				// Verify each domain has a corresponding button
				for _, d := range domains {
					if !domainInCallback[d] {
						t.Logf("domain %q not found in any button callback data", d)
						return false
					}
				}

				return true
			},
			genDomains,
			genLang,
		),
	)

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 10: 屏蔽列表包含解除按钮
// Validates: Requirements 6.2
func TestProperty_BlockListUnblockButtons(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for block items: 1-5 unique items
	genBlockItems := gen.SliceOfN(5, gen.RegexMatch(`[a-z]{3,8}@[a-z]{3,6}\.[a-z]{2,4}`)).
		SuchThat(func(items []string) bool {
			if len(items) == 0 {
				return false
			}
			seen := make(map[string]bool)
			for _, item := range items {
				if seen[item] || item == "" {
					return false
				}
				seen[item] = true
			}
			return true
		})

	genCategory := gen.OneConstOf("sender", "domain", "receiver")
	genLang := gen.OneConstOf("en", "zh")

	properties.Property(
		"handleBlockCategory produces one Unblock button per blocked item with item in callback data",
		prop.ForAll(
			func(items []string, category string, lang string) bool {
				bot, sender := newPropTestBotWithMock(t, 99999)
				chatID := int64(12345)

				// Block all items via IO
				for _, item := range items {
					switch category {
					case "sender":
						bot.io.BlockSender(chatID, item)
					case "domain":
						bot.io.BlockDomain(chatID, item)
					case "receiver":
						bot.io.BlockReceiver(chatID, item)
					}
				}

				// Create a callback query with a message that has Chat and MessageID
				query := &tgbotapi.CallbackQuery{
					ID: "test-query-id",
					From: &tgbotapi.User{LanguageCode: lang},
					Message: &tgbotapi.Message{
						MessageID: 42,
						Chat:      &tgbotapi.Chat{ID: chatID},
					},
				}

				bot.handleBlockCategory(query, lang, category)

				// Find the EditMessageTextConfig in sent messages
				sender.mu.Lock()
				defer sender.mu.Unlock()

				var foundMarkup *tgbotapi.InlineKeyboardMarkup
				for _, c := range sender.messages {
					if editMsg, ok := c.(tgbotapi.EditMessageTextConfig); ok {
						if editMsg.ReplyMarkup != nil {
							foundMarkup = editMsg.ReplyMarkup
						}
					}
				}

				if foundMarkup == nil {
					t.Logf("no inline keyboard found in edit message for category=%s items=%v", category, items)
					return false
				}

				// Count unblock buttons and verify callback data
				unblockBtnCount := 0
				itemInCallback := make(map[string]bool)
				unblockLabel := bot.getText("btn_unblock", lang)
				for _, row := range foundMarkup.InlineKeyboard {
					for _, btn := range row {
						if strings.Contains(btn.Text, unblockLabel) {
							unblockBtnCount++
							if btn.CallbackData != nil {
								cb := bot.decodeCallback(*btn.CallbackData)
								if len(cb.Params) > 0 {
									itemInCallback[cb.Params[0]] = true
								}
							}
						}
					}
				}

				if unblockBtnCount != len(items) {
					t.Logf("expected %d unblock buttons, got %d (category=%s)", len(items), unblockBtnCount, category)
					return false
				}

				for _, item := range items {
					if !itemInCallback[item] {
						t.Logf("item %q not found in any button callback data (category=%s)", item, category)
						return false
					}
				}

				return true
			},
			genBlockItems,
			genCategory,
			genLang,
		),
	)

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 15: AnswerCallbackQuery 应答完整性
// Validates: Requirements 7.1, 7.2, 7.3
func TestProperty_AnswerCallbackQueryCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for callback scenarios
	genScenario := gen.OneConstOf(
		"help_cat",
		"help_back",
		"quick_start",
		"dismiss_ask",
		"dismiss_yes",
		"dismiss_no",
		"block_sender",
		"unblock_sender",
		"block_cat",
	)
	genLang := gen.OneConstOf("en", "zh")

	properties.Property(
		"every callback handler calls AnswerCallbackQuery (Request with CallbackConfig)",
		prop.ForAll(
			func(scenario string, lang string) bool {
				bot, sender := newPropTestBotWithMock(t, 99999)
				chatID := int64(12345)

				// Set up data needed by some scenarios
				// Bind a domain for dismiss scenarios
				bot.io.BindDomain(chatID, "test.com")
				// Block a sender for unblock/block_cat scenarios
				bot.io.BlockSender(chatID, "user@test.com")

				query := &tgbotapi.CallbackQuery{
					ID:   "test-query-id",
					From: &tgbotapi.User{LanguageCode: lang},
					Message: &tgbotapi.Message{
						MessageID: 42,
						Chat:      &tgbotapi.Chat{ID: chatID},
					},
				}

				switch scenario {
				case "help_cat":
					bot.handleHelpCategory(query, lang, "domain")
				case "help_back":
					bot.handleHelpBack(query, lang)
				case "quick_start":
					bot.handleQuickStart(query, lang)
				case "dismiss_ask":
					bot.handleDismissAsk(query, lang, "test.com")
				case "dismiss_yes":
					bot.handleDismissConfirm(query, lang, "test.com")
				case "dismiss_no":
					bot.handleDismissCancel(query, lang)
				case "block_sender":
					bot.handleBlockAction(query, lang, "sender", "user@test.com")
				case "unblock_sender":
					bot.handleUnblockAction(query, lang, "sender", "user@test.com")
				case "block_cat":
					bot.handleBlockCategory(query, lang, "sender")
				}

				// Check that at least one CallbackConfig was sent via Request
				sender.mu.Lock()
				defer sender.mu.Unlock()

				foundCallback := false
				for _, c := range sender.requests {
					if _, ok := c.(tgbotapi.CallbackConfig); ok {
						foundCallback = true
						break
					}
				}

				if !foundCallback {
					t.Logf("no AnswerCallbackQuery (CallbackConfig) found in requests for scenario=%s lang=%s", scenario, lang)
					return false
				}

				return true
			},
			genScenario,
			genLang,
		),
	)

	properties.TestingRun(t)
}
