package main

import (
	"log/slog"
	"net/http"
	"os"

	"mmvalidator/internal/checker"
	"mmvalidator/internal/config"
	"mmvalidator/internal/jobs"
	"mmvalidator/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}
	limiter := checker.NewLimiter(cfg.RateRPS)
	newValidator := func(token string) checker.Validator {
		if cfg.APIURL == "" {
			return checker.Mock{}
		}
		if token == "" {
			token = cfg.APIKey
		}
		return &checker.TrueAPI{URL: cfg.APIURL, Token: token, Client: &http.Client{Timeout: cfg.HTTPTimeout}, Limiter: limiter, Retries: 3}
	}
	if cfg.APIURL == "" {
		slog.Warn("TRUE_API_URL is empty; local mock validator is active")
	}
	m := jobs.NewManager(newValidator(cfg.APIKey), cfg.BatchSize)
	s := web.New(m, cfg.APIKey != "", newValidator)
	slog.Info("mmvalidator starting", "port", cfg.Port, "rate_limit_rps", cfg.RateRPS)
	if err := http.ListenAndServe(":"+cfg.Port, s.Handler()); err != nil {
		slog.Error("server stopped", "error", err)
	}
}
