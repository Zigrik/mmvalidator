package jobs

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mmvalidator/internal/checker"
	"mmvalidator/internal/parser"
)

type State string

const (
	Parsing             State = "parsing"
	Queued              State = "queued"
	Running             State = "running"
	Completed           State = "completed"
	CompletedWithErrors State = "completed_with_errors"
	Cancelled           State = "cancelled"
	Failed              State = "failed"
)

type Occurrence struct {
	LineNumber int    `json:"lineNumber"`
	Tag        string `json:"tag"`
}
type Result struct {
	Code   string `json:"code"`
	Tags   string `json:"tags"`
	Lines  []int  `json:"lines"`
	Reason string `json:"reason"`
}
type Snapshot struct {
	ID             string   `json:"id"`
	State          State    `json:"state"`
	TotalLines     int      `json:"totalLines"`
	UniqueCodes    int      `json:"uniqueCodes"`
	Checked        int      `json:"checked"`
	Valid          int      `json:"valid"`
	Invalid        int      `json:"invalid"`
	Errors         int      `json:"errors"`
	Message        string   `json:"message,omitempty"`
	InvalidResults []Result `json:"invalidResults,omitempty"`
	ErrorResults   []Result `json:"errorResults,omitempty"`
}
type Job struct {
	mu                                             sync.RWMutex
	id                                             string
	state                                          State
	total, unique, checked, valid, invalid, errors int
	message                                        string
	occurrences                                    map[string][]Occurrence
	invalidResults, errorResults                   []Result
	validator                                      checker.Validator
	cancel                                         context.CancelFunc
	finished                                       time.Time
}
type Manager struct {
	mu        sync.RWMutex
	jobs      map[string]*Job
	validator checker.Validator
	batchSize int
	ttl       time.Duration
}

func NewManager(v checker.Validator, batchSize int) *Manager {
	m := &Manager{jobs: make(map[string]*Job), validator: v, batchSize: batchSize, ttl: time.Hour}
	go m.cleanup()
	return m
}
func (m *Manager) Create(records []parser.InputRecord) *Job {
	return m.CreateWithValidator(records, m.validator)
}
func (m *Manager) CreateWithValidator(records []parser.InputRecord, validator checker.Validator) *Job {
	id := newID()
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{id: id, state: Queued, total: len(records), occurrences: make(map[string][]Occurrence), validator: validator, cancel: cancel}
	for _, r := range records {
		j.occurrences[r.Code] = append(j.occurrences[r.Code], Occurrence{LineNumber: r.LineNumber, Tag: r.Tag})
	}
	j.unique = len(j.occurrences)
	m.mu.Lock()
	m.jobs[id] = j
	m.mu.Unlock()
	go m.run(ctx, j)
	return j
}
func (m *Manager) Get(id string) (*Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[id]
	return j, ok
}
func (m *Manager) Cancel(id string) error {
	j, ok := m.Get(id)
	if !ok {
		return errors.New("job not found")
	}
	j.mu.Lock()
	if terminal(j.state) {
		j.mu.Unlock()
		return nil
	}
	j.cancel()
	j.mu.Unlock()
	return nil
}
func (j *Job) Snapshot(withResults bool) Snapshot {
	j.mu.RLock()
	defer j.mu.RUnlock()
	s := Snapshot{ID: j.id, State: j.state, TotalLines: j.total, UniqueCodes: j.unique, Checked: j.checked, Valid: j.valid, Invalid: j.invalid, Errors: j.errors, Message: j.message}
	if withResults {
		s.InvalidResults = append([]Result(nil), j.invalidResults...)
		s.ErrorResults = append([]Result(nil), j.errorResults...)
	}
	return s
}
func (m *Manager) run(ctx context.Context, j *Job) {
	j.mu.Lock()
	j.state = Running
	j.mu.Unlock()
	codes := make([]string, 0, len(j.occurrences))
	for c := range j.occurrences {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for start := 0; start < len(codes); start += m.batchSize {
		if ctx.Err() != nil {
			m.finish(j, Cancelled, "")
			return
		}
		end := start + m.batchSize
		if end > len(codes) {
			end = len(codes)
		}
		batch := codes[start:end]
		results, err := j.validator.Check(ctx, batch)
		if err != nil {
			if ctx.Err() != nil {
				m.finish(j, Cancelled, "")
				return
			}
			results = make([]checker.CheckResult, len(batch))
			for i, c := range batch {
				results[i] = checker.CheckResult{Code: c, Status: checker.CheckError, Reason: "request failed after retries"}
			}
		}
		m.apply(j, results)
	}
	j.mu.Lock()
	state := Completed
	if j.errors > 0 {
		state = CompletedWithErrors
	}
	j.state = state
	j.finished = time.Now()
	j.mu.Unlock()
}
func (m *Manager) apply(j *Job, results []checker.CheckResult) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, r := range results {
		j.checked++
		switch r.Status {
		case checker.Valid:
			j.valid++
		case checker.Invalid:
			j.invalid++
			j.invalidResults = append(j.invalidResults, j.result(r))
		default:
			j.errors++
			j.errorResults = append(j.errorResults, j.result(r))
		}
	}
}
func (j *Job) result(r checker.CheckResult) Result {
	occurrences := j.occurrences[r.Code]
	lines := make([]int, 0, len(occurrences))
	tags := make([]string, 0, len(occurrences))
	seen := map[string]bool{}
	for _, o := range occurrences {
		lines = append(lines, o.LineNumber)
		if o.Tag != "" && !seen[o.Tag] {
			tags = append(tags, o.Tag)
			seen[o.Tag] = true
		}
	}
	return Result{Code: r.Code, Tags: strings.Join(tags, " | "), Lines: lines, Reason: r.Reason}
}
func (m *Manager) finish(j *Job, state State, message string) {
	j.mu.Lock()
	j.state = state
	j.message = message
	j.finished = time.Now()
	j.mu.Unlock()
}
func terminal(s State) bool {
	return s == Completed || s == CompletedWithErrors || s == Cancelled || s == Failed
}
func (m *Manager) cleanup() {
	tick := time.NewTicker(10 * time.Minute)
	defer tick.Stop()
	for range tick.C {
		cut := time.Now().Add(-m.ttl)
		m.mu.Lock()
		for id, j := range m.jobs {
			j.mu.RLock()
			drop := terminal(j.state) && j.finished.Before(cut)
			j.mu.RUnlock()
			if drop {
				delete(m.jobs, id)
			}
		}
		m.mu.Unlock()
	}
}
func newID() string { return fmt.Sprintf("j%x", time.Now().UnixNano()) }
func WriteCSV(s Snapshot, errorsOnly bool, w *csv.Writer) {
	_ = w.Write([]string{"code", "tag", "lines", "reason"})
	items := s.InvalidResults
	if errorsOnly {
		items = s.ErrorResults
	}
	for _, item := range items {
		parts := make([]string, len(item.Lines))
		for i, n := range item.Lines {
			parts[i] = fmt.Sprint(n)
		}
		_ = w.Write([]string{item.Code, item.Tags, strings.Join(parts, " "), item.Reason})
	}
	w.Flush()
}
