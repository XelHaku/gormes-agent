package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type fakeBrowserSessionFactory struct {
	cfgs    []BrowserSessionConfig
	session BrowserSession
	err     error
}

func (f *fakeBrowserSessionFactory) Open(_ context.Context, cfg BrowserSessionConfig) (BrowserSession, error) {
	f.cfgs = append(f.cfgs, cfg)
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

type fakeBrowserSession struct {
	mode          string
	title         string
	location      string
	html          string
	navigateURL   string
	waitSelectors []string
	htmlSelectors []string
	closeCount    int
}

func (s *fakeBrowserSession) Mode() string { return s.mode }

func (s *fakeBrowserSession) Navigate(url string) error {
	s.navigateURL = url
	return nil
}

func (s *fakeBrowserSession) WaitVisible(selector string) error {
	s.waitSelectors = append(s.waitSelectors, selector)
	return nil
}

func (s *fakeBrowserSession) Title() (string, error) {
	return s.title, nil
}

func (s *fakeBrowserSession) Location() (string, error) {
	return s.location, nil
}

func (s *fakeBrowserSession) OuterHTML(selector string) (string, error) {
	s.htmlSelectors = append(s.htmlSelectors, selector)
	return s.html, nil
}

func (s *fakeBrowserSession) Close() error {
	s.closeCount++
	return nil
}

func TestBrowserNavigateToolExecuteUsesFactoryAndDefaults(t *testing.T) {
	session := &fakeBrowserSession{
		mode:     "local",
		title:    "Example Domain",
		location: "https://example.com/",
		html:     "<html>example</html>",
	}
	factory := &fakeBrowserSessionFactory{session: session}
	tool := &BrowserNavigateTool{Factory: factory}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(factory.cfgs) != 1 {
		t.Fatalf("factory.Open calls = %d, want 1", len(factory.cfgs))
	}
	if got := factory.cfgs[0].CDPURL; got != "" {
		t.Fatalf("factory cfg cdp_url = %q, want empty", got)
	}
	if got := factory.cfgs[0].Driver; got != browserDriverChromedp {
		t.Fatalf("factory cfg driver = %q, want %q", got, browserDriverChromedp)
	}
	if got := session.navigateURL; got != "https://example.com" {
		t.Fatalf("Navigate url = %q, want https://example.com", got)
	}
	if got := session.waitSelectors; !reflect.DeepEqual(got, []string{"body"}) {
		t.Fatalf("WaitVisible selectors = %v, want [body]", got)
	}
	if got := session.htmlSelectors; !reflect.DeepEqual(got, []string{"html"}) {
		t.Fatalf("OuterHTML selectors = %v, want [html]", got)
	}
	if session.closeCount != 1 {
		t.Fatalf("Close calls = %d, want 1", session.closeCount)
	}

	var got struct {
		URL         string `json:"url"`
		Title       string `json:"title"`
		HTML        string `json:"html"`
		BrowserMode string `json:"browser_mode"`
		Driver      string `json:"browser_driver"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("json.Unmarshal(output): %v", err)
	}
	if got.URL != "https://example.com/" || got.Title != "Example Domain" || got.HTML != "<html>example</html>" || got.BrowserMode != "local" || got.Driver != browserDriverChromedp {
		t.Fatalf("output = %+v, want location/title/html/browser_mode/browser_driver populated", got)
	}
}

func TestBrowserNavigateToolExecuteUsesEnvCDPURLWhenArgMissing(t *testing.T) {
	t.Setenv(browserCDPURLEnv, "ws://127.0.0.1:9222/devtools/browser/test")

	session := &fakeBrowserSession{mode: "remote"}
	factory := &fakeBrowserSessionFactory{session: session}
	tool := &BrowserNavigateTool{Factory: factory}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"https://example.com"}`)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(factory.cfgs) != 1 {
		t.Fatalf("factory.Open calls = %d, want 1", len(factory.cfgs))
	}
	if got := factory.cfgs[0].CDPURL; got != "ws://127.0.0.1:9222/devtools/browser/test" {
		t.Fatalf("factory cfg cdp_url = %q, want env value", got)
	}
	if got := factory.cfgs[0].Driver; got != browserDriverChromedp {
		t.Fatalf("factory cfg driver = %q, want %q", got, browserDriverChromedp)
	}
}

func TestBrowserNavigateToolExecutePrefersExplicitCDPURLAndDriver(t *testing.T) {
	t.Setenv(browserCDPURLEnv, "ws://127.0.0.1:9222/devtools/browser/env")
	t.Setenv(browserDriverEnv, browserDriverChromedp)

	session := &fakeBrowserSession{mode: "remote"}
	factory := &fakeBrowserSessionFactory{session: session}
	tool := &BrowserNavigateTool{Factory: factory}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"https://example.com","cdp_url":"ws://127.0.0.1:9222/devtools/browser/arg","driver":"rod"}`)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := factory.cfgs[0].CDPURL; got != "ws://127.0.0.1:9222/devtools/browser/arg" {
		t.Fatalf("factory cfg cdp_url = %q, want explicit arg", got)
	}
	if got := factory.cfgs[0].Driver; got != browserDriverRod {
		t.Fatalf("factory cfg driver = %q, want explicit arg", got)
	}
}

func TestBrowserNavigateToolExecuteUsesEnvDriverWhenArgMissing(t *testing.T) {
	t.Setenv(browserDriverEnv, browserDriverRod)

	session := &fakeBrowserSession{mode: "remote"}
	factory := &fakeBrowserSessionFactory{session: session}
	tool := &BrowserNavigateTool{Factory: factory}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := factory.cfgs[0].Driver; got != browserDriverRod {
		t.Fatalf("factory cfg driver = %q, want env value", got)
	}

	var got struct {
		Driver string `json:"browser_driver"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("json.Unmarshal(output): %v", err)
	}
	if got.Driver != browserDriverRod {
		t.Fatalf("output browser_driver = %q, want %q", got.Driver, browserDriverRod)
	}
}

func TestBrowserNavigateToolExecuteRejectsMissingURL(t *testing.T) {
	tool := &BrowserNavigateTool{Factory: &fakeBrowserSessionFactory{session: &fakeBrowserSession{}}}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"   "}`)); err == nil {
		t.Fatal("Execute() error = nil, want missing URL error")
	}
}

func TestBrowserNavigateToolExecuteRejectsUnknownDriver(t *testing.T) {
	tool := &BrowserNavigateTool{Factory: &fakeBrowserSessionFactory{session: &fakeBrowserSession{}}}

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"https://example.com","driver":"webkit"}`))
	if err == nil {
		t.Fatal("Execute() error = nil, want unsupported driver error")
	}
	if !strings.Contains(err.Error(), "unsupported driver") {
		t.Fatalf("Execute() error = %v, want unsupported driver detail", err)
	}
}
