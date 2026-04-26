package cli

import (
	"errors"
	"regexp"
	"strings"
)

// Sentinel errors returned by ValidateProfileName so callers can render uniform
// error messages without parsing free-form strings.
var (
	ErrProfileNameEmpty        = errors.New("profile name is empty")
	ErrProfileNameTooLong      = errors.New("profile name exceeds 64 characters")
	ErrProfileNameInvalidChars = errors.New("profile name must match [a-z0-9][a-z0-9_-]{0,63}")
	ErrProfileNameReserved     = errors.New("profile name collides with a reserved subcommand")
)

var profileNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

var reservedProfileNames = map[string]struct{}{
	"create": {},
	"delete": {},
	"list":   {},
	"use":    {},
	"export": {},
	"import": {},
	"show":   {},
}

// ValidateProfileName reports whether name is a valid profile identifier.
//
// The reserved alias "default" is always accepted. Other names must match
// [a-z0-9][a-z0-9_-]{0,63} and must not collide with the CLI subcommand names
// listed in reservedProfileNames.
func ValidateProfileName(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrProfileNameEmpty
	}
	if len(name) > 64 {
		return ErrProfileNameTooLong
	}
	if !profileNameRE.MatchString(name) {
		return ErrProfileNameInvalidChars
	}
	if _, reserved := reservedProfileNames[name]; reserved {
		return ErrProfileNameReserved
	}
	return nil
}
