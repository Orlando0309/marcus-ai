package builtin

import (
	"context"
	"fmt"
	"sync"

	"github.com/marcus-ai/marcus/internal/skill"
	"github.com/marcus-ai/marcus/internal/tool"
)

// UndoSkill reverts the last file batch
type UndoSkill struct {
	undoStack *[]tool.UndoBatch
	undoMu    *sync.Mutex
}

// NewUndoSkill creates a new undo skill with the given undo stack
func NewUndoSkill(undoStack *[]tool.UndoBatch, undoMu *sync.Mutex) *UndoSkill {
	return &UndoSkill{
		undoStack: undoStack,
		undoMu:    undoMu,
	}
}

func (u *UndoSkill) Name() string { return "undo" }

func (u *UndoSkill) Pattern() string { return "/undo" }

func (u *UndoSkill) Description() string {
	return "Revert the last file batch"
}

func (u *UndoSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	u.undoMu.Lock()
	defer u.undoMu.Unlock()

	if len(*u.undoStack) == 0 {
		return skill.Result{
			Message: "Nothing to undo.",
			Done:    true,
		}, nil
	}

	// Pop the last batch
	batch := (*u.undoStack)[len(*u.undoStack)-1]
	*u.undoStack = (*u.undoStack)[:len(*u.undoStack)-1]

	// Restore files using RestoreUndoBatch
	if err := tool.RestoreUndoBatch(batch); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Undo failed: could not restore files: %v", err),
			Done:    true,
		}, nil
	}

	// Count restored files
	restored := len(batch.Entries)

	return skill.Result{
		Message: fmt.Sprintf("Restored %d file(s).", restored),
		Done:    true,
	}, nil
}
