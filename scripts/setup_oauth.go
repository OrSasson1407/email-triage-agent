// Run once to generate your Gmail OAuth refresh token
// Usage: go run scripts/setup_oauth.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file, reading from environment")
	}

	clientID := os.Getenv("GMAIL_CLIENT_ID")
	clientSecret := os.Getenv("GMAIL_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		log.Fatal("GMAIL_CLIENT_ID and GMAIL_CLIENT_SECRET must be set in .env")
	}

	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "http://localhost:8090/callback",
		Endpoint:     google.Endpoint,
		Scopes: []string{
			gmail.GmailReadonlyScope,
			gmail.GmailModifyScope,
			gmail.GmailComposeScope,
		},
	}

	// Start local callback server to catch the auth code
	codeCh := make(chan string, 1)
	srv := &http.Server{Addr: ":8090"}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			fmt.Fprintf(w, "<h1>Error: no code in callback</h1>")
			return
		}
		fmt.Fprintf(w, `<h1>✅ Success!</h1><p>You can close this tab and return to the terminal.</p>`)
		codeCh <- code
	})

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Local server error: %v", err)
		}
	}()

	// Print the auth URL for the user to open
	authURL := cfg.AuthCodeURL("state-token",
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	)

	fmt.Println("\n============================================")
	fmt.Println(" Gmail OAuth2 Setup")
	fmt.Println("============================================")
	fmt.Println("\n1. Open this URL in your browser:\n")
	fmt.Println(authURL)
	fmt.Println("\n2. Sign in and grant permissions")
	fmt.Println("3. You will be redirected back automatically")
	fmt.Println("\nWaiting for callback on http://localhost:8090 ...")

	// Wait for the auth code
	code := <-codeCh

	// Shut down the local server
	srv.Shutdown(context.Background())

	// Exchange code for token
	token, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("\n❌ Token exchange failed: %v", err)
	}

	if token.RefreshToken == "" {
		log.Fatal("\n❌ No refresh token received. Make sure you used oauth2.ApprovalForce and revoked previous access at https://myaccount.google.com/permissions")
	}

	fmt.Println("\n============================================")
	fmt.Println(" ✅ Success! Add this to your .env file:")
	fmt.Println("============================================\n")
	fmt.Printf("GMAIL_REFRESH_TOKEN=%s\n\n", token.RefreshToken)
}