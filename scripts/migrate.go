//go:build ignore

// Run once to create all required PostgreSQL tables
// Usage: go run scripts/migrate.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"email-triage-agent/config"
	"email-triage-agent/db"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file, reading from environment")
	}

	fmt.Println("\n============================================")
	fmt.Println(" Email Triage Agent — DB Migration")
	fmt.Println("============================================\n")

	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		log.Fatal("❌ POSTGRES_URL is not set in .env")
	}

	fmt.Println("Connecting to Neon PostgreSQL...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Config error: %v", err)
	}

	database, err := db.New(cfg)
	if err != nil {
		log.Fatalf("❌ DB connection failed: %v\n\nCheck your POSTGRES_URL in .env", err)
	}
	defer database.Close()

	fmt.Println("✅ Connected successfully")
	fmt.Println("Running migrations...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := database.Migrate(ctx); err != nil {
		log.Fatalf("❌ Migration failed: %v", err)
	}

	fmt.Println("\n✅ Migration complete!")
	fmt.Println("\nTables created:")
	fmt.Println("  • triage_log       — email classifications")
	fmt.Println("\nIndexes created:")
	fmt.Println("  • idx_urgency      — fast urgency queries")
	fmt.Println("  • idx_topic        — fast topic queries")
	fmt.Println("  • idx_is_vip       — fast VIP queries")
	fmt.Println("  • idx_processed_at — fast time-range queries")
	fmt.Println("\nNext step:")
	fmt.Println("  Copy .env.example to .env, fill in all values, then:")
	fmt.Println("  go run main.go")
	fmt.Println()
}