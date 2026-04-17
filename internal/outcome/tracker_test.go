package outcome

import (
	"testing"
	"time"
)

func TestTracker_RecordOutcome(t *testing.T) {
	tracker := NewTracker("")

	outcome := ActionOutcome{
		ID:         "test-1",
		Timestamp:  time.Now(),
		ActionType: ActionWriteFile,
		Context:    "Test task",
		Success:    true,
		Duration:   100 * time.Millisecond,
	}

	tracker.RecordOutcome(outcome)

	stats := tracker.GetStats(ActionWriteFile)
	if stats == nil {
		t.Fatal("Expected stats to be recorded")
	}

	if stats.TotalAttempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", stats.TotalAttempts)
	}

	if stats.Successful != 1 {
		t.Errorf("Expected 1 successful, got %d", stats.Successful)
	}
}

func TestTracker_SuccessRate(t *testing.T) {
	tracker := NewTracker("")

	// Record 5 successes and 3 failures
	for i := 0; i < 5; i++ {
		tracker.RecordOutcome(ActionOutcome{
			ID:         "success-" + string(rune(i)),
			Timestamp:  time.Now(),
			ActionType: ActionReadFile,
			Success:    true,
		})
	}

	for i := 0; i < 3; i++ {
		tracker.RecordOutcome(ActionOutcome{
			ID:         "fail-" + string(rune(i)),
			Timestamp:  time.Now(),
			ActionType: ActionReadFile,
			Success:    false,
		})
	}

	stats := tracker.GetStats(ActionReadFile)
	expectedRate := 62.5 // 5/8 * 100

	if stats.SuccessRate() != expectedRate {
		t.Errorf("Expected success rate %.2f, got %.2f", expectedRate, stats.SuccessRate())
	}
}

func TestTracker_PerformanceScore(t *testing.T) {
	tracker := NewTracker("")

	// High performer
	tracker.RecordOutcome(ActionOutcome{
		ID:         "high-1",
		Timestamp:  time.Now(),
		ActionType: ActionWriteFile,
		Success:    true,
	})
	tracker.RecordOutcome(ActionOutcome{
		ID:         "high-2",
		Timestamp:  time.Now(),
		ActionType: ActionWriteFile,
		Success:    true,
	})

	// Low performer
	tracker.RecordOutcome(ActionOutcome{
		ID:         "low-1",
		Timestamp:  time.Now(),
		ActionType: ActionRunCommand,
		Success:    false,
	})
	tracker.RecordOutcome(ActionOutcome{
		ID:         "low-2",
		Timestamp:  time.Now(),
		ActionType: ActionRunCommand,
		Success:    false,
	})

	writeStats := tracker.GetStats(ActionWriteFile)
	cmdStats := tracker.GetStats(ActionRunCommand)

	if writeStats.PerformanceScore() <= cmdStats.PerformanceScore() {
		t.Error("Expected write_file to have higher performance score")
	}
}

func TestTracker_GetPatterns(t *testing.T) {
	tracker := NewTracker("")

	// Record similar failures to create a pattern
	for i := 0; i < 3; i++ {
		tracker.RecordOutcome(ActionOutcome{
			ID:         "fail-" + string(rune(i)),
			Timestamp:  time.Now(),
			ActionType: ActionWriteFile,
			Success:    false,
			Error:      "permission denied: /etc/protected",
			Context:    "Write to protected directory",
		})
	}

	patterns := tracker.GetPatterns()
	if len(patterns) == 0 {
		t.Error("Expected at least one pattern to be detected")
	}
}

func TestToolStats_SuccessRate(t *testing.T) {
	tests := []struct {
		name       string
		stats      ToolStats
		expectRate float64
	}{
		{
			name: "all successful",
			stats: ToolStats{
				TotalAttempts: 10,
				Successful:    10,
			},
			expectRate: 100.0,
		},
		{
			name: "half successful",
			stats: ToolStats{
				TotalAttempts: 10,
				Successful:    5,
			},
			expectRate: 50.0,
		},
		{
			name: "no attempts",
			stats: ToolStats{
				TotalAttempts: 0,
				Successful:    0,
			},
			expectRate: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate := tt.stats.SuccessRate()
			if rate != tt.expectRate {
				t.Errorf("Expected %.2f, got %.2f", tt.expectRate, rate)
			}
		})
	}
}

func TestToolStats_AverageDuration(t *testing.T) {
	stats := ToolStats{
		TotalAttempts: 3,
		TotalDuration: 300, // 300ms total
	}

	avg := stats.AverageDuration()
	expected := 100 * time.Millisecond

	if avg != expected {
		t.Errorf("Expected %v, got %v", expected, avg)
	}
}

func TestTracker_GetRankedTools(t *testing.T) {
	tracker := NewTracker("")

	// Record outcomes for multiple tools
	tracker.RecordOutcome(ActionOutcome{
		ID:         "read-1",
		ActionType: ActionReadFile,
		Success:    true,
	})
	tracker.RecordOutcome(ActionOutcome{
		ID:         "write-1",
		ActionType: ActionWriteFile,
		Success:    true,
	})
	tracker.RecordOutcome(ActionOutcome{
		ID:         "write-2",
		ActionType: ActionWriteFile,
		Success:    true,
	})
	tracker.RecordOutcome(ActionOutcome{
		ID:         "cmd-1",
		ActionType: ActionRunCommand,
		Success:    false,
	})

	ranked := tracker.GetRankedTools()
	if len(ranked) != 3 {
		t.Errorf("Expected 3 tools, got %d", len(ranked))
	}

	// run_command should be ranked lowest (0 successes)
	if ranked[len(ranked)-1].ActionType != ActionRunCommand {
		t.Errorf("Expected run_command to be ranked last, got %s", ranked[len(ranked)-1].ActionType)
	}
}
