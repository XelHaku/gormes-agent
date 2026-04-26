package cli

import (
	"errors"
	"testing"
)

func TestResolveCustomProviderSecret_EnvTemplatePreserved(t *testing.T) {
	ref := CustomProviderRef{Name: "acme", APIKey: "${ACME_KEY}"}
	env := map[string]string{"ACME_KEY": "sk-real"}

	got, err := ResolveCustomProviderSecret(ref, env)
	if err != nil {
		t.Fatalf("ResolveCustomProviderSecret returned error: %v", err)
	}
	want := CustomProviderResolution{
		EffectiveSecret: "sk-real",
		PersistAsRef:    "${ACME_KEY}",
		Evidence:        "secret_ref_preserved",
	}
	if got != want {
		t.Fatalf("ResolveCustomProviderSecret = %+v, want %+v", got, want)
	}
}

func TestResolveCustomProviderSecret_KeyEnvFallback(t *testing.T) {
	ref := CustomProviderRef{Name: "acme", APIKey: "", KeyEnv: "ACME_KEY"}
	env := map[string]string{"ACME_KEY": "sk-real"}

	got, err := ResolveCustomProviderSecret(ref, env)
	if err != nil {
		t.Fatalf("ResolveCustomProviderSecret returned error: %v", err)
	}
	want := CustomProviderResolution{
		EffectiveSecret: "sk-real",
		PersistAsRef:    "${ACME_KEY}",
		Evidence:        "secret_ref_preserved",
	}
	if got != want {
		t.Fatalf("ResolveCustomProviderSecret = %+v, want %+v", got, want)
	}
}

func TestResolveCustomProviderSecret_PlaintextProvided(t *testing.T) {
	ref := CustomProviderRef{Name: "acme", APIKey: "sk-plain"}
	env := map[string]string{}

	got, err := ResolveCustomProviderSecret(ref, env)
	if err != nil {
		t.Fatalf("ResolveCustomProviderSecret returned error: %v", err)
	}
	want := CustomProviderResolution{
		EffectiveSecret: "sk-plain",
		PersistAsRef:    "sk-plain",
		Evidence:        "plaintext_provided",
	}
	if got != want {
		t.Fatalf("ResolveCustomProviderSecret = %+v, want %+v", got, want)
	}
}

func TestResolveCustomProviderSecret_EnvVarUnset(t *testing.T) {
	ref := CustomProviderRef{Name: "acme", APIKey: "${ACME_KEY}"}
	env := map[string]string{}

	got, err := ResolveCustomProviderSecret(ref, env)
	if err == nil {
		t.Fatalf("ResolveCustomProviderSecret returned nil error, want ErrCustomProviderEnvUnset")
	}
	if !errors.Is(err, ErrCustomProviderEnvUnset) {
		t.Fatalf("ResolveCustomProviderSecret error = %v, want errors.Is(_, ErrCustomProviderEnvUnset)", err)
	}
	want := CustomProviderResolution{
		EffectiveSecret: "",
		PersistAsRef:    "${ACME_KEY}",
		Evidence:        "env_var_unset",
	}
	if got != want {
		t.Fatalf("ResolveCustomProviderSecret = %+v, want %+v", got, want)
	}
}

func TestResolveCustomProviderSecret_BothEmpty(t *testing.T) {
	ref := CustomProviderRef{Name: "acme", APIKey: "", KeyEnv: ""}
	env := map[string]string{}

	got, err := ResolveCustomProviderSecret(ref, env)
	if err == nil {
		t.Fatalf("ResolveCustomProviderSecret returned nil error, want ErrCustomProviderCredentialMissing")
	}
	if !errors.Is(err, ErrCustomProviderCredentialMissing) {
		t.Fatalf("ResolveCustomProviderSecret error = %v, want errors.Is(_, ErrCustomProviderCredentialMissing)", err)
	}
	want := CustomProviderResolution{
		EffectiveSecret: "",
		PersistAsRef:    "",
		Evidence:        "credential_missing",
	}
	if got != want {
		t.Fatalf("ResolveCustomProviderSecret = %+v, want %+v", got, want)
	}
}
