package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type authTokenVault struct {
	CredentialPool map[string][]vaultCredential `json:"credential_pool"`
}

type vaultCredential struct {
	AccessToken string `json:"access_token"`
	APIKey      string `json:"api_key"`
	Token       string `json:"token"`
}

type providerTokenFile struct {
	AccessToken string `json:"access_token"`
	Access      string `json:"access"`
	APIKey      string `json:"api_key"`
	Token       string `json:"token"`
	Refresh     string `json:"refresh"`
	Expires     int64  `json:"expires"`
	Email       string `json:"email"`
}

// AuthTokenVaultPath returns the JSON token-vault path used for provider-wide
// credential pools. The file lives under XDG_DATA_HOME because OAuth/device
// credentials are mutable runtime state rather than static config.
func AuthTokenVaultPath() string {
	return filepath.Join(xdgDataHome(), "gormes", "auth.json")
}

// ProviderTokenPath returns the provider-scoped token file path for providers
// that refresh OAuth/device credentials independently of the shared auth.json
// pool. The filename shape mirrors the upstream provider intent while living
// under Gormes' XDG data root.
func ProviderTokenPath(provider string) string {
	return filepath.Join(xdgDataHome(), "gormes", "auth", providerTokenFilename(provider))
}

func resolveTokenVault(cfg *Config) error {
	if strings.TrimSpace(cfg.Hermes.APIKey) != "" {
		return nil
	}

	provider := normalizeTokenVaultProvider(cfg.Hermes.Provider)
	if provider == "" {
		return nil
	}

	if token, err := loadProviderToken(provider); err != nil {
		return err
	} else if token != "" {
		cfg.Hermes.APIKey = token
		return nil
	}

	token, err := loadAuthVaultToken(provider)
	if err != nil {
		return err
	}
	if token != "" {
		cfg.Hermes.APIKey = token
	}
	return nil
}

func loadProviderToken(provider string) (string, error) {
	path := ProviderTokenPath(provider)
	var tokenFile providerTokenFile
	if err := readJSONFile(path, &tokenFile); err != nil {
		return "", err
	}
	if normalizeTokenVaultProvider(provider) == "google-gemini-cli" {
		return loadGoogleOAuthProviderToken(path, tokenFile)
	}
	return firstNonEmpty(tokenFile.AccessToken, tokenFile.Access, tokenFile.APIKey, tokenFile.Token), nil
}

func loadAuthVaultToken(provider string) (string, error) {
	path := AuthTokenVaultPath()
	var vault authTokenVault
	if err := readJSONFile(path, &vault); err != nil {
		return "", err
	}

	for _, key := range authVaultLookupKeys(provider) {
		for _, credential := range vault.CredentialPool[key] {
			if token := firstNonEmpty(credential.AccessToken, credential.APIKey, credential.Token); token != "" {
				return token, nil
			}
		}
	}
	return "", nil
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func authVaultLookupKeys(provider string) []string {
	normalized := normalizeTokenVaultProvider(provider)
	if normalized == "" {
		return nil
	}

	keys := []string{normalized}
	switch normalized {
	case "google-gemini-cli":
		return append(keys, "google-code-assist", "google_code_assist", "gemini-cli", "gemini-oauth")
	case "codex":
		return append(keys, "openai-codex", "openai_codex")
	case "gemini":
		return append(keys, "google-gemini", "google_gemini")
	case "bedrock":
		return append(keys, "aws-bedrock", "amazon-bedrock")
	default:
		return keys
	}
}

func providerTokenFilename(provider string) string {
	switch normalizeTokenVaultProvider(provider) {
	case "google-gemini-cli":
		return "google_oauth.json"
	case "anthropic":
		return "anthropic_oauth.json"
	case "codex":
		return "codex_oauth.json"
	default:
		return sanitizeProviderFilename(provider) + ".json"
	}
}

func normalizeTokenVaultProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "openai", "openai-compatible", "openai_compatible", "hermes":
		return "openai"
	case "anthropic":
		return "anthropic"
	case "bedrock", "aws-bedrock", "amazon-bedrock":
		return "bedrock"
	case "gemini", "google-gemini", "google_gemini":
		return "gemini"
	case "openrouter":
		return "openrouter"
	case "google-gemini-cli", "gemini-cli", "gemini-oauth", "google-code-assist", "google_code_assist":
		return "google-gemini-cli"
	case "codex", "openai-codex", "openai_codex":
		return "codex"
	default:
		return sanitizeProviderFilename(provider)
	}
}

func sanitizeProviderFilename(provider string) string {
	name := strings.ToLower(strings.TrimSpace(provider))
	if name == "" {
		return "provider"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", "-", "_")
	return replacer.Replace(name)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
