package tui

import (
	"fmt"
	"sync"
	"time"
)

// BadgeType represents the type of badge
type BadgeType string

const (
	BadgeSuccess  BadgeType = "success"
	BadgeError    BadgeType = "error"
	BadgeWarning  BadgeType = "warning"
	BadgeInfo     BadgeType = "info"
	BadgeProgress BadgeType = "progress"
)

// Badge represents a visual indicator for an operation
type Badge struct {
	Type      BadgeType
	Message   string
	Timestamp time.Time
	ActionID  string     // Links badge to a specific action
	ExpiresAt *time.Time // Optional expiration time
}

// BadgeManager manages badges for the TUI
type BadgeManager struct {
	badges []Badge
	mu     sync.RWMutex
}

// NewBadgeManager creates a new badge manager
func NewBadgeManager() *BadgeManager {
	return &BadgeManager{
		badges: make([]Badge, 0),
	}
}

// Add adds a badge to the manager
func (bm *BadgeManager) Add(badge Badge) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if badge.Timestamp.IsZero() {
		badge.Timestamp = time.Now()
	}

	bm.badges = append(bm.badges, badge)
}

// AddSuccess adds a success badge
func (bm *BadgeManager) AddSuccess(message, actionID string) {
	bm.Add(Badge{
		Type:     BadgeSuccess,
		Message:  message,
		ActionID: actionID,
	})
}

// AddError adds an error badge
func (bm *BadgeManager) AddError(message, actionID string) {
	bm.Add(Badge{
		Type:     BadgeError,
		Message:  message,
		ActionID: actionID,
	})
}

// AddWarning adds a warning badge
func (bm *BadgeManager) AddWarning(message, actionID string) {
	bm.Add(Badge{
		Type:     BadgeWarning,
		Message:  message,
		ActionID: actionID,
	})
}

// AddInfo adds an info badge
func (bm *BadgeManager) AddInfo(message, actionID string) {
	bm.Add(Badge{
		Type:     BadgeInfo,
		Message:  message,
		ActionID: actionID,
	})
}

// AddProgress adds a progress badge
func (bm *BadgeManager) AddProgress(message, actionID string) {
	bm.Add(Badge{
		Type:     BadgeProgress,
		Message:  message,
		ActionID: actionID,
	})
}

// Clear removes all badges
func (bm *BadgeManager) Clear() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.badges = make([]Badge, 0)
}

// ClearExpired removes badges that have expired
func (bm *BadgeManager) ClearExpired() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	now := time.Now()
	var active []Badge
	for _, badge := range bm.badges {
		if badge.ExpiresAt == nil || now.Before(*badge.ExpiresAt) {
			active = append(active, badge)
		}
	}
	bm.badges = active
}

// ForAction returns badges for a specific action
func (bm *BadgeManager) ForAction(actionID string) []Badge {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	var result []Badge
	for _, badge := range bm.badges {
		if badge.ActionID == actionID {
			result = append(result, badge)
		}
	}
	return result
}

// Latest returns the most recent badges (up to limit)
func (bm *BadgeManager) Latest(limit int) []Badge {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	if limit <= 0 || limit > len(bm.badges) {
		limit = len(bm.badges)
	}

	// Return a copy of the latest badges
	start := len(bm.badges) - limit
	if start < 0 {
		start = 0
	}

	result := make([]Badge, limit)
	copy(result, bm.badges[start:])
	return result
}

// Count returns the total number of badges
func (bm *BadgeManager) Count() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return len(bm.badges)
}

// BadgeIcon returns the icon for a badge type
func BadgeIcon(t BadgeType) string {
	switch t {
	case BadgeSuccess:
		return "✓"
	case BadgeError:
		return "✗"
	case BadgeWarning:
		return "⚠"
	case BadgeInfo:
		return "ℹ"
	case BadgeProgress:
		return "◌"
	default:
		return "•"
	}
}

// RenderBadge renders a badge with the given styles
func RenderBadge(badge Badge, styles Styles) string {
	icon := BadgeIcon(badge.Type)
	style := styles.Badge(badge.Type)

	return style.Render(fmt.Sprintf("%s %s", icon, badge.Message))
}
