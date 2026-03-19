# Coding Agent — Partition Rules

These rules define which actions Marcus auto-runs vs. asks approval for.

## Safe actions (always auto-run)
- `list_files` — browse directory structure
- `read_file` — inspect file contents
- `search_code` — regex search across files
- `find_symbol` — find a function/type definition
- `list_symbols` — list symbols in a file

## Auto-run commands (no approval needed)
Any `run_command` that starts with one of these prefixes is auto-approved:
- `go build`
- `go build ./...`
- `cargo build`
- `cargo build 2>&1`
- `npm run build`
- `npm run build 2>&1`
- `ruff check`
- `ruff check .`
- `ruff format`
- `go test`
- `go test ./...`
- `go vet`
- `golangci-lint run`
- `python -m py_compile`
- `mvn compile`
- `gradle build`
- `make`
- `cmake --build`
- Any `go run` that runs a specific file (not a general `go run`)

## Write file policy
- `write_if = "first_wave"` — auto-run when there is exactly ONE write_file action and NO run_command in the same batch
- If there are multiple writes or a write + run_command mix, all writes go to pending (ask user approval)

## Verification
- `verification = "detect"` — after any write_file, detect the project type and run the appropriate build command automatically
- If `verification = "always"`, always run verification after writes
- If `verification = "never"`, skip verification after writes
- To override the detected command, add a specific command: e.g. `verification = "go test ./..."`

## Step mode
- `step_mode = false` — by default, run autonomously without pausing
- If `step_mode = true`, pause after each iteration and wait for user to press Space to continue
