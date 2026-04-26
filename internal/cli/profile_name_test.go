package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateProfileName_AcceptsValid(t *testing.T) {
	cases := []string{
		"default",
		"coder",
		"work-1",
		"tier_2",
		"a",
		strings.Repeat("a", 64),
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateProfileName(name); err != nil {
				t.Fatalf("ValidateProfileName(%q) = %v, want nil", name, err)
			}
		})
	}
}

func TestValidateProfileName_RejectsEmpty(t *testing.T) {
	cases := []string{"", "   "}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateProfileName(name)
			if !errors.Is(err, ErrProfileNameEmpty) {
				t.Fatalf("ValidateProfileName(%q) = %v, want ErrProfileNameEmpty", name, err)
			}
		})
	}
}

func TestValidateProfileName_RejectsTooLong(t *testing.T) {
	name := strings.Repeat("a", 65)
	err := ValidateProfileName(name)
	if !errors.Is(err, ErrProfileNameTooLong) {
		t.Fatalf("ValidateProfileName(65-byte name) = %v, want ErrProfileNameTooLong", err)
	}
}

func TestValidateProfileName_RejectsInvalidChars(t *testing.T) {
	cases := []string{
		"Coder",
		"my profile",
		"-leading",
		"_leading",
		"dot.name",
		"slash/name",
		"tab\tname",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateProfileName(name)
			if !errors.Is(err, ErrProfileNameInvalidChars) {
				t.Fatalf("ValidateProfileName(%q) = %v, want ErrProfileNameInvalidChars", name, err)
			}
		})
	}
}

func TestValidateProfileName_RejectsReserved(t *testing.T) {
	cases := []string{
		"create",
		"delete",
		"list",
		"use",
		"export",
		"import",
		"show",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateProfileName(name)
			if !errors.Is(err, ErrProfileNameReserved) {
				t.Fatalf("ValidateProfileName(%q) = %v, want ErrProfileNameReserved", name, err)
			}
		})
	}
}
