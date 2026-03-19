# Marcus Tools

Drop manifest-defined tools in subfolders under `.marcus/tools/`.

Each tool folder can contain:

- `tool.toml` with metadata, schema, timeout, and permissions
- `run.ps1`, `run.cmd`, `run.bat`, or `run.sh` as the executable entrypoint

Marcus passes tool input as JSON in the `MARCUS_TOOL_INPUT` environment variable and exposes the project root in `MARCUS_PROJECT_ROOT`.

Example:

```toml
type = "shell"
description = "List Python files in the repo"
timeout = 10
permissions = ["read", "inspect"]

[tool]
name = "list_python_files"
description = "List Python files in the project"

[schema]
type = "object"
```
