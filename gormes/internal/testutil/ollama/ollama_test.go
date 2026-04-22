package ollama

import "testing"

func TestEnabled_DefaultFalse(t *testing.T) {
	t.Setenv(RunEnvVar, "")
	if Enabled() {
		t.Fatal("Enabled() = true, want false when env var is unset")
	}
}

func TestEnabled_TrueValues(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE"} {
		t.Setenv(RunEnvVar, value)
		if !Enabled() {
			t.Fatalf("Enabled() = false, want true for %q", value)
		}
	}
}

func TestEnabled_InvalidValueFalse(t *testing.T) {
	t.Setenv(RunEnvVar, "definitely-not-bool")
	if Enabled() {
		t.Fatal("Enabled() = true, want false for invalid boolean")
	}
}

func TestEndpoint_DefaultAndOverride(t *testing.T) {
	t.Setenv(EndpointEnvVar, "")
	if got := Endpoint(); got != DefaultEndpoint {
		t.Fatalf("Endpoint() = %q, want %q", got, DefaultEndpoint)
	}

	t.Setenv(EndpointEnvVar, " http://127.0.0.1:1234 ")
	if got := Endpoint(); got != "http://127.0.0.1:1234" {
		t.Fatalf("Endpoint() = %q, want override", got)
	}
}

func TestModel_DefaultAndOverride(t *testing.T) {
	t.Setenv(ModelEnvVar, "")
	if got := Model(); got != DefaultModel {
		t.Fatalf("Model() = %q, want %q", got, DefaultModel)
	}

	t.Setenv(ModelEnvVar, " qwen2.5:3b ")
	if got := Model(); got != "qwen2.5:3b" {
		t.Fatalf("Model() = %q, want override", got)
	}
}
