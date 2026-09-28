package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type TrueAPI struct {
	URL, Token string
	Client     *http.Client
	Limiter    *Limiter
	Retries    int
}
type Limiter struct{ ticks <-chan time.Time }

func NewLimiter(rps int) *Limiter {
	if rps < 1 {
		rps = 1
	}
	t := time.NewTicker(time.Second / time.Duration(rps))
	return &Limiter{ticks: t.C}
}
func (l *Limiter) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.ticks:
		return nil
	}
}

type apiResponse struct {
	Codes []struct {
		CIS      string `json:"cis"`
		Verified bool   `json:"verified"`
		Message  string `json:"message"`
	} `json:"codes"`
}

func (c *TrueAPI) Check(ctx context.Context, codes []string) ([]CheckResult, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	if len(codes) > 1000 {
		return nil, fmt.Errorf("batch contains %d codes; maximum is 1000", len(codes))
	}
	body, err := json.Marshal(struct {
		Codes []string `json:"codes"`
	}{codes})
	if err != nil {
		return nil, err
	}
	var last error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if err := c.Limiter.Wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.Token)
		resp, err := c.Client.Do(req)
		if err == nil {
			raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			resp.Body.Close()
			if readErr != nil {
				err = readErr
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return parseResponse(codes, raw)
			} else {
				err = fmt.Errorf("True API returned HTTP %d", resp.StatusCode)
				if !retryableStatus(resp.StatusCode) {
					return nil, err
				}
				if d := retryAfter(resp.Header.Get("Retry-After")); d > 0 {
					if wait(ctx, d) != nil {
						return nil, ctx.Err()
					}
					continue
				}
			}
		}
		last = err
		if attempt == c.Retries || !retryableError(err) {
			break
		}
		if wait(ctx, backoff(attempt)) != nil {
			return nil, ctx.Err()
		}
	}
	return nil, last
}
func parseResponse(requested []string, raw []byte) ([]CheckResult, error) {
	var a apiResponse
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("invalid True API response: %w", err)
	}
	seen := make(map[string]struct{}, len(a.Codes))
	out := make([]CheckResult, 0, len(requested))
	for _, item := range a.Codes {
		seen[item.CIS] = struct{}{}
		if item.Verified {
			out = append(out, CheckResult{Code: item.CIS, Status: Valid})
		} else {
			reason := item.Message
			if reason == "" {
				reason = "crypto verification failed"
			}
			out = append(out, CheckResult{Code: item.CIS, Status: Invalid, Reason: reason})
		}
	}
	for _, code := range requested {
		if _, ok := seen[code]; !ok {
			out = append(out, CheckResult{Code: code, Status: CheckError, Reason: "code missing from API response"})
		}
	}
	return out, nil
}
func retryableStatus(s int) bool { return s == 429 || s == 500 || s == 502 || s == 503 || s == 504 }
func retryableError(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "HTTP ") {
		return false
	}
	return true
}
func retryAfter(v string) time.Duration {
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}
func backoff(n int) time.Duration {
	return time.Duration(1<<n)*time.Second + time.Duration(rand.Intn(250))*time.Millisecond
}
func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
