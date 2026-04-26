package api

import (
    "encoding/json"
    "fmt"
    "net/http"

    "go.uber.org/zap"
    "email-triage-agent/config"
)

func Start(cfg config.Config, logger *zap.Logger) error {
    mux := http.NewServeMux()
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
    })
    mux.HandleFunc("/trigger", func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("X-API-Secret") != cfg.APISecret {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"status": "triggered"})
    })
    logger.Info("API listening", zap.String("port", cfg.APIPort))
    return http.ListenAndServe(fmt.Sprintf(":%s", cfg.APIPort), mux)
}
