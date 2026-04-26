package tuigateway

import "testing"

// withInjectedHome swaps the package-level HomeFn for the duration of a
// test so the helper resolves a deterministic home directory without
// touching the real filesystem. The previous value is restored on cleanup.
func withInjectedHome(t *testing.T, home string) {
	t.Helper()
	prev := HomeFn
	t.Cleanup(func() { HomeFn = prev })
	HomeFn = func() string { return home }
}

// TestNormalizeCompletionPath_TildeExpansion mirrors upstream
// hermes-agent/tui_gateway/server.py:_normalize_completion_path's
// os.path.expanduser pass: a leading "~/" gets rewritten to the
// caller-injected home directory. The helper itself never reads HOME.
func TestNormalizeCompletionPath_TildeExpansion(t *testing.T) {
	withInjectedHome(t, "/home/operator")
	got := NormalizeCompletionPath("~/projects/x", false)
	const want = "/home/operator/projects/x"
	if got != want {
		t.Errorf("NormalizeCompletionPath(%q, false) = %q; want %q", "~/projects/x", got, want)
	}
}

// TestNormalizeCompletionPath_BareTildeExpandsToHome covers the upstream
// `os.path.expanduser("~")` branch: a lone "~" becomes the injected home
// without trailing slashes.
func TestNormalizeCompletionPath_BareTildeExpandsToHome(t *testing.T) {
	withInjectedHome(t, "/home/operator")
	got := NormalizeCompletionPath("~", false)
	const want = "/home/operator"
	if got != want {
		t.Errorf("NormalizeCompletionPath(%q, false) = %q; want %q", "~", got, want)
	}
}

// TestNormalizeCompletionPath_WindowsToMntOnLinux mirrors the WSL
// drive-mount rewrite: on a non-Windows caller, an absolute Windows path
// like "C:/Users/x" becomes "/mnt/c/Users/x" so completion menus on
// Linux/macOS still resolve through the WSL mount point.
func TestNormalizeCompletionPath_WindowsToMntOnLinux(t *testing.T) {
	withInjectedHome(t, "/home/operator")
	got := NormalizeCompletionPath("C:/Users/x", false)
	const want = "/mnt/c/Users/x"
	if got != want {
		t.Errorf("NormalizeCompletionPath(%q, false) = %q; want %q", "C:/Users/x", got, want)
	}
}

// TestNormalizeCompletionPath_BackslashWindowsToMntOnLinux exercises the
// `expanded.replace("\\", "/")` upstream step: a Windows-flavoured path
// with backslashes still maps to /mnt/<drive>/... on a non-Windows caller.
func TestNormalizeCompletionPath_BackslashWindowsToMntOnLinux(t *testing.T) {
	withInjectedHome(t, "/home/operator")
	got := NormalizeCompletionPath(`C:\Users\x`, false)
	const want = "/mnt/c/Users/x"
	if got != want {
		t.Errorf("NormalizeCompletionPath(%q, false) = %q; want %q", `C:\Users\x`, got, want)
	}
}

// TestNormalizeCompletionPath_DriveLetterLowercased confirms the upstream
// `normalized[0].lower()` behaviour: an upper-case drive letter is
// lower-cased in the resulting /mnt/<drive>/... path.
func TestNormalizeCompletionPath_DriveLetterLowercased(t *testing.T) {
	withInjectedHome(t, "/home/operator")
	got := NormalizeCompletionPath("D:/work/notes", false)
	const want = "/mnt/d/work/notes"
	if got != want {
		t.Errorf("NormalizeCompletionPath(%q, false) = %q; want %q", "D:/work/notes", got, want)
	}
}

// TestNormalizeCompletionPath_NoExpansionOnWindows asserts the upstream
// `if os.name != "nt"` guard: when the caller declares it is running on
// Windows, the drive-mount rewrite is skipped and the input is returned
// after tilde expansion only.
func TestNormalizeCompletionPath_NoExpansionOnWindows(t *testing.T) {
	withInjectedHome(t, `C:\Users\operator`)
	got := NormalizeCompletionPath("C:/Users/x", true)
	const want = "C:/Users/x"
	if got != want {
		t.Errorf("NormalizeCompletionPath(%q, true) = %q; want %q", "C:/Users/x", got, want)
	}
}

// TestNormalizeCompletionPath_EmptyInputReturnsEmpty exercises the
// degenerate input path: an empty string round-trips without panic and
// without invoking the home-dir hook.
func TestNormalizeCompletionPath_EmptyInputReturnsEmpty(t *testing.T) {
	t.Cleanup(func() { HomeFn = func() string { return "" } })
	HomeFn = func() string {
		t.Errorf("HomeFn must not be consulted for empty input")
		return ""
	}
	got := NormalizeCompletionPath("", false)
	if got != "" {
		t.Errorf("NormalizeCompletionPath(\"\", false) = %q; want \"\"", got)
	}
}

// TestNormalizeCompletionPath_NoSlashConversionWithoutDriveColon covers
// the upstream "no match" branch: a plain POSIX path that lacks the
// drive-letter pattern is returned untouched on non-Windows callers.
func TestNormalizeCompletionPath_NoSlashConversionWithoutDriveColon(t *testing.T) {
	withInjectedHome(t, "/home/operator")
	got := NormalizeCompletionPath("/usr/bin", false)
	const want = "/usr/bin"
	if got != want {
		t.Errorf("NormalizeCompletionPath(%q, false) = %q; want %q", "/usr/bin", got, want)
	}
}

// TestNormalizeCompletionPath_NonTildeNonDrivePassthrough rounds out the
// passthrough surface: relative paths and bare names without a leading
// tilde or drive prefix are returned verbatim.
func TestNormalizeCompletionPath_NonTildeNonDrivePassthrough(t *testing.T) {
	withInjectedHome(t, "/home/operator")
	cases := []string{
		"projects/x",
		"./relative/path",
		"plainfile",
	}
	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			if got := NormalizeCompletionPath(in, false); got != in {
				t.Errorf("NormalizeCompletionPath(%q, false) = %q; want %q", in, got, in)
			}
		})
	}
}

// TestNormalizeCompletionPath_TildeOnWindowsStillExpands mirrors the
// upstream invariant that os.path.expanduser fires before the os.name
// branch: even on Windows callers, "~/foo" should resolve through the
// injected home (the WSL rewrite is the only Windows-specific guard).
func TestNormalizeCompletionPath_TildeOnWindowsStillExpands(t *testing.T) {
	withInjectedHome(t, `C:\Users\operator`)
	got := NormalizeCompletionPath("~/Documents", true)
	const want = `C:\Users\operator/Documents`
	if got != want {
		t.Errorf("NormalizeCompletionPath(%q, true) = %q; want %q", "~/Documents", got, want)
	}
}

// TestNormalizeCompletionPath_DriveColonWithoutSlashUntouched exercises
// the upstream guard `normalized[2] == "/"`: a "C:foo" string lacks the
// trailing slash, so it falls through the pattern check unchanged.
func TestNormalizeCompletionPath_DriveColonWithoutSlashUntouched(t *testing.T) {
	withInjectedHome(t, "/home/operator")
	got := NormalizeCompletionPath("C:foo", false)
	const want = "C:foo"
	if got != want {
		t.Errorf("NormalizeCompletionPath(%q, false) = %q; want %q", "C:foo", got, want)
	}
}
