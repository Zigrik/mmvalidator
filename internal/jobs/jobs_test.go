package jobs

import (
	"context"
	"mmvalidator/internal/checker"
	"mmvalidator/internal/parser"
	"testing"
	"time"
)

type fake struct{}

func (fake) Check(_ context.Context, c []string) ([]checker.CheckResult, error) {
	o := make([]checker.CheckResult, len(c))
	for i, v := range c {
		o[i] = checker.CheckResult{Code: v, Status: checker.Invalid, Reason: "bad"}
	}
	return o, nil
}
func TestDeduplicationPreservesOccurrences(t *testing.T) {
	m := NewManager(fake{}, 1000)
	j := m.Create([]parser.InputRecord{{LineNumber: 1, Code: "x", Tag: "a"}, {LineNumber: 2, Code: "x", Tag: "b"}})
	for i := 0; i < 50; i++ {
		s := j.Snapshot(true)
		if terminal(s.State) {
			if s.UniqueCodes != 1 || len(s.InvalidResults) != 1 || len(s.InvalidResults[0].Lines) != 2 {
				t.Fatalf("%+v", s)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("not finished")
}
