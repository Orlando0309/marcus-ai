//go:build !windows

package config

// readPasswordWindows is defined in credentials_windows.go on Windows.
// This stub is for non-Windows platforms.
func readPasswordWindows() (string, error) {
	return readPasswordUnix()
}
