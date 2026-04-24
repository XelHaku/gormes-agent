package tuigateway

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestEstimateImageTokens(t *testing.T) {
	cases := []struct {
		name          string
		width, height int
		want          int
	}{
		{name: "zero width returns 0", width: 0, height: 512, want: 0},
		{name: "zero height returns 0", width: 512, height: 0, want: 0},
		{name: "negative width returns 0", width: -10, height: 100, want: 0},
		{name: "negative height returns 0", width: 100, height: -10, want: 0},
		{name: "one pixel is one tile", width: 1, height: 1, want: 85},
		{name: "512 square is one tile", width: 512, height: 512, want: 85},
		{name: "513 square rounds to 2x2 tiles", width: 513, height: 513, want: 4 * 85},
		{name: "1024 square is 2x2 tiles", width: 1024, height: 1024, want: 4 * 85},
		{name: "1025x2049 is 3x5 tiles", width: 1025, height: 2049, want: 15 * 85},
		{name: "wide short image 2048x256 is 4x1 tiles", width: 2048, height: 256, want: 4 * 85},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EstimateImageTokens(c.width, c.height)
			if got != c.want {
				t.Fatalf("EstimateImageTokens(%d,%d) = %d, want %d", c.width, c.height, got, c.want)
			}
		})
	}
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
}

func writeJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 75}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write jpeg: %v", err)
	}
}

func TestReadImageMeta_PNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shot.png")
	writePNG(t, path, 800, 600)

	got := ReadImageMeta(path)
	if got.Name != "shot.png" {
		t.Fatalf("Name = %q, want %q", got.Name, "shot.png")
	}
	if got.Width != 800 || got.Height != 600 {
		t.Fatalf("dims = %dx%d, want 800x600", got.Width, got.Height)
	}
	wantTokens := EstimateImageTokens(800, 600)
	if got.TokenEstimate != wantTokens {
		t.Fatalf("TokenEstimate = %d, want %d", got.TokenEstimate, wantTokens)
	}
}

func TestReadImageMeta_JPEG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	writeJPEG(t, path, 320, 240)

	got := ReadImageMeta(path)
	if got.Name != "photo.jpg" {
		t.Fatalf("Name = %q, want %q", got.Name, "photo.jpg")
	}
	if got.Width != 320 || got.Height != 240 {
		t.Fatalf("dims = %dx%d, want 320x240", got.Width, got.Height)
	}
	if got.TokenEstimate != EstimateImageTokens(320, 240) {
		t.Fatalf("TokenEstimate = %d, want %d", got.TokenEstimate, EstimateImageTokens(320, 240))
	}
}

func TestReadImageMeta_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.png")

	got := ReadImageMeta(path)
	if got.Name != "nope.png" {
		t.Fatalf("Name = %q, want %q", got.Name, "nope.png")
	}
	if got.Width != 0 || got.Height != 0 || got.TokenEstimate != 0 {
		t.Fatalf("missing file should leave dims/tokens zero, got %+v", got)
	}
}

func TestReadImageMeta_NonImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("this is not an image"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := ReadImageMeta(path)
	if got.Name != "notes.txt" {
		t.Fatalf("Name = %q, want %q", got.Name, "notes.txt")
	}
	if got.Width != 0 || got.Height != 0 || got.TokenEstimate != 0 {
		t.Fatalf("non-image should leave dims/tokens zero, got %+v", got)
	}
}

func TestImageMeta_JSONOmitsEmpty(t *testing.T) {
	// When decode fails we only know the filename, matching the upstream
	// `_image_meta` dict which only carries the `name` key in that case.
	b, err := json.Marshal(ImageMeta{Name: "only-name.bin"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"name":"only-name.bin"}`
	if string(b) != want {
		t.Fatalf("json = %s, want %s", string(b), want)
	}

	// When decode succeeds the full dict shape is emitted.
	b2, err := json.Marshal(ImageMeta{Name: "full.png", Width: 10, Height: 20, TokenEstimate: 85})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want2 := `{"name":"full.png","width":10,"height":20,"token_estimate":85}`
	if string(b2) != want2 {
		t.Fatalf("json = %s, want %s", string(b2), want2)
	}
}
