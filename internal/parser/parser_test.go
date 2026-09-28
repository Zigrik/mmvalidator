package parser

import (
	"strings"
	"testing"
)

func TestTextAndGS(t *testing.T) {
	got, err := ParseText(strings.NewReader("\nA tag\nX\x1d93Y\n"))
	if err != nil || len(got) != 2 || got[0].Code != "A" || got[0].Tag != "tag" || got[1].Code != "X\x1d93Y" {
		t.Fatalf("%+v %v", got, err)
	}
}
func TestCSV(t *testing.T) {
	got, err := ParseCSV(strings.NewReader("\"A,B\",tag\n"))
	if err != nil || got[0].Code != "A,B" {
		t.Fatal(got, err)
	}
}
