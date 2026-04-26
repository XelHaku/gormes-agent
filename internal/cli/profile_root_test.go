package cli

import (
	"errors"
	"testing"
)

func TestResolveProfileRoot_DefaultProfile(t *testing.T) {
	got, err := ResolveProfileRoot("default", "/tmp/cfg")
	if err != nil {
		t.Fatalf("ResolveProfileRoot(default, /tmp/cfg) err = %v, want nil", err)
	}
	if want := "/tmp/cfg/gormes"; got != want {
		t.Fatalf("ResolveProfileRoot(default, /tmp/cfg) = %q, want %q", got, want)
	}
}

func TestResolveProfileRoot_NamedProfile(t *testing.T) {
	got, err := ResolveProfileRoot("coder", "/tmp/cfg")
	if err != nil {
		t.Fatalf("ResolveProfileRoot(coder, /tmp/cfg) err = %v, want nil", err)
	}
	if want := "/tmp/cfg/gormes/profiles/coder"; got != want {
		t.Fatalf("ResolveProfileRoot(coder, /tmp/cfg) = %q, want %q", got, want)
	}
}

func TestResolveProfileRoot_RejectsInvalidName(t *testing.T) {
	got, err := ResolveProfileRoot("Coder", "/tmp/cfg")
	if !errors.Is(err, ErrProfileNameInvalidChars) {
		t.Fatalf("ResolveProfileRoot(Coder, /tmp/cfg) err = %v, want ErrProfileNameInvalidChars", err)
	}
	if got != "" {
		t.Fatalf("ResolveProfileRoot(Coder, /tmp/cfg) path = %q, want empty string", got)
	}
}

func TestResolveProfileRoot_RejectsEmptyXDGRoot(t *testing.T) {
	got, err := ResolveProfileRoot("default", "")
	if !errors.Is(err, ErrProfileXDGRootRequired) {
		t.Fatalf("ResolveProfileRoot(default, \"\") err = %v, want ErrProfileXDGRootRequired", err)
	}
	if got != "" {
		t.Fatalf("ResolveProfileRoot(default, \"\") path = %q, want empty string", got)
	}
}

func TestResolveProfileRoot_NoFilesystemAccess(t *testing.T) {
	const nonexistent = "/this/path/definitely/does/not/exist/anywhere"
	got, err := ResolveProfileRoot("coder", nonexistent)
	if err != nil {
		t.Fatalf("ResolveProfileRoot under nonexistent root err = %v, want nil", err)
	}
	if want := nonexistent + "/gormes/profiles/coder"; got != want {
		t.Fatalf("ResolveProfileRoot under nonexistent root = %q, want %q", got, want)
	}
}
