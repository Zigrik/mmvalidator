package config

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port        string
	APIKey      string
	APIURL      string
	RateRPS     int
	BatchSize   int
	HTTPTimeout time.Duration
}

func Load() (Config, error) {
	_ = loadDotEnv(".env")
	c := Config{Port: value("PORT", "8080"), APIKey: os.Getenv("API_KEY"), APIURL: os.Getenv("TRUE_API_URL"), RateRPS: intValue("RATE_LIMIT_RPS", 1), BatchSize: intValue("BATCH_SIZE", 1000), HTTPTimeout: time.Duration(intValue("HTTP_TIMEOUT_SECONDS", 20)) * time.Second}
	if c.RateRPS < 1 {
		return c, errors.New("RATE_LIMIT_RPS must be at least 1")
	}
	if c.BatchSize < 1 || c.BatchSize > 1000 {
		return c, errors.New("BATCH_SIZE must be between 1 and 1000")
	}
	return c, nil
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func intValue(key string, fallback int) int {
	v, err := strconv.Atoi(value(key, strconv.Itoa(fallback)))
	if err != nil {
		return fallback
	}
	return v
}

func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && os.Getenv(strings.TrimSpace(k)) == "" {
			_ = os.Setenv(strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), "\""))
		}
	}
	return s.Err()
}
