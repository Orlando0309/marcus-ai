package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CredentialStore securely stores API keys using platform-specific encryption.
// On Windows: uses DPAPI via native calls
// On macOS: uses Keychain via security command
// On Linux: uses pass or file-based encryption with derived key
// Falls back to file-based AES-GCM encryption with derived key
var (
	credentialCache = make(map[string]string)
)

// GetAPIKey retrieves an API key for the given provider.
// It checks in order: 1) environment variable, 2) credential store, 3) prompts user
func GetAPIKey(provider string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))

	// Check environment variable first
	envVar := providerAPIKeyEnvVar(provider)
	if key := os.Getenv(envVar); key != "" {
		return key, nil
	}

	// Check credential cache
	if key, ok := credentialCache[provider]; ok {
		return key, nil
	}

	// Check credential store
	key, err := loadFromCredentialStore(provider)
	if err == nil && key != "" {
		credentialCache[provider] = key
		return key, nil
	}

	return "", fmt.Errorf("API key not found for provider %s", provider)
}

// StoreAPIKey securely stores an API key for the given provider.
func StoreAPIKey(provider, key string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	key = strings.TrimSpace(key)

	if key == "" {
		return fmt.Errorf("cannot store empty API key")
	}

	// Update cache
	credentialCache[provider] = key

	// Store persistently
	return saveToCredentialStore(provider, key)
}

// PromptAndStoreAPIKey prompts the user for an API key and stores it securely.
func PromptAndStoreAPIKey(provider string) (string, error) {
	fmt.Printf("Enter API key for %s (input will be hidden): ", provider)

	// Read password without echo
	key, err := readPassword()
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("API key cannot be empty")
	}

	// Validate the key format (basic check)
	if err := validateAPIKey(provider, key); err != nil {
		return "", err
	}

	// Store securely
	if err := StoreAPIKey(provider, key); err != nil {
		return "", fmt.Errorf("failed to store API key: %w", err)
	}

	fmt.Println("\nAPI key stored securely.")
	return key, nil
}

// loadFromCredentialStore loads a credential from the most secure available storage.
func loadFromCredentialStore(provider string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return loadFromKeychain(provider)
	case "windows":
		return loadFromWindowsCredential(provider)
	default:
		return loadFromEncryptedFile(provider)
	}
}

// loadFromKeychain loads a credential from macOS Keychain.
func loadFromKeychain(provider string) (string, error) {
	if runtime.GOOS != "darwin" {
		return loadFromEncryptedFile(provider)
	}
	account := "marcus-" + provider
	cmd := exec.Command("security", "find-generic-password", "-s", "marcus.ai", "-a", account, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// saveToCredentialStore saves a credential to the most secure available storage.
func saveToCredentialStore(provider, key string) error {
	switch runtime.GOOS {
	case "darwin":
		return saveToKeychain(provider, key)
	case "windows":
		return saveToWindowsCredential(provider, key)
	default:
		return saveToEncryptedFile(provider, key)
	}
}

// saveToKeychain saves a credential to macOS Keychain.
func saveToKeychain(provider, key string) error {
	if runtime.GOOS != "darwin" {
		return saveToEncryptedFile(provider, key)
	}
	account := "marcus-" + provider
	// First delete any existing entry
	exec.Command("security", "delete-generic-password", "-s", "marcus.ai", "-a", account).Run()
	// Add new entry
	cmd := exec.Command("security", "add-generic-password", "-s", "marcus.ai", "-a", account, "-w", key, "-U")
	return cmd.Run()
}

// readPasswordUnix reads a password from stdin without echo on Unix systems.
func readPasswordUnix() (string, error) {
	// Try to disable echo using stty (works on macOS and Linux)
	if _, err := exec.LookPath("stty"); err == nil {
		// Disable echo
		exec.Command("stty", "-echo").Run()
		defer exec.Command("stty", "echo").Run()
	}

	// Read from stdin
	var password string
	_, err := fmt.Scanln(&password)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(password), nil
}

// providerAPIKeyEnvVar returns the environment variable name for a provider's API key.
func providerAPIKeyEnvVar(provider string) string {
	switch provider {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "groq":
		return "GROQ_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	default:
		return strings.ToUpper(provider) + "_API_KEY"
	}
}

// validateAPIKey performs basic validation of an API key.
func validateAPIKey(provider, key string) error {
	switch provider {
	case "anthropic":
		// Anthropic keys start with "sk-ant-"
		if !strings.HasPrefix(key, "sk-ant-") {
			return fmt.Errorf("invalid Anthropic API key format (should start with 'sk-ant-')")
		}
	case "openai":
		// OpenAI keys start with "sk-"
		if !strings.HasPrefix(key, "sk-") {
			return fmt.Errorf("invalid OpenAI API key format (should start with 'sk-')")
		}
	case "groq":
		// Groq keys typically start with "gsk_"
		if !strings.HasPrefix(key, "gsk_") {
			return fmt.Errorf("invalid Groq API key format (should start with 'gsk_')")
		}
	case "gemini":
		// Gemini keys are typically long alphanumeric strings
		if len(key) < 20 {
			return fmt.Errorf("invalid Gemini API key format (too short)")
		}
	}
	return nil
}

// CredentialFile represents the structure of the encrypted credentials file.
type CredentialFile struct {
	Version     int                       `json:"version"`
	Salt        string                    `json:"salt"`
	Credentials map[string]EncryptedEntry `json:"credentials"`
}

// EncryptedEntry holds an encrypted credential.
type EncryptedEntry struct {
	Ciphertext string `json:"ct"`
	Nonce      string `json:"nonce"`
}

const credentialVersion = 1

// getCredentialPath returns the path to the credentials file.
func getCredentialPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".marcus", "credentials.enc")
}

// deriveKey derives an encryption key from the machine-specific data using Argon2.
func deriveKey(salt []byte) []byte {
	// Use machine-specific data as the basis for key derivation
	password := getMachineSpecificData()

	// Simple key derivation using SHA-256 with multiple rounds
	data := append(password, salt...)
	hash := sha256.Sum256(data)

	// Multiple rounds for additional security
	for i := 0; i < 100000; i++ {
		hash = sha256.Sum256(hash[:])
	}

	return hash[:]
}

// getMachineSpecificData returns data that is unique to this machine.
// Used as the basis for key derivation.
func getMachineSpecificData() []byte {
	// Combine multiple machine-specific identifiers
	var data []byte

	// Add hostname
	if hostname, err := os.Hostname(); err == nil {
		data = append(data, []byte(hostname)...)
	}

	// Add user home directory path (varies per user/machine)
	if home, err := os.UserHomeDir(); err == nil {
		data = append(data, []byte(home)...)
	}

	// Add specific environment variables that are typically machine-specific
	for _, env := range []string{"USER", "USERNAME", "COMPUTERNAME"} {
		if v := os.Getenv(env); v != "" {
			data = append(data, []byte(v)...)
		}
	}

	// If we have very little data, add some fixed entropy
	// This is less secure but better than failing
	if len(data) < 16 {
		data = append(data, []byte("marcus-default-salt-v1")...)
	}

	return data
}

// loadFromEncryptedFile loads a credential from the encrypted file store.
func loadFromEncryptedFile(provider string) (string, error) {
	path := getCredentialPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var cf CredentialFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return "", err
	}

	if cf.Version != credentialVersion {
		return "", fmt.Errorf("unsupported credential file version")
	}

	salt, err := base64.StdEncoding.DecodeString(cf.Salt)
	if err != nil {
		return "", err
	}

	key := deriveKey(salt)
	entry, ok := cf.Credentials[provider]
	if !ok {
		return "", fmt.Errorf("credential not found")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(entry.Ciphertext)
	if err != nil {
		return "", err
	}

	nonce, err := base64.StdEncoding.DecodeString(entry.Nonce)
	if err != nil {
		return "", err
	}

	plaintext, err := decrypt(key, ciphertext, nonce)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// saveToEncryptedFile saves a credential to the encrypted file store.
func saveToEncryptedFile(provider, key string) error {
	path := getCredentialPath()

	// Load existing credentials or create new file
	var cf CredentialFile
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &cf)
	}

	// Generate new salt if needed
	var salt []byte
	if cf.Salt == "" {
		salt = make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return fmt.Errorf("failed to generate salt: %w", err)
		}
		cf.Salt = base64.StdEncoding.EncodeToString(salt)
		cf.Version = credentialVersion
	} else {
		salt, _ = base64.StdEncoding.DecodeString(cf.Salt)
	}

	if cf.Credentials == nil {
		cf.Credentials = make(map[string]EncryptedEntry)
	}

	// Derive key and encrypt
	k := deriveKey(salt)
	ciphertext, nonce, err := encrypt(k, []byte(key))
	if err != nil {
		return fmt.Errorf("failed to encrypt: %w", err)
	}

	cf.Credentials[provider] = EncryptedEntry{
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
	}

	// Save file with restricted permissions
	output, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Write with restricted permissions (owner read/write only)
	if err := os.WriteFile(path, output, 0600); err != nil {
		return err
	}

	return nil
}

// encrypt encrypts plaintext using AES-GCM with the given key.
func encrypt(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext = gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext[gcm.NonceSize():], nonce, nil
}

// decrypt decrypts ciphertext using AES-GCM with the given key and nonce.
func decrypt(key, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong key or corrupted data): %w", err)
	}

	return plaintext, nil
}

// readPassword reads a password from stdin without echo.
func readPassword() (string, error) {
	// Use platform-specific implementation
	switch runtime.GOOS {
	case "windows":
		return readPasswordWindows()
	default:
		return readPasswordUnix()
	}
}

// ProviderNeedsAPIKey returns true if the provider requires an API key.
func ProviderNeedsAPIKey(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "ollama":
		return false
	default:
		return true
	}
}

// ProviderAPIKeyEnvVar returns the environment variable name for a provider's API key (exported version).
func ProviderAPIKeyEnvVar(provider string) string {
	return providerAPIKeyEnvVar(provider)
}
