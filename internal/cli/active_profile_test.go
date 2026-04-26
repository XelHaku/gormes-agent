package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadActiveProfile_UnsetWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active_profile")

	got, err := ReadActiveProfile(path)
	if !errors.Is(err, ErrActiveProfileUnset) {
		t.Fatalf("ReadActiveProfile(missing) err = %v, want ErrActiveProfileUnset", err)
	}
	if got != "" {
		t.Fatalf("ReadActiveProfile(missing) name = %q, want empty string", got)
	}
}

func TestWriteActiveProfile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active_profile")

	if err := WriteActiveProfile(path, "coder"); err != nil {
		t.Fatalf("WriteActiveProfile err = %v, want nil", err)
	}

	got, err := ReadActiveProfile(path)
	if err != nil {
		t.Fatalf("ReadActiveProfile err = %v, want nil", err)
	}
	if want := "coder"; got != want {
		t.Fatalf("ReadActiveProfile = %q, want %q", got, want)
	}
}

func TestWriteActiveProfile_RejectsInvalidName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active_profile")

	err := WriteActiveProfile(path, "Bad Name")
	if !errors.Is(err, ErrProfileNameInvalidChars) {
		t.Fatalf("WriteActiveProfile(Bad Name) err = %v, want ErrProfileNameInvalidChars", err)
	}

	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("active profile file stat err = %v, want IsNotExist", statErr)
	}
}

func TestWriteActiveProfile_AtomicTempRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active_profile")
	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, []byte("stale-sentinel"), 0o600); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}

	if err := WriteActiveProfile(path, "coder"); err != nil {
		t.Fatalf("WriteActiveProfile err = %v, want nil", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(path) err = %v, want nil", err)
	}
	if want := []byte("coder"); string(data) != string(want) {
		t.Fatalf("file contents = %q, want %q (byte-for-byte)", data, want)
	}

	if _, statErr := os.Stat(tmp); !os.IsNotExist(statErr) {
		t.Fatalf("temp file stat err = %v, want IsNotExist (rename should consume it)", statErr)
	}
}

func TestClearActiveProfile_IdempotentMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active_profile")

	if err := ClearActiveProfile(path); err != nil {
		t.Fatalf("ClearActiveProfile(missing) err = %v, want nil", err)
	}
}

func TestClearActiveProfile_RemovesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active_profile")

	if err := WriteActiveProfile(path, "coder"); err != nil {
		t.Fatalf("WriteActiveProfile err = %v, want nil", err)
	}
	if err := ClearActiveProfile(path); err != nil {
		t.Fatalf("ClearActiveProfile err = %v, want nil", err)
	}

	got, err := ReadActiveProfile(path)
	if !errors.Is(err, ErrActiveProfileUnset) {
		t.Fatalf("ReadActiveProfile after clear err = %v, want ErrActiveProfileUnset", err)
	}
	if got != "" {
		t.Fatalf("ReadActiveProfile after clear name = %q, want empty string", got)
	}
}
