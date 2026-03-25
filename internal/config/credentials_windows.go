//go:build windows

package config

import (
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle = kernel32.NewProc("GetStdHandle")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
)

const (
	stdInputHandle   = ^uintptr(10) + 1 // -10
	enableEchoInput  = 0x0004
	enableLineInput  = 0x0002
)

// loadFromWindowsCredential loads a credential from Windows Credential Manager.
func loadFromWindowsCredential(provider string) (string, error) {
	// Windows Credential Manager via wincred or cmdkey
	// For now, fall back to encrypted file
	return loadFromEncryptedFile(provider)
}

// saveToWindowsCredential saves a credential to Windows Credential Manager.
func saveToWindowsCredential(provider, key string) error {
	// Windows Credential Manager via wincred or cmdkey
	// For now, fall back to encrypted file
	return saveToEncryptedFile(provider, key)
}

// readPasswordWindows reads a password from stdin without echo on Windows.
func readPasswordWindows() (string, error) {
	// Try to disable console echo using Windows API
	handle, _, _ := procGetStdHandle.Call(uintptr(stdInputHandle))
	if handle != 0 && handle != ^uintptr(0) {
		var oldMode uint32
		ret, _, _ := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&oldMode)))
		if ret != 0 {
			// Disable echo
			newMode := oldMode &^ (enableEchoInput | enableLineInput)
			procSetConsoleMode.Call(handle, uintptr(newMode))
			defer procSetConsoleMode.Call(handle, uintptr(oldMode))
		}
	}

	// Read password
	return readPasswordUnix()
}
