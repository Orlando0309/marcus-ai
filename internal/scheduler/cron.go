package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronSchedule represents a parsed cron expression
type CronSchedule struct {
	Minute     []int
	Hour       []int
	DayOfMonth []int
	Month      []int
	DayOfWeek  []int
	Expression string
}

// ParseCron parses a cron expression (5-field format: minute hour day month dow)
func ParseCron(expr string) (*CronSchedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty cron expression")
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("invalid cron expression: expected 5 fields, got %d", len(fields))
	}

	schedule := &CronSchedule{Expression: expr}

	var err error
	schedule.Minute, err = parseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}

	schedule.Hour, err = parseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}

	schedule.DayOfMonth, err = parseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day of month field: %w", err)
	}

	schedule.Month, err = parseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}

	schedule.DayOfWeek, err = parseCronField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("invalid day of week field: %w", err)
	}

	return schedule, nil
}

// IsValidCron returns true if the expression is a valid cron expression
func IsValidCron(expr string) bool {
	_, err := ParseCron(expr)
	return err == nil
}

// Next calculates the next occurrence after the given time
func (s *CronSchedule) Next(from time.Time) time.Time {
	// Start from the next minute
	t := from.Add(time.Minute).Truncate(time.Minute)

	// Limit search to prevent infinite loops
	maxIterations := 366 * 24 * 60 // 1 year of minutes
	for i := 0; i < maxIterations; i++ {
		if s.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}

	return time.Time{} // No next occurrence found
}

// matches returns true if the given time matches the schedule
func (s *CronSchedule) matches(t time.Time) bool {
	if !contains(s.Minute, t.Minute()) {
		return false
	}
	if !contains(s.Hour, t.Hour()) {
		return false
	}
	if !contains(s.DayOfMonth, t.Day()) {
		return false
	}
	if !contains(s.Month, int(t.Month())) {
		return false
	}
	if !contains(s.DayOfWeek, int(t.Weekday())) {
		return false
	}
	return true
}

// parseCronField parses a single cron field
func parseCronField(field string, min, max int) ([]int, error) {
	// Handle wildcard
	if field == "*" {
		var result []int
		for i := min; i <= max; i++ {
			result = append(result, i)
		}
		return result, nil
	}

	var result []int
	seen := make(map[int]bool)

	// Split by comma for list values
	parts := strings.Split(field, ",")
	for _, part := range parts {
		values, err := parseCronRange(part, min, max)
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			if !seen[v] {
				result = append(result, v)
				seen[v] = true
			}
		}
	}

	return result, nil
}

// parseCronRange parses a range like "1-5" or "*/5" or "1"
func parseCronRange(part string, min, max int) ([]int, error) {
	part = strings.TrimSpace(part)

	// Handle step values like */5 or 1-10/2
	if strings.Contains(part, "/") {
		parts := strings.Split(part, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid step value: %s", part)
		}

		step, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid step: %s", parts[1])
		}

		var baseValues []int
		if parts[0] == "*" {
			for i := min; i <= max; i++ {
				baseValues = append(baseValues, i)
			}
		} else {
			baseValues, err = parseCronRange(parts[0], min, max)
			if err != nil {
				return nil, err
			}
		}

		var result []int
		for i, v := range baseValues {
			if i%step == 0 {
				result = append(result, v)
			}
		}
		return result, nil
	}

	// Handle ranges like 1-5
	if strings.Contains(part, "-") {
		parts := strings.Split(part, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range: %s", part)
		}

		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid range start: %s", parts[0])
		}

		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid range end: %s", parts[1])
		}

		if start < min || end > max || start > end {
			return nil, fmt.Errorf("range out of bounds: %s", part)
		}

		var result []int
		for i := start; i <= end; i++ {
			result = append(result, i)
		}
		return result, nil
	}

	// Single value
	val, err := strconv.Atoi(part)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %s", part)
	}

	if val < min || val > max {
		return nil, fmt.Errorf("value out of bounds: %d", val)
	}

	return []int{val}, nil
}

// contains checks if a slice contains a value
func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// Common cron presets
var CronPresets = map[string]string{
	"@yearly":  "0 0 1 1 *",
	"@monthly": "0 0 1 * *",
	"@weekly":  "0 0 * * 0",
	"@daily":   "0 0 * * *",
	"@hourly":  "0 * * * *",
}

// ExpandPreset expands a preset like "@daily" to its cron expression
func ExpandPreset(preset string) string {
	if expr, ok := CronPresets[preset]; ok {
		return expr
	}
	return preset
}
