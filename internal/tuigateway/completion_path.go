package tuigateway

import "strings"

// HomeFn returns the operator's home directory used by NormalizeCompletionPath
// to expand a leading "~" segment. The default returns an empty string so the
// helper stays inert when the caller has not wired a home source. Production
// callers replace it with a closure over their own home-dir lookup (typically
// os.UserHomeDir, performed once at startup); tests stub it per case to keep
// the helper deterministic without touching the real filesystem.
//
// The package-level hook keeps NormalizeCompletionPath's signature aligned
// with the upstream tui_gateway/server.py:_normalize_completion_path contract
// (path + isWindows) while still satisfying the "no os.UserHomeDir inside the
// helper" invariant.
var HomeFn = func() string { return "" }

// NormalizeCompletionPath rewrites a completion-menu path the same way
// hermes-agent/tui_gateway/server.py:_normalize_completion_path (line 428)
// does upstream:
//
//  1. A leading "~" is expanded to the operator's home directory (sourced
//     from the injected HomeFn hook). Only "~" alone or "~/..." is
//     expanded — "~user/..." forms fall through unchanged because the
//     helper has no user-database access.
//  2. When the caller is *not* on Windows (isWindows == false), backslashes
//     in the expanded path are normalised to forward slashes for the
//     drive-letter check; if the result begins with "<letter>:/" the path
//     is rewritten to "/mnt/<lower-letter>/<rest>" so completion menus on
//     Linux/macOS resolve through the WSL mount point. When the pattern
//     does not match, the *expanded* path (still containing any original
//     backslashes) is returned, mirroring upstream's `return expanded`.
//  3. When the caller is on Windows (isWindows == true), the WSL rewrite
//     is skipped entirely — only the tilde-expansion step applies.
//
// The helper performs no filesystem I/O, never consults runtime.GOOS, and
// never reads HOME directly; all platform/home decisions are threaded
// through the parameters and the package-level HomeFn hook.
func NormalizeCompletionPath(in string, isWindows bool) string {
	if in == "" {
		return ""
	}

	expanded := expandLeadingTilde(in)

	if isWindows {
		return expanded
	}

	normalized := strings.ReplaceAll(expanded, `\`, "/")
	if len(normalized) >= 3 &&
		normalized[1] == ':' &&
		normalized[2] == '/' &&
		isASCIILetter(normalized[0]) {
		drive := strings.ToLower(string(normalized[0]))
		return "/mnt/" + drive + "/" + normalized[3:]
	}
	return expanded
}

// expandLeadingTilde mirrors the subset of os.path.expanduser semantics the
// upstream helper relies on: only a leading "~" with no user component is
// expanded, and only when followed by end-of-string or a path separator.
func expandLeadingTilde(in string) string {
	if in == "" || in[0] != '~' {
		return in
	}
	if len(in) == 1 {
		return HomeFn()
	}
	if in[1] == '/' || in[1] == '\\' {
		return HomeFn() + in[1:]
	}
	return in
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
