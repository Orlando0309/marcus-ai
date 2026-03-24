package tool

import (
	"github.com/marcus-ai/marcus/internal/safety"
)

func (tr *ToolRunner) validateActionProposalPaths(p ActionProposal) error {
	switch p.Type {
	case "write_file", "read_file", "patch_file", "edit_file", "delete_file", "create_file",
		"glob_files", "list_directory", "list_files":
		if _, err := tr.resolvePath(p.Path); err != nil {
			return err
		}
	case "search_code":
		if p.Path != "" {
			if _, err := tr.resolvePath(p.Path); err != nil {
				return err
			}
		}
	case "find_symbol":
		if p.Path != "" {
			if _, err := tr.resolvePath(p.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (tr *ToolRunner) validateActionProposal(p ActionProposal) error {
	if err := tr.validateActionProposalPaths(p); err != nil {
		return err
	}
	if p.Type != "run_command" {
		return nil
	}
	if err := safety.ValidateRunCommand(p.Command); err != nil {
		return err
	}
	if p.Dir != "" {
		if _, err := tr.resolvePath(p.Dir); err != nil {
			return err
		}
	}
	return safety.ValidateRunCommandPolicy(p.Command, tr.commandPolicy)
}
