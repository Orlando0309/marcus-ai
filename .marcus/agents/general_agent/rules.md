# General Agent — Partition Rules

## Safe actions (always auto-run)
- `list_files` — browse directory
- `read_file` — read any file
- `search_code` — search files
- `find_symbol` — find definitions

## Auto-run commands
None — all run_command actions require user approval.

## Write file policy
`write_if = "never"` — any write_file requires explicit user approval.

## Verification
`verification = "never"` — no automatic build verification.

## Step mode
`step_mode = true` — pause between iterations so the user can follow along and approve actions.
