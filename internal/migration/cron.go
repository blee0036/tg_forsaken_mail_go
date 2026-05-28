package migration

import (
	"log"
	"time"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
	"go-version-rewrite/internal/telegram"
)

// CronScheduler sends daily migration alerts at 0:00 server local time.
// It stops automatically when close_old_date is reached.
type CronScheduler struct {
	config      *config.Config
	ioModule    *io.IO
	db          *db.DB
	oldBots     []*telegram.Bot
	alertSender *AlertSender
	stop        chan struct{}
}

// NewCronScheduler creates a new CronScheduler.
func NewCronScheduler(cfg *config.Config, ioModule *io.IO, database *db.DB, oldBots []*telegram.Bot, alertSender *AlertSender) *CronScheduler {
	return &CronScheduler{
		config:      cfg,
		ioModule:    ioModule,
		db:          database,
		oldBots:     oldBots,
		alertSender: alertSender,
		stop:        make(chan struct{}),
	}
}

// Start launches the daily cron loop. It blocks until Stop() is called or
// close_old_date is reached. Intended to be run in a goroutine.
func (c *CronScheduler) Start() {
	for {
		// Calculate duration until next 0:00 local time
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.Local)
		duration := next.Sub(now)

		timer := time.NewTimer(duration)
		select {
		case <-c.stop:
			timer.Stop()
			return
		case <-timer.C:
			// Check if close_old_date is reached
			if c.isCloseOldDateReached() {
				log.Println("[cron] close_old_date reached, stopping cron scheduler")
				return
			}
			c.RunOnce()
		}
	}
}

// Stop signals the cron scheduler to stop.
func (c *CronScheduler) Stop() {
	select {
	case <-c.stop:
		// Already closed
	default:
		close(c.stop)
	}
}

// RunOnce executes one round of migration alert sending.
// It fetches all unique tgIDs from domain_tg, then for each tgID,
// sends an alert through all old bots. Each tgID receives exactly one alert
// (not one per domain binding).
func (c *CronScheduler) RunOnce() {
	if !c.alertSender.HasAlert() {
		return
	}

	tgIDs, err := c.db.SelectAllUniqueTgIDs()
	if err != nil {
		log.Printf("[cron] failed to get unique tgIDs: %v", err)
		return
	}

	for _, tgID := range tgIDs {
		lang := c.ioModule.GetUserLang(tgID)

		for _, bot := range c.oldBots {
			c.alertSender.SendAlert(bot.GetSender(), tgID, lang)
		}
	}
}

// isCloseOldDateReached checks if the current local date has reached or passed
// the configured close_old_date.
func (c *CronScheduler) isCloseOldDateReached() bool {
	if c.config.CloseOldDateParsed == nil {
		return false
	}

	closeDate := *c.config.CloseOldDateParsed
	now := time.Now()
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	closeDateLocal := time.Date(closeDate.Year(), closeDate.Month(), closeDate.Day(), 0, 0, 0, 0, time.Local)

	return !nowDate.Before(closeDateLocal)
}
