package cli

import (
	"errors"
	"fmt"
	"strings"
)

// CustomProviderRef is the pure input model describing a custom provider's
// credential configuration as it appears on disk or in memory.
type CustomProviderRef struct {
	Name    string
	BaseURL string
	APIKey  string
	KeyEnv  string
}

// CustomProviderResolution carries the result of resolving a custom provider
// credential. EffectiveSecret is the cleartext used for outbound calls;
// PersistAsRef is the value that should be written back to config so that
// references (env templates) are never replaced with plaintext. Evidence
// labels how the resolution was reached so callers can branch without
// inspecting strings for "${" prefixes.
type CustomProviderResolution struct {
	EffectiveSecret string
	PersistAsRef    string
	Evidence        string
}

// ErrCustomProviderEnvUnset signals that an env-template ${VAR} reference was
// supplied but the named variable is missing or empty in the environment map.
var ErrCustomProviderEnvUnset = errors.New("custom provider env reference is unset")

// ErrCustomProviderCredentialMissing signals that neither APIKey nor KeyEnv
// was supplied, so no credential could be resolved.
var ErrCustomProviderCredentialMissing = errors.New("custom provider credential missing")

// ResolveCustomProviderSecret resolves a custom provider credential without
// touching the filesystem, network, or process environment. The function
// preserves env-template references (`${VAR}`) in PersistAsRef so callers can
// persist the reference back to config without leaking plaintext, while still
// returning the resolved EffectiveSecret for outbound calls.
//
// Resolution order:
//  1. APIKey holding a `${VAR}` env template -> resolve via env, preserve ref.
//  2. APIKey holding plaintext -> use as-is, persist as plaintext.
//  3. APIKey empty + KeyEnv set -> resolve via env, persist as `${KeyEnv}` ref.
//  4. APIKey empty + KeyEnv empty -> credential missing.
func ResolveCustomProviderSecret(ref CustomProviderRef, env map[string]string) (CustomProviderResolution, error) {
	apiKey := strings.TrimSpace(ref.APIKey)
	keyEnv := strings.TrimSpace(ref.KeyEnv)

	if envName, ok := envTemplateName(apiKey); ok {
		ref := apiKey
		secret, present := env[envName]
		if !present || secret == "" {
			return CustomProviderResolution{
				PersistAsRef: ref,
				Evidence:     "env_var_unset",
			}, fmt.Errorf("%w: %s", ErrCustomProviderEnvUnset, envName)
		}
		return CustomProviderResolution{
			EffectiveSecret: secret,
			PersistAsRef:    ref,
			Evidence:        "secret_ref_preserved",
		}, nil
	}

	if apiKey != "" {
		return CustomProviderResolution{
			EffectiveSecret: apiKey,
			PersistAsRef:    apiKey,
			Evidence:        "plaintext_provided",
		}, nil
	}

	if keyEnv != "" {
		ref := fmt.Sprintf("${%s}", keyEnv)
		secret, present := env[keyEnv]
		if !present || secret == "" {
			return CustomProviderResolution{
				PersistAsRef: ref,
				Evidence:     "env_var_unset",
			}, fmt.Errorf("%w: %s", ErrCustomProviderEnvUnset, keyEnv)
		}
		return CustomProviderResolution{
			EffectiveSecret: secret,
			PersistAsRef:    ref,
			Evidence:        "secret_ref_preserved",
		}, nil
	}

	return CustomProviderResolution{
		Evidence: "credential_missing",
	}, ErrCustomProviderCredentialMissing
}

// envTemplateName returns the inner variable name when value is exactly a
// `${VAR}` template, and a boolean indicating whether the value matches.
func envTemplateName(value string) (string, bool) {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return "", false
	}
	name := strings.TrimSpace(value[2 : len(value)-1])
	if name == "" {
		return "", false
	}
	return name, true
}
