package telegram

import (
	"os"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
)

// identityTrackingSender is a mock sender that records its own identity string
// on every Send/Request call, allowing us to verify which sender was used.
type identityTrackingSender struct {
	mu       sync.Mutex
	identity string
	sends    []string // records identity for each Send call
	requests []string // records identity for each Request call
}

func newIdentityTrackingSender(identity string) *identityTrackingSender {
	return &identityTrackingSender{identity: identity}
}

func (s *identityTrackingSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sends = append(s.sends, s.identity)
	return tgbotapi.Message{}, nil
}

func (s *identityTrackingSender) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, s.identity)
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func (s *identityTrackingSender) allCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, 0, len(s.sends)+len(s.requests))
	result = append(result, s.sends...)
	result = append(result, s.requests...)
	return result
}

func (s *identityTrackingSender) totalCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sends) + len(s.requests)
}

// interactionType represents the type of user interaction to simulate.
type interactionType int

const (
	interactionMessage  interactionType = iota
	interactionCallback interactionType = iota
)

// createBotWithTrackingSender creates a Bot instance with an identity-tracking sender
// and a real IO module backed by a temp DB.
func createBotWithTrackingSender(t *testing.T, identity string, database *db.DB) (*Bot, *identityTrackingSender) {
	t.Helper()

	cfg := &config.Config{
		MailDomain: "test.example.com",
		AdminTgID:  99999,
	}
	ioModule := io.New(database, cfg)

	sender := newIdentityTrackingSender(identity)
	bot := NewForTest(cfg, ioModule, sender)

	return bot, sender
}

// Feature: bot-migration, Property 4: 交互响应发送者身份一致性
// Validates: Requirements 4.3
func TestProperty_InteractionResponseSenderIdentity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for interaction type (message or callback)
	genInteractionType := gen.IntRange(0, 1).Map(func(v int) interactionType {
		return interactionType(v)
	})

	// Generator for bot identity index (simulating multiple bots)
	genBotIndex := gen.IntRange(0, 3) // 0=new bot, 1-3=old bots

	// Generator for chat ID
	genChatID := gen.Int64Range(1, 999999)

	// Generator for command type (various commands that trigger IO interactions)
	genCommandType := gen.IntRange(0, 9)

	properties.Property(
		"all responses from an interaction go through the same sender that received it",
		prop.ForAll(
			func(interaction interactionType, botIdx int, chatID int64, cmdType int) bool {
				// Create a shared temp DB for all bots
				tmpFile, err := os.CreateTemp("", "test-sender-identity-*.db")
				if err != nil {
					t.Fatalf("failed to create temp db: %v", err)
				}
				tmpFile.Close()
				defer os.Remove(tmpFile.Name())

				database, err := db.New(tmpFile.Name())
				if err != nil {
					t.Fatalf("failed to create db: %v", err)
				}
				defer database.Close()

				// Create multiple bots with different identity-tracking senders
				botIdentities := []string{"new-bot", "old-bot-1", "old-bot-2", "old-bot-3"}
				bots := make([]*Bot, len(botIdentities))
				senders := make([]*identityTrackingSender, len(botIdentities))

				for i, identity := range botIdentities {
					bots[i], senders[i] = createBotWithTrackingSender(t, identity, database)
				}

				// Select the bot that "receives" this interaction
				selectedBot := bots[botIdx]
				selectedSender := senders[botIdx]
				expectedIdentity := botIdentities[botIdx]

				// Pre-bind a domain so some commands have data to work with
				selectedBot.io.BindDomainWith(selectedSender, chatID, "test-domain.example.com")

				// Clear the sender's tracking after setup
				selectedSender.mu.Lock()
				selectedSender.sends = nil
				selectedSender.requests = nil
				selectedSender.mu.Unlock()

				// Simulate the interaction based on type
				if interaction == interactionMessage {
					msg := &tgbotapi.Message{
						Chat: &tgbotapi.Chat{ID: chatID},
						From: &tgbotapi.User{LanguageCode: "en"},
					}

					// Execute different commands to exercise various IO paths
					switch cmdType % 10 {
					case 0:
						selectedBot.handleStart(msg, "en")
					case 1:
						selectedBot.handleHelp(msg, "en")
					case 2:
						selectedBot.handleBind(msg, "en", []string{"another.example.com"})
					case 3:
						selectedBot.handleBind(msg, "en", []string{}) // usage hint
					case 4:
						selectedBot.handleDismiss(msg, "en", []string{"test-domain.example.com"})
					case 5:
						selectedBot.handleList(msg, "en")
					case 6:
						selectedBot.handleListBlock(msg, "en", "sender")
					case 7:
						selectedBot.handleListBlock(msg, "en", "domain")
					case 8:
						selectedBot.handleListBlock(msg, "en", "receiver")
					case 9:
						selectedBot.handleUnblock(msg, "en", "domain", []string{"spam.com"})
					}
				} else {
					// Callback query interaction
					query := &tgbotapi.CallbackQuery{
						ID: "test-query-id",
						Message: &tgbotapi.Message{
							MessageID: 42,
							Chat:      &tgbotapi.Chat{ID: chatID},
						},
						From: &tgbotapi.User{LanguageCode: "en"},
					}

					switch cmdType % 6 {
					case 0:
						selectedBot.handleHelpCategory(query, "en", "domain")
					case 1:
						selectedBot.handleHelpBack(query, "en")
					case 2:
						selectedBot.handleDismissCancel(query, "en")
					case 3:
						selectedBot.handleBlockCategory(query, "en", "sender")
					case 4:
						selectedBot.handleQuickStart(query, "en")
					case 5:
						selectedBot.handleGoMain(query, "en")
					}
				}

				// Verify: all Send/Request calls went through the selected sender
				calls := selectedSender.allCalls()
				for i, identity := range calls {
					if identity != expectedIdentity {
						t.Logf("call %d used sender %q, expected %q (botIdx=%d, interaction=%d, cmdType=%d)",
							i, identity, expectedIdentity, botIdx, interaction, cmdType)
						return false
					}
				}

				// Verify: no other sender received any calls
				for i, sender := range senders {
					if i == botIdx {
						continue
					}
					otherCalls := sender.totalCalls()
					if otherCalls > 0 {
						t.Logf("sender %q (index %d) received %d unexpected calls (expected only %q to be used)",
							botIdentities[i], i, otherCalls, expectedIdentity)
						return false
					}
				}

				// Verify at least one call was made (the interaction produced a response)
				if selectedSender.totalCalls() == 0 {
					t.Logf("no calls made for interaction=%d, cmdType=%d, botIdx=%d", interaction, cmdType, botIdx)
					return false
				}

				return true
			},
			genInteractionType,
			genBotIndex,
			genChatID,
			genCommandType,
		),
	)

	properties.Property(
		"message command responses never leak to other bot senders",
		prop.ForAll(
			func(botIdx int, chatID int64, lang string) bool {
				// Create a shared temp DB
				tmpFile, err := os.CreateTemp("", "test-sender-leak-*.db")
				if err != nil {
					t.Fatalf("failed to create temp db: %v", err)
				}
				tmpFile.Close()
				defer os.Remove(tmpFile.Name())

				database, err := db.New(tmpFile.Name())
				if err != nil {
					t.Fatalf("failed to create db: %v", err)
				}
				defer database.Close()

				// Create 2 bots sharing the same IO/DB
				botIdentities := []string{"bot-A", "bot-B"}
				bots := make([]*Bot, 2)
				senders := make([]*identityTrackingSender, 2)

				for i, identity := range botIdentities {
					bots[i], senders[i] = createBotWithTrackingSender(t, identity, database)
				}

				activeIdx := botIdx % 2
				inactiveIdx := 1 - activeIdx
				activeBot := bots[activeIdx]
				activeSender := senders[activeIdx]
				inactiveSender := senders[inactiveIdx]

				// Bind a domain via the active bot
				activeBot.io.BindDomainWith(activeSender, chatID, "leak-test.example.com")

				// Clear tracking
				activeSender.mu.Lock()
				activeSender.sends = nil
				activeSender.requests = nil
				activeSender.mu.Unlock()

				// Simulate /start command on the active bot
				msg := &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: chatID},
					From: &tgbotapi.User{LanguageCode: lang},
				}
				activeBot.handleStart(msg, lang)

				// The inactive bot's sender should have zero calls
				if inactiveSender.totalCalls() > 0 {
					t.Logf("inactive sender %q got %d calls when active sender %q handled /start",
						botIdentities[inactiveIdx], inactiveSender.totalCalls(), botIdentities[activeIdx])
					return false
				}

				// The active sender should have at least one call
				if activeSender.totalCalls() == 0 {
					t.Logf("active sender %q got 0 calls for /start", botIdentities[activeIdx])
					return false
				}

				return true
			},
			gen.IntRange(0, 1),
			gen.Int64Range(1, 999999),
			gen.OneConstOf("en", "zh"),
		),
	)

	properties.TestingRun(t)
}
