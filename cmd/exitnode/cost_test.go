package main

import (
	"testing"
	"time"
)

func TestParsePeriod(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"24h", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"month", 30 * 24 * time.Hour},
		{"30m", 30 * time.Minute},
	}
	for _, tc := range cases {
		got, err := parsePeriod(tc.in)
		if err != nil {
			t.Errorf("parsePeriod(%q) err=%v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parsePeriod(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
