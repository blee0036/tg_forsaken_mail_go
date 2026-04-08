package main

import (
	"log"
	"os"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
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

	// 5. Set bot on IO
	ioModule.SetBot(bot.GetBotAPI())

	// 6. Wire up upload function if upload_url is configured
	if cfg.UploadURL != "" {
		uploader := upload.New(cfg.UploadURL, cfg.UploadToken)
		ioModule.UploadHTMLFunc = uploader.UploadHTML
		log.Println("HTML upload configured")
	}

	// 7. Create SMTP server, register handler, then start in goroutine
	smtpServer := smtp.New(cfg.Mailin.Host, cfg.Mailin.Port)
	smtpServer.OnMessage(ioModule.HandleMail)
	log.Println("Mail handler registered")

	go func() {
		if err := smtpServer.Start(); err != nil {
			log.Printf("SMTP server error: %v", err)
			os.Exit(1)
		}
	}()
	log.Printf("SMTP server starting on %s:%d", cfg.Mailin.Host, cfg.Mailin.Port)

	// 9. Start bot listening (blocking)
	log.Println("Bot is running, listening for Telegram messages...")
	bot.Start()
}
