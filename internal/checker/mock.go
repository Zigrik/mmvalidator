package checker

import (
	"context"
	"strings"
)

// Mock is intentionally deterministic and is selected only when TRUE_API_URL
// is empty. It lets the complete application be exercised without credentials.
type Mock struct{}

func (Mock) Check(_ context.Context, codes []string) ([]CheckResult, error) {
	out := make([]CheckResult, 0, len(codes))
	for _, code := range codes {
		if strings.Contains(strings.ToUpper(code), "INVALID") {
			out = append(out, CheckResult{Code: code, Status: Invalid, Reason: "mock crypto verification failed"})
		} else {
			out = append(out, CheckResult{Code: code, Status: Valid})
		}
	}
	return out, nil
}
