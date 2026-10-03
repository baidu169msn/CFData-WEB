package main

import (
	"strings"
	"testing"
)

func TestFilterQualifiedRows(t *testing.T) {
	rows := []cliResultRow{
		{"ip": "1.1.1.1", "port": "443", "speed": "2.50MB/s"},
		{"ip": "1.1.1.2", "port": "443", "speed": "0.05MB/s"},
		{"ip": "1.1.1.3", "port": "443", "speed": "-"},
		{"ip": "1.1.1.4", "port": "443", "speed": ""},
		{"ip": "1.1.1.5", "port": "443", "speed": "0.10MB/s"},
	}
	got := filterQualifiedRows(rows, 0.1)
	if len(got) != 2 || got[0]["ip"] != "1.1.1.1" || got[1]["ip"] != "1.1.1.5" {
		t.Fatalf("unexpected qualified rows: %v", got)
	}
	if len(filterQualifiedRows(rows, 10)) != 0 {
		t.Fatal("expected no rows above 10MB/s")
	}
}

func TestFormatTXTQualifiedOnly(t *testing.T) {
	rows := filterQualifiedRows([]cliResultRow{
		{"ip": "1.1.1.1", "port": "443", "ipport": "1.1.1.1:443", "dc": "HKG", "speed": "2.50MB/s"},
		{"ip": "2606:4700::1", "port": "443", "ipport": "2606:4700::1:443", "dc": "HKG", "speed": "1.00MB/s"},
		{"ip": "1.1.1.9", "port": "443", "ipport": "1.1.1.9:443", "dc": "HKG", "speed": "0.01MB/s"},
	}, 0.1)
	out, err := formatCLIResults(rows, cliExportConfig{Format: "txt", Fields: "compact", V6Bracket: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "1.1.1.1:443#HKG\n[2606:4700::1]:443#HKG\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
	if strings.Contains(out, "1.1.1.9") {
		t.Fatal("unqualified IP leaked into export")
	}
}
