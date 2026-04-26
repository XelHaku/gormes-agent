package cli

import "errors"

// ErrProfileXDGRootRequired is returned when ResolveProfileRoot is called
// without a non-empty XDG config home; the helper refuses to invent a default
// so callers stay in charge of env resolution.
var ErrProfileXDGRootRequired = errors.New("gormes XDG config home is required")

// ResolveProfileRoot maps a profile name and a caller-supplied XDG config home
// to the directory that would hold that profile's gormes state. It is a pure
// string helper: it never reads the environment, never stats, and never
// creates directories.
//
// For name=="default" it returns gormesXDGConfigHome+"/gormes"; for any other
// name accepted by ValidateProfileName it returns
// gormesXDGConfigHome+"/gormes/profiles/"+name. Invalid names propagate the
// same sentinel errors as ValidateProfileName.
func ResolveProfileRoot(name string, gormesXDGConfigHome string) (string, error) {
	if gormesXDGConfigHome == "" {
		return "", ErrProfileXDGRootRequired
	}
	if err := ValidateProfileName(name); err != nil {
		return "", err
	}
	if name == "default" {
		return gormesXDGConfigHome + "/gormes", nil
	}
	return gormesXDGConfigHome + "/gormes/profiles/" + name, nil
}
