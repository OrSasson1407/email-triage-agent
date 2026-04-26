// go run scripts/migrate.go
package main

import (
    "context"
    "log"

    "github.com/joho/godotenv"
    "email-triage-agent/config"
    "email-triage-agent/db"
)

func main() {
    godotenv.Load()
    cfg, err := config.Load()
    if err != nil { log.Fatalf("Config error: %v", err) }
    database, err := db.New(cfg)
    if err != nil { log.Fatalf("DB connect failed: %v", err) }
    defer database.Close()
    if err := database.Migrate(context.Background()); err != nil { log.Fatalf("Migration failed: %v", err) }
    log.Println("Migration complete — triage_log table ready")
}
