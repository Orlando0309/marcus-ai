package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// DoomDetector detects when the model is repeating identical tool calls
// within the same session, preventing infinite loops.
type DoomDetector struct {
	mu          sync.RWMutex
	window      []string   // Hashes of recent tool calls (circular buffer)
	windowSize  int
	doomThreshold int    // Number of repeats before triggering doom
	hashCounts  map[string]int // Count of each hash in window
}

// DoomResult indicates whether a tool call appears to be a doom loop
type DoomResult struct {
	IsDoom      bool
	RepeatCount int
	Message     string
}

// NewDoomDetector creates a new doom detector with the given window size
// and doom threshold (number of repeats before triggering).
func NewDoomDetector(windowSize, doomThreshold int) *DoomDetector {
	return &DoomDetector{
		window:        make([]string, 0, windowSize),
		windowSize:    windowSize,
		doomThreshold: doomThreshold,
		hashCounts:    make(map[string]int),
	}
}

// Check checks if a tool call appears to be part of a doom loop.
// Returns a DoomResult indicating whether this looks like a repeated call.
func (d *DoomDetector) Check(toolName string, input []byte) DoomResult {
	hash := d.hashToolCall(toolName, input)

	d.mu.Lock()
	defer d.mu.Unlock()

	// Add to window
	if len(d.window) >= d.windowSize {
		// Remove oldest
		oldest := d.window[0]
		d.hashCounts[oldest]--
		if d.hashCounts[oldest] <= 0 {
			delete(d.hashCounts, oldest)
		}
		d.window = d.window[1:]
	}

	d.window = append(d.window, hash)
	d.hashCounts[hash]++

	count := d.hashCounts[hash]
	if count >= d.doomThreshold {
		return DoomResult{
			IsDoom:      true,
			RepeatCount: count,
			Message:     "Doom loop detected: this tool call has been repeated %d times in the last %d calls",
		}
	}

	return DoomResult{
		IsDoom:      false,
		RepeatCount: count,
		Message:     "",
	}
}

// Reset clears the detector state
func (d *DoomDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.window = d.window[:0]
	d.hashCounts = make(map[string]int)
}

// hashToolCall creates a hash of a tool call (name + input)
func (d *DoomDetector) hashToolCall(toolName string, input []byte) string {
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write([]byte{0x00}) // Separator
	h.Write(input)
	hash := h.Sum(nil)
	return hex.EncodeToString(hash)
}

// DefaultDoomDetector returns a detector with sensible defaults:
// - Window size: 20 (last 20 tool calls)
// - Doom threshold: 3 (trigger after 3 repeats)
func DefaultDoomDetector() *DoomDetector {
	return NewDoomDetector(20, 3)
}
