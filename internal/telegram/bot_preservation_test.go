package telegram

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: high-cpu-usage-fix, Property 5: Telegram Bot Command Processing Preservation
// **Validates: Requirements 3.2, 3.5**
//
// For any valid command input, parseCommand and detectLanguageStatic must produce
// consistent results. This captures the baseline behavior that must be preserved
// after the Telegram Bot reconnection backoff changes.

// TestProperty_Preservation_ParseCommandDeterminism verifies that parseCommand
// produces identical results when called multiple times with the same input.
func TestProperty_Preservation_ParseCommandDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for command-like strings
	genCommandText := gen.OneGenOf(
		// Valid commands with args
		gen.RegexMatch(`/[a-z_]{1,20}( [a-zA-Z0-9._@\-]{1,20}){0,3}`),
		// Just a command
		gen.RegexMatch(`/[a-z_]{1,15}`),
		// Random text (non-command)
		gen.AnyString(),
		// Known commands
		gen.OneConstOf(
			"/start", "/help", "/list",
			"/bind example.com", "/dismiss example.com",
			"/unblock_domain spam.com",
			"/unblock_sender bad@evil.com",
			"/unblock_receiver me@private.com",
			"/list_block_domain", "/list_block_sender", "/list_block_receiver",
			"/send_all hello world",
			"/lang",
		),
	)

	properties.Property("parseCommand is deterministic for any input", prop.ForAll(
		func(text string) bool {
			r1 := parseCommand(text)
			r2 := parseCommand(text)

			if r1.Command != r2.Command {
				t.Logf("Command mismatch for %q: %q vs %q", text, r1.Command, r2.Command)
				return false
			}
			if len(r1.Args) != len(r2.Args) {
				t.Logf("Args length mismatch for %q: %d vs %d", text, len(r1.Args), len(r2.Args))
				return false
			}
			for i := range r1.Args {
				if r1.Args[i] != r2.Args[i] {
					t.Logf("Args[%d] mismatch for %q: %q vs %q", i, text, r1.Args[i], r2.Args[i])
					return false
				}
			}
			if r1.Raw != r2.Raw {
				t.Logf("Raw mismatch for %q: %q vs %q", text, r1.Raw, r2.Raw)
				return false
			}

			return true
		},
		genCommandText,
	))

	properties.TestingRun(t)
}

// TestProperty_Preservation_ParseCommandSlashStripping verifies that parseCommand
// correctly strips the leading "/" from command names for all valid commands.
func TestProperty_Preservation_ParseCommandSlashStripping(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	genCmdName := gen.OneConstOf(
		"start", "help", "list", "bind", "dismiss",
		"unblock_domain", "unblock_sender", "unblock_receiver",
		"list_block_domain", "list_block_sender", "list_block_receiver",
		"send_all", "lang",
	)
	genArg := gen.RegexMatch(`[a-zA-Z0-9._@\-]{1,20}`)

	properties.Property("parseCommand strips leading / and extracts command name correctly", prop.ForAll(
		func(cmdName, arg string) bool {
			// With slash prefix
			text := "/" + cmdName + " " + arg
			result := parseCommand(text)

			if result.Command != cmdName {
				t.Logf("Command mismatch: expected %q, got %q (input: %q)", cmdName, result.Command, text)
				return false
			}
			if len(result.Args) != 1 || result.Args[0] != arg {
				t.Logf("Args mismatch: expected [%q], got %v (input: %q)", arg, result.Args, text)
				return false
			}

			return true
		},
		genCmdName,
		genArg,
	))

	properties.Property("parseCommand without slash still works (treats first word as command)", prop.ForAll(
		func(cmdName string) bool {
			result := parseCommand(cmdName)
			if result.Command != cmdName {
				t.Logf("Command mismatch: expected %q, got %q", cmdName, result.Command)
				return false
			}
			return true
		},
		genCmdName,
	))

	properties.TestingRun(t)
}

// TestProperty_Preservation_DetectLanguageStaticDeterminism verifies that
// detectLanguageStatic produces consistent results for any language code.
func TestProperty_Preservation_DetectLanguageStaticDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	genLangCode := gen.OneGenOf(
		// zh-prefixed codes
		gen.OneConstOf("zh", "zh-hans", "zh-cn", "zh-tw", "zh-hant", "ZH", "Zh", "ZH-CN"),
		// Non-zh codes
		gen.OneConstOf("", "en", "en-us", "ja", "fr", "de", "ko", "es", "ru"),
		// Random strings
		gen.AnyString(),
	)

	properties.Property("detectLanguageStatic is deterministic and returns only en or zh", prop.ForAll(
		func(langCode string) bool {
			user := &tgbotapi.User{LanguageCode: langCode}
			r1 := detectLanguageStatic(user)
			r2 := detectLanguageStatic(user)

			// Must be deterministic
			if r1 != r2 {
				t.Logf("Non-deterministic for %q: %q vs %q", langCode, r1, r2)
				return false
			}

			// Must return only "en" or "zh"
			if r1 != "en" && r1 != "zh" {
				t.Logf("Invalid result %q for langCode %q", r1, langCode)
				return false
			}

			// Verify correctness: zh prefix → "zh", otherwise → "en"
			isZh := strings.HasPrefix(strings.ToLower(langCode), "zh")
			if isZh && r1 != "zh" {
				t.Logf("Expected zh for %q, got %q", langCode, r1)
				return false
			}
			if !isZh && r1 != "en" {
				t.Logf("Expected en for %q, got %q", langCode, r1)
				return false
			}

			return true
		},
		genLangCode,
	))

	properties.Property("detectLanguageStatic with nil user returns en", prop.ForAll(
		func(_ int) bool {
			result := detectLanguageStatic(nil)
			return result == "en"
		},
		gen.Const(0),
	))

	properties.TestingRun(t)
}

// TestProperty_Preservation_ParseCommandEmptyInput verifies that parseCommand
// handles empty and whitespace-only inputs correctly.
func TestProperty_Preservation_ParseCommandEmptyInput(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	genWhitespace := gen.RegexMatch(`[ \t\n]{0,10}`)

	properties.Property("parseCommand with empty/whitespace input returns empty command", prop.ForAll(
		func(ws string) bool {
			result := parseCommand(ws)
			trimmed := strings.TrimSpace(ws)
			if trimmed == "" {
				// Empty input should produce empty command
				if result.Command != "" {
					t.Logf("Expected empty command for whitespace input %q, got %q", ws, result.Command)
					return false
				}
			}
			return true
		},
		genWhitespace,
	))

	properties.TestingRun(t)
}

// TestProperty_Preservation_CommandRoutingConsistency verifies that known commands
// are correctly identified and routed by parseCommand.
func TestProperty_Preservation_CommandRoutingConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	knownCommands := []string{
		"start", "help", "list", "bind", "dismiss",
		"unblock_domain", "unblock_sender", "unblock_receiver",
		"list_block_domain", "list_block_sender", "list_block_receiver",
		"send_all", "lang",
	}

	properties.Property("known commands are correctly parsed regardless of args", prop.ForAll(
		func(cmdIdx int, argCount int) bool {
			cmd := knownCommands[cmdIdx%len(knownCommands)]
			text := "/" + cmd
			expectedArgCount := 0

			if argCount > 0 {
				args := []string{"arg1", "arg2", "arg3"}
				actualArgs := args[:argCount]
				text += " " + strings.Join(actualArgs, " ")
				expectedArgCount = argCount
			}

			result := parseCommand(text)

			if result.Command != cmd {
				t.Logf("Command mismatch: expected %q, got %q (input: %q)", cmd, result.Command, text)
				return false
			}
			if len(result.Args) != expectedArgCount {
				t.Logf("Args count mismatch: expected %d, got %d (input: %q)", expectedArgCount, len(result.Args), text)
				return false
			}

			return true
		},
		gen.IntRange(0, len(knownCommands)-1),
		gen.IntRange(0, 3),
	))

	properties.TestingRun(t)
}
