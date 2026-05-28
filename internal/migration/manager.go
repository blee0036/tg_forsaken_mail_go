package migration

import (
	"fmt"
	"log"
	"time"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/io"
	"go-version-rewrite/internal/telegram"
)

// MigrationManager manages multi-bot migration logic.
// It creates old Bot instances, starts their listeners, and provides
// active delivery endpoints based on close_old_date gating.
type MigrationManager struct {
	config      *config.Config
	ioModule    *io.IO
	newBot      *telegram.Bot
	oldBots     []*telegram.Bot
	alertSender *AlertSender
	stopCh      chan struct{}
	cron        *CronScheduler // nil if cron not started
}

// NewMigrationManager creates a MigrationManager.
// For each old token in cfg.OldTelegramBotTokens, it calls telegram.NewWithOptions
// to create a full Bot instance with an AfterInteraction hook that sends alerts.
// Invalid tokens are logged and skipped.
func NewMigrationManager(cfg *config.Config, ioModule *io.IO, newBot *telegram.Bot) *MigrationManager {
	alertSender := NewAlertSender(cfg)

	mm := &MigrationManager{
		config:      cfg,
		ioModule:    ioModule,
		newBot:      newBot,
		oldBots:     make([]*telegram.Bot, 0, len(cfg.OldTelegramBotTokens)),
		alertSender: alertSender,
		stopCh:      make(chan struct{}),
	}

	for _, token := range cfg.OldTelegramBotTokens {
		opts := telegram.BotOptions{
			Token: token,
			IsOld: true,
		}

		// Inject AfterInteraction hook: send alert after each interaction
		if alertSender.HasAlert() {
			opts.AfterInteraction = func(sender telegram.BotSender, tgID int64, lang string) {
				// telegram.BotSender and io.TelegramSender are structurally identical interfaces
				alertSender.SendAlert(sender, tgID, lang)
			}
		}

		bot, err := telegram.NewWithOptions(cfg, ioModule, opts)
		if err != nil {
			log.Printf("[migration] failed to create old bot (token=%s...): %v, skipping",
				truncateToken(token), err)
			continue
		}

		mm.oldBots = append(mm.oldBots, bot)
	}

	return mm
}

// Start launches a goroutine for each old Bot's Start() method (which blocks).
// This method is non-blocking; each old Bot runs in its own goroutine.
func (m *MigrationManager) Start() {
	for _, bot := range m.oldBots {
		go func(b *telegram.Bot) {
			b.Start()
		}(bot)
	}
}

// Stop signals all goroutines to stop. Stops all old Bot listeners,
// the CronScheduler (if running), and closes the stop channel.
func (m *MigrationManager) Stop() {
	select {
	case <-m.stopCh:
		// Already closed
	default:
		close(m.stopCh)
	}
	// Stop cron scheduler if set
	if m.cron != nil {
		m.cron.Stop()
	}
	// Stop all old bot listeners
	for _, bot := range m.oldBots {
		bot.Stop()
	}
}

// SetCronScheduler registers the CronScheduler so that Stop() can shut it down.
func (m *MigrationManager) SetCronScheduler(c *CronScheduler) {
	m.cron = c
}

// ActiveMailEndpoints returns the list of DeliveryEndpoints that should
// participate in mail delivery at the given time.
// Always includes the New_Bot. Includes all Old_Bots only when close_old_date
// has not been reached.
func (m *MigrationManager) ActiveMailEndpoints(now time.Time) []io.DeliveryEndpoint {
	endpoints := make([]io.DeliveryEndpoint, 0, 1+len(m.oldBots))

	// Always include the new bot
	endpoints = append(endpoints, io.DeliveryEndpoint{
		Name:   "new_bot",
		Sender: m.newBot.GetSender(),
		IsOld:  false,
	})

	// Include old bots only if close_old_date has not been reached
	if !m.isCloseOldDateReachedAt(now) {
		for i, bot := range m.oldBots {
			endpoints = append(endpoints, io.DeliveryEndpoint{
				Name:   fmt.Sprintf("old_bot_%d", i),
				Sender: bot.GetSender(),
				IsOld:  true,
			})
		}
	}

	return endpoints
}

// IsCloseOldDateReached checks if the current local date has reached or passed
// the configured close_old_date. Returns false if close_old_date is not configured.
func (m *MigrationManager) IsCloseOldDateReached() bool {
	return m.isCloseOldDateReachedAt(time.Now())
}

// isCloseOldDateReachedAt checks if the given time's local date has reached
// or passed the configured close_old_date.
func (m *MigrationManager) isCloseOldDateReachedAt(now time.Time) bool {
	if m.config.CloseOldDateParsed == nil {
		return false
	}

	// Compare by local date (year, month, day), not exact time
	closeDate := *m.config.CloseOldDateParsed
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	closeDateLocal := time.Date(closeDate.Year(), closeDate.Month(), closeDate.Day(), 0, 0, 0, 0, time.Local)

	return !nowDate.Before(closeDateLocal)
}

// AlertFunc returns the AlertSender's callback as an io.AlertFunc.
// Returns nil if no alert messages are configured.
func (m *MigrationManager) AlertFunc() io.AlertFunc {
	return m.alertSender.AsAlertFunc()
}

// OldBots returns the slice of old Bot instances (for use by CronScheduler etc.)
func (m *MigrationManager) OldBots() []*telegram.Bot {
	return m.oldBots
}

// StopCh returns the stop channel for coordination with other components.
func (m *MigrationManager) StopCh() <-chan struct{} {
	return m.stopCh
}

// truncateToken returns the first 10 characters of a token for safe logging.
func truncateToken(token string) string {
	if len(token) <= 10 {
		return token
	}
	return token[:10]
}
