package tool

import (
	"encoding/json"
	"testing"
)

func TestDoomDetector_NoDoom(t *testing.T) {
	det := DefaultDoomDetector()

	// Different tool calls should not trigger doom
	input1, _ := json.Marshal(map[string]string{"path": "file1.go"})
	input2, _ := json.Marshal(map[string]string{"path": "file2.go"})
	input3, _ := json.Marshal(map[string]string{"command": "go build"})

	result1 := det.Check("read_file", input1)
	if result1.IsDoom {
		t.Errorf("Expected no doom for first call, got %v", result1)
	}

	result2 := det.Check("read_file", input2)
	if result2.IsDoom {
		t.Errorf("Expected no doom for second call, got %v", result2)
	}

	result3 := det.Check("run_command", input3)
	if result3.IsDoom {
		t.Errorf("Expected no doom for third call, got %v", result3)
	}
}

func TestDoomDetector_TriggersAfterThreshold(t *testing.T) {
	det := NewDoomDetector(20, 3)

	input, _ := json.Marshal(map[string]string{"path": "test.go"})

	// First two calls should not trigger
	result1 := det.Check("read_file", input)
	if result1.IsDoom {
		t.Errorf("Expected no doom on first call")
	}

	result2 := det.Check("read_file", input)
	if result2.IsDoom {
		t.Errorf("Expected no doom on second call, got repeat count %d", result2.RepeatCount)
	}

	// Third call should trigger doom
	result3 := det.Check("read_file", input)
	if !result3.IsDoom {
		t.Errorf("Expected doom on third call, got %v", result3)
	}
	if result3.RepeatCount != 3 {
		t.Errorf("Expected repeat count 3, got %d", result3.RepeatCount)
	}
}

func TestDoomDetector_WindowExpiry(t *testing.T) {
	det := NewDoomDetector(5, 3)

	input, _ := json.Marshal(map[string]string{"path": "test.go"})

	// Call 3 times to trigger doom
	det.Check("read_file", input)
	det.Check("read_file", input)
	result3 := det.Check("read_file", input)
	if !result3.IsDoom {
		t.Errorf("Expected doom on third call")
	}

	// Add 5 different calls to push old calls out of window
	for i := 0; i < 5; i++ {
		differentInput, _ := json.Marshal(map[string]int{"i": i})
		det.Check("read_file", differentInput)
	}

	// Now the same call should not trigger doom (old calls expired)
	resultAfterExpiry := det.Check("read_file", input)
	if resultAfterExpiry.IsDoom {
		t.Errorf("Expected no doom after window expiry, got %v", resultAfterExpiry)
	}
	if resultAfterExpiry.RepeatCount != 1 {
		t.Errorf("Expected repeat count 1 after expiry, got %d", resultAfterExpiry.RepeatCount)
	}
}

func TestDoomDetector_DifferentInputs(t *testing.T) {
	det := DefaultDoomDetector()

	// Same tool, different inputs should not be considered doom
	input1, _ := json.Marshal(map[string]string{"path": "file1.go"})
	input2, _ := json.Marshal(map[string]string{"path": "file2.go"})

	det.Check("read_file", input1)
	det.Check("read_file", input1)
	resultSame := det.Check("read_file", input1)
	if !resultSame.IsDoom {
		t.Errorf("Expected doom after 3 identical calls, got %v", resultSame)
	}

	// Different input resets the count for that specific hash
	resultDifferent := det.Check("read_file", input2)
	if resultDifferent.IsDoom {
		t.Errorf("Expected no doom for different input, got %v", resultDifferent)
	}
}

func TestDoomDetector_Reset(t *testing.T) {
	det := NewDoomDetector(20, 3)

	input, _ := json.Marshal(map[string]string{"path": "test.go"})

	// Trigger doom
	det.Check("read_file", input)
	det.Check("read_file", input)
	result3 := det.Check("read_file", input)
	if !result3.IsDoom {
		t.Errorf("Expected doom on third call")
	}

	// Reset should clear state
	det.Reset()

	// Same call should not trigger doom after reset
	resultAfterReset := det.Check("read_file", input)
	if resultAfterReset.IsDoom {
		t.Errorf("Expected no doom after reset, got %v", resultAfterReset)
	}
	if resultAfterReset.RepeatCount != 1 {
		t.Errorf("Expected repeat count 1 after reset, got %d", resultAfterReset.RepeatCount)
	}
}

func TestDoomDetector_MultipleTools(t *testing.T) {
	det := NewDoomDetector(20, 3)

	// Alternate between two tools with DIFFERENT inputs each time
	for i := 0; i < 10; i++ {
		input1, _ := json.Marshal(map[string]any{"path": "file.go", "i": i})
		input2, _ := json.Marshal(map[string]int{"iteration": i})

		result1 := det.Check("read_file", input1)
		result2 := det.Check("run_command", input2)

		if result1.IsDoom {
			t.Errorf("Expected no doom for read_file at iteration %d, got %v", i, result1)
		}
		if result2.IsDoom {
			t.Errorf("Expected no doom for run_command at iteration %d, got %v", i, result2)
		}
	}
}

func TestDoomDetector_SameToolDifferentArgs(t *testing.T) {
	det := NewDoomDetector(20, 3)

	// Same tool, same file path = doom
	input1, _ := json.Marshal(map[string]string{"path": "test.go"})
	det.Check("read_file", input1)
	det.Check("read_file", input1)
	result := det.Check("read_file", input1)
	if !result.IsDoom {
		t.Errorf("Expected doom for repeated identical calls")
	}

	// Same tool, different file path = no doom (different hash)
	det2 := NewDoomDetector(20, 3)
	inputA, _ := json.Marshal(map[string]string{"path": "fileA.go"})
	inputB, _ := json.Marshal(map[string]string{"path": "fileB.go"})
	inputC, _ := json.Marshal(map[string]string{"path": "fileC.go"})

	det2.Check("read_file", inputA)
	det2.Check("read_file", inputB)
	resultC := det2.Check("read_file", inputC)
	if resultC.IsDoom {
		t.Errorf("Expected no doom when reading different files")
	}
}
