package cli

import (
	"errors"
	"fmt"
	"os"
)

// ErrActiveProfileUnset is returned by ReadActiveProfile when the active
// profile file does not exist. Callers can use errors.Is to detect this
// degraded mode and fall back to the default profile without surfacing a
// hard error.
var ErrActiveProfileUnset = errors.New("active profile is unset")

// ReadActiveProfile reads the sticky active profile name from rootFile.
//
// When rootFile does not exist it returns ("", ErrActiveProfileUnset) so
// callers can branch on the sentinel rather than parsing free-form errors.
// Other I/O errors are returned wrapped with %w.
func ReadActiveProfile(rootFile string) (string, error) {
	data, err := os.ReadFile(rootFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrActiveProfileUnset
		}
		return "", fmt.Errorf("read active profile: %w", err)
	}
	return string(data), nil
}

// WriteActiveProfile persists name as the active profile in rootFile.
//
// The name is first checked with ValidateProfileName so invalid identifiers
// surface the validator's sentinel errors before any filesystem work. The
// payload is written to rootFile+".tmp" first and then renamed onto rootFile,
// so a crash mid-write can never leave a half-written file at the canonical
// path. A pre-existing temp file from a prior failed attempt is overwritten.
func WriteActiveProfile(rootFile, name string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	tmp := rootFile + ".tmp"
	if err := os.WriteFile(tmp, []byte(name), 0o600); err != nil {
		return fmt.Errorf("write active profile temp: %w", err)
	}
	if err := os.Rename(tmp, rootFile); err != nil {
		// Best-effort cleanup; keep the rename error as the primary signal.
		_ = os.Remove(tmp)
		return fmt.Errorf("rename active profile: %w", err)
	}
	return nil
}

// ClearActiveProfile removes rootFile so future reads return
// ErrActiveProfileUnset. The operation is idempotent: a missing file is not
// an error.
func ClearActiveProfile(rootFile string) error {
	if err := os.Remove(rootFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("clear active profile: %w", err)
	}
	return nil
}
