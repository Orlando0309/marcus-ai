$root = $env:MARCUS_PROJECT_ROOT
if ([string]::IsNullOrWhiteSpace($root)) {
  $root = (Get-Location).Path
}

$files = Get-ChildItem -Path $root -Recurse -File -Include *.py |
  Where-Object { $_.FullName -notmatch '\\.git\\|\\node_modules\\|\\__pycache__\\|\\\.venv\\' } |
  ForEach-Object { $_.FullName.Substring($root.Length).TrimStart('\') -replace '\\','/' }

$result = @{
  files = $files
  count = $files.Count
}

$result | ConvertTo-Json -Depth 5
