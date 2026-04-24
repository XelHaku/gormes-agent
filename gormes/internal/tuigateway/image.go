package tuigateway

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
)

// ImageMeta describes a pre-analyzed image attachment surfaced to the TUI.
// It mirrors the upstream `_image_meta` dict shape in
// `tui_gateway/server.py` (lines 166-178): the Name field is always
// populated, and Width/Height/TokenEstimate are only included when the
// image decoder successfully reads the file header.
type ImageMeta struct {
	Name          string `json:"name"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	TokenEstimate int    `json:"token_estimate,omitempty"`
}

// EstimateImageTokens is a rough UI estimate for image prompt cost,
// matching `_estimate_image_tokens` in `tui_gateway/server.py` (lines
// 155-163). It tiles the image into 512×512 blocks at ~85 tokens/tile as
// a lightweight cross-provider hint. Non-positive dimensions return 0 so
// callers can short-circuit without branching on a sentinel value.
func EstimateImageTokens(width, height int) int {
	if width <= 0 || height <= 0 {
		return 0
	}
	tilesW := (width + 511) / 512
	if tilesW < 1 {
		tilesW = 1
	}
	tilesH := (height + 511) / 512
	if tilesH < 1 {
		tilesH = 1
	}
	return tilesW * tilesH * 85
}

// ReadImageMeta opens the file at path and returns its ImageMeta. A file
// that cannot be opened or decoded as a known image format returns an
// ImageMeta with only the Name field populated — matching the upstream
// `_image_meta` fallback path which swallows PIL errors and keeps the
// filename-only dict. Supported formats are PNG, JPEG, and GIF via the
// anonymous image decoder registrations in this package.
func ReadImageMeta(path string) ImageMeta {
	meta := ImageMeta{Name: filepath.Base(path)}
	f, err := os.Open(path)
	if err != nil {
		return meta
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return meta
	}
	meta.Width = cfg.Width
	meta.Height = cfg.Height
	meta.TokenEstimate = EstimateImageTokens(cfg.Width, cfg.Height)
	return meta
}
