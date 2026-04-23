package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

const (
	browserCDPURLEnv           = "BROWSER_CDP_URL"
	browserDriverEnv           = "BROWSER_DRIVER"
	browserDriverChromedp      = "chromedp"
	browserDriverRod           = "rod"
	defaultBrowserWaitSelector = "body"
	defaultBrowserHTMLSelector = "html"
)

// BrowserSessionConfig controls how a browser session is opened.
type BrowserSessionConfig struct {
	CDPURL string
	Driver string
}

// BrowserSessionFactory opens browser sessions for tools.
type BrowserSessionFactory interface {
	Open(ctx context.Context, cfg BrowserSessionConfig) (BrowserSession, error)
}

// BrowserSession is the minimum browser contract the browser tool needs.
type BrowserSession interface {
	Mode() string
	Navigate(url string) error
	WaitVisible(selector string) error
	Title() (string, error)
	Location() (string, error)
	OuterHTML(selector string) (string, error)
	Close() error
}

// RegisterBrowserTools adds the browser toolset with driver-selectable backends.
func RegisterBrowserTools(reg *Registry) {
	if reg == nil {
		panic("tools: nil registry")
	}
	reg.MustRegisterEntry(ToolEntry{
		Tool:    &BrowserNavigateTool{Factory: NewDefaultBrowserSessionFactory()},
		Toolset: "browser",
		CheckFn: browserToolAvailable,
	})
}

// BrowserNavigateTool opens a page and returns page metadata plus raw HTML.
type BrowserNavigateTool struct {
	Factory BrowserSessionFactory
}

func (*BrowserNavigateTool) Name() string { return "browser_navigate" }
func (*BrowserNavigateTool) Description() string {
	return "Navigate to a URL with a local Chrome/Chromium session or an existing CDP browser using Chromedp or Rod, then return page metadata plus raw HTML."
}
func (*BrowserNavigateTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"absolute URL to open"},"driver":{"type":"string","description":"optional browser driver: chromedp or rod; defaults to $BROWSER_DRIVER or chromedp"},"cdp_url":{"type":"string","description":"optional Chrome DevTools websocket URL; defaults to $BROWSER_CDP_URL when set"},"wait_visible":{"type":"string","description":"CSS selector to wait for before capture; defaults to body"},"outer_html_selector":{"type":"string","description":"CSS selector to capture via OuterHTML; defaults to html"}},"required":["url"]}`)
}
func (*BrowserNavigateTool) Timeout() time.Duration { return 45 * time.Second }

func (t *BrowserNavigateTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		URL               string `json:"url"`
		Driver            string `json:"driver"`
		CDPURL            string `json:"cdp_url"`
		WaitVisible       string `json:"wait_visible"`
		OuterHTMLSelector string `json:"outer_html_selector"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("browser_navigate: invalid args: %w", err)
	}

	url := strings.TrimSpace(in.URL)
	if url == "" {
		return nil, fmt.Errorf("browser_navigate: url is required")
	}

	driver, err := normalizeBrowserDriver(firstNonEmpty(strings.TrimSpace(in.Driver), strings.TrimSpace(os.Getenv(browserDriverEnv))))
	if err != nil {
		return nil, fmt.Errorf("browser_navigate: %w", err)
	}

	cfg := BrowserSessionConfig{
		CDPURL: firstNonEmpty(strings.TrimSpace(in.CDPURL), strings.TrimSpace(os.Getenv(browserCDPURLEnv))),
		Driver: driver,
	}
	factory := t.Factory
	if factory == nil {
		factory = NewDefaultBrowserSessionFactory()
	}

	session, err := factory.Open(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("browser_navigate: open session: %w", err)
	}
	defer func() { _ = session.Close() }()

	if err := session.Navigate(url); err != nil {
		return nil, fmt.Errorf("browser_navigate: navigate: %w", err)
	}
	waitSelector := firstNonEmpty(strings.TrimSpace(in.WaitVisible), defaultBrowserWaitSelector)
	if err := session.WaitVisible(waitSelector); err != nil {
		return nil, fmt.Errorf("browser_navigate: wait_visible(%s): %w", waitSelector, err)
	}
	title, err := session.Title()
	if err != nil {
		return nil, fmt.Errorf("browser_navigate: title: %w", err)
	}
	location, err := session.Location()
	if err != nil {
		return nil, fmt.Errorf("browser_navigate: location: %w", err)
	}
	htmlSelector := firstNonEmpty(strings.TrimSpace(in.OuterHTMLSelector), defaultBrowserHTMLSelector)
	html, err := session.OuterHTML(htmlSelector)
	if err != nil {
		return nil, fmt.Errorf("browser_navigate: outer_html(%s): %w", htmlSelector, err)
	}
	if location == "" {
		location = url
	}

	out := struct {
		URL           string `json:"url"`
		Title         string `json:"title"`
		HTML          string `json:"html"`
		BrowserMode   string `json:"browser_mode"`
		BrowserDriver string `json:"browser_driver"`
	}{
		URL:           location,
		Title:         title,
		HTML:          html,
		BrowserMode:   session.Mode(),
		BrowserDriver: driver,
	}
	return json.Marshal(out)
}

// ChromedpSessionFactory opens browser sessions backed by chromedp.
type ChromedpSessionFactory struct {
	newLocalAllocator  func(context.Context) (context.Context, context.CancelFunc)
	newRemoteAllocator func(context.Context, string) (context.Context, context.CancelFunc)
	newTabContext      func(context.Context) (context.Context, context.CancelFunc)
	newSession         func(context.Context, string, func()) BrowserSession
}

// NewChromedpSessionFactory returns the default Chromedp-backed factory.
func NewChromedpSessionFactory() *ChromedpSessionFactory {
	return &ChromedpSessionFactory{
		newLocalAllocator: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return chromedp.NewExecAllocator(ctx, append([]chromedp.ExecAllocatorOption(nil), chromedp.DefaultExecAllocatorOptions[:]...)...)
		},
		newRemoteAllocator: func(ctx context.Context, url string) (context.Context, context.CancelFunc) {
			return chromedp.NewRemoteAllocator(ctx, url)
		},
		newTabContext: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return chromedp.NewContext(ctx)
		},
		newSession: func(ctx context.Context, mode string, closeFn func()) BrowserSession {
			return &chromedpSession{ctx: ctx, mode: mode, closeFn: closeFn}
		},
	}
}

// Open creates a browser session against either a remote CDP endpoint or a
// locally launched Chrome/Chromium process.
func (f *ChromedpSessionFactory) Open(ctx context.Context, cfg BrowserSessionConfig) (BrowserSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	factory := f.withDefaults()

	mode := "local"
	allocatorCtx := ctx
	allocatorCancel := func() {}
	if cdpURL := strings.TrimSpace(cfg.CDPURL); cdpURL != "" {
		mode = "remote"
		allocatorCtx, allocatorCancel = factory.newRemoteAllocator(ctx, cdpURL)
	} else {
		allocatorCtx, allocatorCancel = factory.newLocalAllocator(ctx)
	}

	tabCtx, tabCancel := factory.newTabContext(allocatorCtx)
	closeFn := func() {
		tabCancel()
		allocatorCancel()
	}
	return factory.newSession(tabCtx, mode, closeFn), nil
}

func (f *ChromedpSessionFactory) withDefaults() *ChromedpSessionFactory {
	defaults := NewChromedpSessionFactory()
	if f == nil {
		return defaults
	}
	out := *f
	if out.newLocalAllocator == nil {
		out.newLocalAllocator = defaults.newLocalAllocator
	}
	if out.newRemoteAllocator == nil {
		out.newRemoteAllocator = defaults.newRemoteAllocator
	}
	if out.newTabContext == nil {
		out.newTabContext = defaults.newTabContext
	}
	if out.newSession == nil {
		out.newSession = defaults.newSession
	}
	return &out
}

type chromedpSession struct {
	ctx      context.Context
	mode     string
	closeFn  func()
	closeOne sync.Once
}

func (s *chromedpSession) Mode() string { return s.mode }

func (s *chromedpSession) Navigate(url string) error {
	return chromedp.Run(s.ctx, chromedp.Navigate(url))
}

func (s *chromedpSession) WaitVisible(selector string) error {
	return chromedp.Run(s.ctx, chromedp.WaitVisible(selector, chromedp.ByQuery))
}

func (s *chromedpSession) Title() (string, error) {
	var title string
	err := chromedp.Run(s.ctx, chromedp.Title(&title))
	return title, err
}

func (s *chromedpSession) Location() (string, error) {
	var location string
	err := chromedp.Run(s.ctx, chromedp.Location(&location))
	return location, err
}

func (s *chromedpSession) OuterHTML(selector string) (string, error) {
	var html string
	err := chromedp.Run(s.ctx, chromedp.OuterHTML(selector, &html, chromedp.ByQuery))
	return html, err
}

func (s *chromedpSession) Close() error {
	s.closeOne.Do(func() {
		if s.closeFn != nil {
			s.closeFn()
		}
	})
	return nil
}

var browserLookupPath = exec.LookPath

func browserToolAvailable() bool {
	if strings.TrimSpace(os.Getenv(browserCDPURLEnv)) != "" {
		return true
	}
	for _, name := range []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
		"chrome",
	} {
		if _, err := browserLookupPath(name); err == nil {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeBrowserDriver(value string) (string, error) {
	driver := strings.ToLower(strings.TrimSpace(value))
	if driver == "" {
		return browserDriverChromedp, nil
	}
	switch driver {
	case browserDriverChromedp, browserDriverRod:
		return driver, nil
	default:
		return "", fmt.Errorf("unsupported driver %q", value)
	}
}
