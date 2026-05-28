package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
	"go-version-rewrite/internal/migration"
	"go-version-rewrite/internal/smtp"
	"go-version-rewrite/internal/telegram"
	"go-version-rewrite/internal/upload"
)

func main() {
	// 1. Load config.json
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		os.Exit(1)
	}
	log.Println("Config loaded successfully")

	// 2. Initialize database
	database, err := db.New("mail.db")
	if err != nil {
		log.Printf("Failed to initialize database: %v", err)
		os.Exit(1)
	}
	defer database.Close()
	log.Println("Database initialized successfully")

	// 3. Create IO instance and Init() — load domain mappings from DB
	ioModule := io.New(database, cfg)
	if err := ioModule.Init(); err != nil {
		log.Printf("Failed to initialize IO module: %v", err)
		os.Exit(1)
	}
	log.Println("IO module initialized successfully")

	// 4. Create Telegram Bot instance
	bot, err := telegram.New(cfg, ioModule)
	if err != nil {
		log.Printf("Failed to create Telegram bot: %v", err)
		os.Exit(1)
	}
	log.Println("Telegram bot created successfully")

	// 5. Set bot on IO (SetBot accepts TelegramSender interface)
	ioModule.SetBot(bot.GetBotAPI())

	// 6. Wire up upload function if upload_url is configured
	if cfg.UploadURL != "" {
		uploader := upload.New(cfg.UploadURL, cfg.UploadToken)
		ioModule.UploadHTMLFunc = uploader.UploadHTML
		log.Println("HTML upload configured")
	}

	// 7. Create SMTP server
	smtpServer := smtp.New(cfg.Mailin.Host, cfg.Mailin.Port)

	// 8. Migration setup: create MigrationManager if old tokens are configured
	var migrationManager *migration.MigrationManager
	if len(cfg.OldTelegramBotTokens) > 0 {
		migrationManager = migration.NewMigrationManager(cfg, ioModule, bot)
		log.Printf("Migration manager created with %d old bot token(s)", len(cfg.OldTelegramBotTokens))

		// Register SMTP handler with multi-delivery path
		smtpServer.OnMessage(func(mail *smtp.ParsedMail) {
			endpoints := migrationManager.ActiveMailEndpoints(time.Now())
			ioModule.HandleMailMulti(mail, endpoints, migrationManager.AlertFunc(), time.Now())
		})
		log.Println("Mail handler registered (multi-bot delivery)")

		// Start CronScheduler if alert is configured and close_old_date not yet reached
		alertSender := migration.NewAlertSender(cfg)
		if alertSender.HasAlert() && !migrationManager.IsCloseOldDateReached() {
			cronScheduler := migration.NewCronScheduler(cfg, ioModule, database, migrationManager.OldBots(), alertSender)
			migrationManager.SetCronScheduler(cronScheduler)
			go cronScheduler.Start()
			log.Println("Cron scheduler started for daily migration alerts")
		}

		// Start MigrationManager (non-blocking: launches old bot goroutines)
		migrationManager.Start()
		log.Println("Migration manager started (old bots listening)")
	} else {
		// No migration: register original single-bot handler
		smtpServer.OnMessage(ioModule.HandleMail)
		log.Println("Mail handler registered")
	}

	// 9. Start SMTP server in goroutine
	go func() {
		if err := smtpServer.Start(); err != nil {
			log.Printf("SMTP server error: %v", err)
			os.Exit(1)
		}
	}()
	log.Printf("SMTP server starting on %s:%d", cfg.Mailin.Host, cfg.Mailin.Port)

	// 10. Start pprof server for debugging (localhost only)
	go func() {
		log.Println("pprof server listening on :6060")
		if err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
			log.Printf("pprof server error: %v", err)
		}
	}()

	// 11. Start bot listening (blocking)
	log.Println("Bot is running, listening for Telegram messages...")
	bot.Start()
}
