package parser

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

const MaxRecords = 400000

type InputRecord struct {
	LineNumber int    `json:"lineNumber"`
	Code       string `json:"code"`
	Tag        string `json:"tag"`
}

// ParseText treats every physical non-empty line as a record. Only the first
// whitespace run separates an optional tag; ASCII GS is ordinary code data.
func ParseText(r io.Reader) ([]InputRecord, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	var out []InputRecord
	line := 0
	for s.Scan() {
		line++
		raw := strings.TrimSuffix(s.Text(), "\r")
		rec, ok := split(raw)
		if ok {
			out = append(out, InputRecord{LineNumber: line, Code: rec[0], Tag: rec[1]})
			if len(out) > MaxRecords {
				return nil, fmt.Errorf("maximum is %d non-empty records", MaxRecords)
			}
		}
	}
	return out, s.Err()
}

// ParseCSV uses the standard parser, so quoted commas/newlines are not damaged.
// The first column is code and the optional second column is the tag.
func ParseCSV(r io.Reader) ([]InputRecord, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = false
	var out []InputRecord
	physical := 0
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV: %w", err)
		}
		physical++
		if len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		code := row[0]
		tag := ""
		if len(row) > 1 {
			tag = row[1]
		}
		out = append(out, InputRecord{LineNumber: physical, Code: code, Tag: tag})
		if len(out) > MaxRecords {
			return nil, fmt.Errorf("maximum is %d non-empty records", MaxRecords)
		}
	}
	return out, nil
}

func split(raw string) ([2]string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return [2]string{}, false
	}
	i := strings.IndexAny(raw, " \t")
	if i < 0 {
		return [2]string{raw, ""}, true
	}
	return [2]string{raw[:i], strings.TrimSpace(raw[i:])}, true
}
