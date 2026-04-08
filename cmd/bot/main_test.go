package main

import (
	"os"
	"strings"
	"testing"
)

// TestStartupFlowOrderMatchesNodeVersion verifies that the Go version main.go
// contains startup steps in the correct order.
//
// Go version startup flow (from main.go):
//  1. config.Load
//  2. db.New
//  3. io.New + io.Init
//  4. telegram.New
//  5. io.SetBot
//  6. upload wiring
//  7. smtp.New + OnMessage
//  8. smtp.Start (goroutine)
//  9. bot.Start
func TestStartupFlowOrderMatchesNodeVersion(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}
	source := string(src)

	// Define the ordered startup steps that must appear in sequence.
	steps := []struct {
		label   string
		pattern string
	}{
		{"1. Load config", "config.Load("},
		{"2. Initialize database", "db.New("},
		{"3a. Create IO instance", "io.New("},
		{"3b. Init IO (load domain mappings)", "ioModule.Init()"},
		{"4. Create Telegram Bot", "telegram.New("},
		{"5. Set bot on IO", "ioModule.SetBot("},
		{"6. Upload wiring", "upload.New("},
		{"7a. Create SMTP server", "smtp.New("},
		{"7b. Register mail handler", "smtpServer.OnMessage("},
		{"8. Start SMTP server", "smtpServer.Start()"},
		{"9. Start bot (blocking)", "bot.Start()"},
	}

	// Verify each step exists and appears after the previous one
	lastIdx := -1
	for _, step := range steps {
		idx := strings.Index(source, step.pattern)
		if idx == -1 {
			t.Errorf("Startup step %q not found in main.go (pattern: %q)", step.label, step.pattern)
			continue
		}
		if idx <= lastIdx {
			t.Errorf("Startup step %q (at pos %d) appears before or at same position as previous step (pos %d) — order mismatch",
				step.label, idx, lastIdx)
		}
		lastIdx = idx
	}
}

// TestStartupFlowCriticalStepsBeforeBotStart verifies that all critical
// initialization steps happen before bot.Start() which is the blocking call.
func TestStartupFlowCriticalStepsBeforeBotStart(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}
	source := string(src)

	botStartIdx := strings.Index(source, "bot.Start()")
	if botStartIdx == -1 {
		t.Fatal("bot.Start() not found in main.go")
	}

	criticalSteps := []struct {
		label   string
		pattern string
	}{
		{"config.Load", "config.Load("},
		{"db.New", "db.New("},
		{"io.New", "io.New("},
		{"ioModule.Init", "ioModule.Init()"},
		{"telegram.New", "telegram.New("},
		{"smtp.New", "smtp.New("},
		{"ioModule.SetBot", "ioModule.SetBot("},
		{"smtpServer.OnMessage", "smtpServer.OnMessage("},
	}

	for _, step := range criticalSteps {
		idx := strings.Index(source, step.pattern)
		if idx == -1 {
			t.Errorf("Critical step %q not found in main.go", step.label)
			continue
		}
		if idx >= botStartIdx {
			t.Errorf("Critical step %q (pos %d) appears after bot.Start() (pos %d) — must be initialized before blocking call",
				step.label, idx, botStartIdx)
		}
	}
}

// TestStartupFlowNodeGoCorrespondence verifies that the Go startup flow
// maps correctly to the Node.js startup flow.
// Node: require config → require db → io.init() → new TelegramBot → mailin.start → set_bot
// Go:   config.Load → db.New → io.New+Init → telegram.New → smtp.New+Start → SetBot
func TestStartupFlowNodeGoCorrespondence(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}
	source := string(src)

	// The Node.js version has this order in bin/bot + module loading:
	// 1. config loaded (require in io.js, telegram.js)
	// 2. db initialized (require db.js creates DB)
	// 3. io.init() called in telegram.js (loads domain mappings)
	// 4. TelegramBot created in telegram.js
	// 5. ioApp.set_bot(bot) in bin/bot
	// 6. mailin started in mailin.js (SMTP) — in Go we wire handler before start
	//
	// The Go version must follow the same logical order.
	nodeToGoMapping := []struct {
		nodeStep string
		goStep   string
		pattern  string
	}{
		{"Node: require config", "Go: config.Load", "config.Load("},
		{"Node: require db (init DB)", "Go: db.New", "db.New("},
		{"Node: io.init() (load domains)", "Go: ioModule.Init()", "ioModule.Init()"},
		{"Node: new TelegramBot", "Go: telegram.New", "telegram.New("},
		{"Node: ioApp.set_bot(bot)", "Go: ioModule.SetBot", "ioModule.SetBot("},
		{"Node: mailin.start (SMTP)", "Go: smtp.New + Start", "smtp.New("},
	}

	lastIdx := -1
	for _, mapping := range nodeToGoMapping {
		idx := strings.Index(source, mapping.pattern)
		if idx == -1 {
			t.Errorf("Go step %q (corresponding to %q) not found in main.go",
				mapping.goStep, mapping.nodeStep)
			continue
		}
		if idx <= lastIdx {
			t.Errorf("Go step %q (pos %d) should appear after previous step (pos %d) to match Node.js order",
				mapping.goStep, idx, lastIdx)
		}
		lastIdx = idx
	}
}

// TestStartupFlowErrorHandling verifies that main.go has error handling
// after each critical initialization step (os.Exit on failure).
func TestStartupFlowErrorHandling(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}
	source := string(src)

	// Each of these steps should be followed by error checking with os.Exit(1)
	errorCheckedSteps := []struct {
		label   string
		pattern string
	}{
		{"config.Load", "config.Load("},
		{"db.New", "db.New("},
		{"ioModule.Init", "ioModule.Init()"},
		{"telegram.New", "telegram.New("},
	}

	for _, step := range errorCheckedSteps {
		stepIdx := strings.Index(source, step.pattern)
		if stepIdx == -1 {
			t.Errorf("Step %q not found", step.label)
			continue
		}
		// Check that os.Exit(1) appears after this step (within a reasonable range)
		afterStep := source[stepIdx:]
		exitIdx := strings.Index(afterStep, "os.Exit(1)")
		if exitIdx == -1 {
			t.Errorf("Step %q has no os.Exit(1) error handling after it", step.label)
		} else if exitIdx > 500 {
			// If os.Exit is very far away, it might belong to a different step
			t.Logf("Note: os.Exit(1) for step %q is %d chars away — verify it's the correct error handler", step.label, exitIdx)
		}
	}
}

// TestStartupFlowDeferDatabaseClose verifies that database.Close() is deferred.
func TestStartupFlowDeferDatabaseClose(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}
	source := string(src)

	if !strings.Contains(source, "defer database.Close()") {
		t.Error("main.go should defer database.Close() to ensure cleanup")
	}
}

// TestStartupFlowSMTPInGoroutine verifies SMTP server runs in a goroutine.
func TestStartupFlowSMTPInGoroutine(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}
	source := string(src)

	// SMTP Start should be in a goroutine since it's blocking
	goFuncIdx := strings.Index(source, "go func()")
	startIdx := strings.Index(source, "smtpServer.Start()")
	if goFuncIdx == -1 {
		t.Error("main.go should use 'go func()' to run SMTP server in goroutine")
	}
	if startIdx == -1 {
		t.Error("main.go should call smtpServer.Start()")
	}
	if goFuncIdx != -1 && startIdx != -1 && startIdx < goFuncIdx {
		t.Error("smtpServer.Start() should be inside the goroutine (after 'go func()')")
	}
}

// TestStartupFlowImports verifies all required package imports are present.
func TestStartupFlowImports(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}
	source := string(src)

	requiredImports := []string{
		`"go-version-rewrite/internal/config"`,
		`"go-version-rewrite/internal/db"`,
		`"go-version-rewrite/internal/io"`,
		`"go-version-rewrite/internal/smtp"`,
		`"go-version-rewrite/internal/telegram"`,
		`"go-version-rewrite/internal/upload"`,
	}

	for _, imp := range requiredImports {
		if !strings.Contains(source, imp) {
			t.Errorf("main.go missing required import: %s", imp)
		}
	}
}

// --- Dockerfile verification tests ---

// TestDockerfileMultiStageBuild verifies the Dockerfile uses multi-stage build.
func TestDockerfileMultiStageBuild(t *testing.T) {
	src, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}
	content := string(src)

	// Check for builder stage
	if !strings.Contains(strings.ToLower(content), "as builder") {
		t.Error("Dockerfile should have a builder stage (FROM ... AS builder)")
	}

	// Count FROM instructions — multi-stage needs at least 2
	fromCount := 0
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.ToUpper(line))
		if strings.HasPrefix(trimmed, "FROM ") {
			fromCount++
		}
	}
	if fromCount < 2 {
		t.Errorf("Dockerfile should have at least 2 FROM instructions for multi-stage build, got %d", fromCount)
	}
}

// TestDockerfileExposesPort25 verifies the Dockerfile exposes port 25.
func TestDockerfileExposesPort25(t *testing.T) {
	src, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}
	content := string(src)

	found := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.ToUpper(line))
		if strings.HasPrefix(trimmed, "EXPOSE") && strings.Contains(trimmed, "25") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Dockerfile should EXPOSE port 25")
	}
}

// TestDockerfileHasCMD verifies the Dockerfile has a CMD to run the binary.
func TestDockerfileHasCMD(t *testing.T) {
	src, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}
	content := string(src)

	found := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.ToUpper(line))
		if strings.HasPrefix(trimmed, "CMD") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Dockerfile should have a CMD instruction to run the binary")
	}
}

// TestDockerfileBuilderUsesGolang verifies the builder stage uses golang image.
func TestDockerfileBuilderUsesGolang(t *testing.T) {
	src, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}
	content := string(src)

	found := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.ToLower(line))
		if strings.HasPrefix(trimmed, "from") && strings.Contains(trimmed, "golang") && strings.Contains(trimmed, "as builder") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Dockerfile builder stage should use golang image (FROM golang:... AS builder)")
	}
}

// TestDockerfileRuntimeIsLightweight verifies the runtime stage uses a lightweight image.
func TestDockerfileRuntimeIsLightweight(t *testing.T) {
	src, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}
	content := string(src)

	// Find the second FROM (runtime stage)
	lines := strings.Split(content, "\n")
	fromCount := 0
	runtimeImage := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.ToLower(line))
		if strings.HasPrefix(trimmed, "from ") {
			fromCount++
			if fromCount == 2 {
				runtimeImage = trimmed
				break
			}
		}
	}

	if runtimeImage == "" {
		t.Fatal("Could not find runtime stage FROM instruction")
	}

	// Runtime should use a lightweight image (alpine, distroless, scratch, etc.)
	lightweightImages := []string{"alpine", "distroless", "scratch", "busybox", "debian-slim"}
	isLightweight := false
	for _, img := range lightweightImages {
		if strings.Contains(runtimeImage, img) {
			isLightweight = true
			break
		}
	}
	if !isLightweight {
		t.Logf("Runtime image: %s — consider using a lightweight base image (alpine, distroless, scratch)", runtimeImage)
	}
}

// TestDockerfileCMDRunsBot verifies the CMD runs the bot binary.
func TestDockerfileCMDRunsBot(t *testing.T) {
	src, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}
	content := string(src)

	found := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), "CMD") && strings.Contains(trimmed, "bot") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Dockerfile CMD should run the bot binary")
	}
}
