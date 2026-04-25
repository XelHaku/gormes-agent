package docs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHugoBuild_HeadMetaAndFavicons builds the docs site with Hugo and
// asserts that the favicon link tags and Open Graph / Twitter card meta
// tags are present on the home page. Guards against regressions in
// docs/layouts/_default/baseof.html that would silently break browser
// tab icons or social previews.
func TestHugoBuild_HeadMetaAndFavicons(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("hugo", "--minify", "-d", tmp)
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hugo build failed: %v\noutput:\n%s", err, string(out))
	}

	home, err := os.ReadFile(filepath.Join(tmp, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	body := string(home)

	// Hugo --minify strips attribute quotes for simple values, so the
	// favicon hrefs render unquoted (e.g. href=/favicon.ico). Open Graph
	// content values keep their quotes because they contain spaces.
	wants := []string{
		`href=/favicon.ico`,
		`href=/favicon-16x16.png`,
		`href=/favicon-32x32.png`,
		`href=/apple-touch-icon.png`,
		// Open Graph + Twitter cards. Permalink + image must be absolute
		// because social crawlers won't resolve relative URLs.
		`property="og:type"`,
		`property="og:site_name" content="Gormes Docs"`,
		`property="og:url" content="https://docs.gormes.ai/"`,
		`property="og:image" content="https://docs.gormes.ai/social-card.png"`,
		`property="og:image:width" content="1200"`,
		`property="og:image:height" content="630"`,
		`name=twitter:card content="summary_large_image"`,
		`name=twitter:image content="https://docs.gormes.ai/social-card.png"`,
	}
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Fatalf("docs index.html missing %q", want)
		}
	}

	// The actual asset files must be staged under the public dir so the
	// link tags resolve once the site is served.
	for _, asset := range []string{
		"favicon.ico",
		"favicon-16x16.png",
		"favicon-32x32.png",
		"apple-touch-icon.png",
		"android-chrome-192x192.png",
		"android-chrome-512x512.png",
		"social-card.png",
	} {
		path := filepath.Join(tmp, asset)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("docs static export missing %s: %v", asset, err)
		}
		if info.Size() == 0 {
			t.Fatalf("docs static export %s is empty", asset)
		}
	}
}
