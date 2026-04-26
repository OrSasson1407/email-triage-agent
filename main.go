package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"email-triage-agent/api"
	"email-triage-agent/config"
	"email-triage-agent/scheduler"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from environment")
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start HTTP API in background
	go func() {
		if err := api.Start(cfg, logger); err != nil {
			logger.Error("API server stopped", zap.Error(err))
		}
	}()

	// Start scheduler (cron + pipeline)
	sched := scheduler.New(cfg, logger)
	sched.Start(ctx)

	// Wait for CTRL+C or kill signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutdown signal received, stopping...")
	cancel()
	sched.Stop()
	logger.Info("Shutdown complete")
}