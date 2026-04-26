// go run scripts/setup_oauth.go
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
    godotenv.Load()
    cfg := &oauth2.Config{
        ClientID:     os.Getenv("GMAIL_CLIENT_ID"),
        ClientSecret: os.Getenv("GMAIL_CLIENT_SECRET"),
        RedirectURL:  "http://localhost:8090/callback",
        Endpoint:     google.Endpoint,
        Scopes:       []string{gmail.GmailReadonlyScope, gmail.GmailModifyScope, gmail.GmailComposeScope},
    }
    codeCh := make(chan string, 1)
    http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "<h1>Success! Close this tab.</h1>")
        codeCh <- r.URL.Query().Get("code")
    })
    go http.ListenAndServe(":8090", nil)
    fmt.Println("\nOpen this URL in your browser:")
    fmt.Println(cfg.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.ApprovalForce))
    token, err := cfg.Exchange(context.Background(), <-codeCh)
    if err != nil { log.Fatalf("Token exchange failed: %v", err) }
    fmt.Printf("\nAdd to .env:\nGMAIL_REFRESH_TOKEN=%s\n", token.RefreshToken)
}
