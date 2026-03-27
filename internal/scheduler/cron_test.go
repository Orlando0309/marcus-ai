package scheduler

import (
	"testing"
	"time"
)

func TestParseCron(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{
			name: "valid daily at midnight",
			expr: "0 0 * * *",
		},
		{
			name: "valid every minute",
			expr: "* * * * *",
		},
		{
			name: "valid every 5 minutes",
			expr: "*/5 * * * *",
		},
		{
			name: "valid range",
			expr: "0 9-17 * * 1-5",
		},
		{
			name: "valid list",
			expr: "0 0,12 * * *",
		},
		{
			name:    "empty expression",
			expr:    "",
			wantErr: true,
		},
		{
			name:    "invalid field count",
			expr:    "0 0 * *",
			wantErr: true,
		},
		{
			name:    "invalid minute",
			expr:    "60 0 * * *",
			wantErr: true,
		},
		{
			name:    "invalid hour",
			expr:    "0 24 * * *",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule, err := ParseCron(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tt.expr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if schedule.Expression != tt.expr {
				t.Errorf("expected expression %q, got %q", tt.expr, schedule.Expression)
			}
		})
	}
}

func TestCronSchedule_Next(t *testing.T) {
	// Test daily at 9am
	schedule, err := ParseCron("0 9 * * *")
	if err != nil {
		t.Fatalf("failed to parse cron: %v", err)
	}

	// From 8am today, next should be 9am today
	from := time.Date(2026, 3, 27, 8, 0, 0, 0, time.UTC)
	next := schedule.Next(from)

	expected := time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected next run at %v, got %v", expected, next)
	}

	// From 10am today, next should be 9am tomorrow
	from = time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	next = schedule.Next(from)

	expected = time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected next run at %v, got %v", expected, next)
	}
}

func TestCronSchedule_matches(t *testing.T) {
	schedule, _ := ParseCron("0 9 * * 1") // Monday at 9am

	tests := []struct {
		time  time.Time
		match bool
	}{
		{time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC), true},  // Monday 9am
		{time.Date(2026, 3, 23, 9, 1, 0, 0, time.UTC), false}, // Monday 9:01am
		{time.Date(2026, 3, 24, 9, 0, 0, 0, time.UTC), false}, // Tuesday 9am
		{time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC), false}, // Monday 10am
	}

	for _, tt := range tests {
		got := schedule.matches(tt.time)
		if got != tt.match {
			t.Errorf("matches(%v) = %v, want %v", tt.time, got, tt.match)
		}
	}
}

func TestIsValidCron(t *testing.T) {
	tests := []struct {
		expr string
		want bool
	}{
		{"0 0 * * *", true},
		{"*/5 * * * *", true},
		{"0 9-17 * * 1-5", true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsValidCron(tt.expr)
		if got != tt.want {
			t.Errorf("IsValidCron(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func TestExpandPreset(t *testing.T) {
	tests := []struct {
		preset   string
		expected string
	}{
		{"@daily", "0 0 * * *"},
		{"@hourly", "0 * * * *"},
		{"@weekly", "0 0 * * 0"},
		{"@monthly", "0 0 1 * *"},
		{"@yearly", "0 0 1 1 *"},
		{"0 0 * * *", "0 0 * * *"}, // Not a preset, should return as-is
	}

	for _, tt := range tests {
		got := ExpandPreset(tt.preset)
		if got != tt.expected {
			t.Errorf("ExpandPreset(%q) = %q, want %q", tt.preset, got, tt.expected)
		}
	}
}
