package checker

import "context"

type Status string

const (
	Valid      Status = "valid"
	Invalid    Status = "invalid"
	CheckError Status = "check_error"
)

type CheckResult struct {
	Code   string `json:"code"`
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
}
type Validator interface {
	Check(context.Context, []string) ([]CheckResult, error)
}
