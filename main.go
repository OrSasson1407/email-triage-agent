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
    logger, _ := zap.NewProduction()
    defer logger.Sync()
    cfg, err := config.Load()
    if err != nil {
        logger.Fatal("Failed to load config", zap.Error(err))
    }
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    go func() {
        if err := api.Start(cfg, logger); err != nil {
            logger.Error("API server error", zap.Error(err))
        }
    }()
    sched := scheduler.New(cfg, logger)
    sched.Start(ctx)
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    logger.Info("Shutting down...")
    sched.Stop()
}
