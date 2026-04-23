package tools

import (
	"context"
	"errors"
	"testing"
)

func TestDefaultBrowserSessionFactoryUsesRodFactory(t *testing.T) {
	chromedpFactory := &fakeBrowserSessionFactory{session: &fakeBrowserSession{mode: "local"}}
	rodFactory := &fakeBrowserSessionFactory{session: &fakeBrowserSession{mode: "remote"}}
	factory := &DefaultBrowserSessionFactory{
		chromedp: chromedpFactory,
		rod:      rodFactory,
	}

	session, err := factory.Open(context.Background(), BrowserSessionConfig{
		Driver: browserDriverRod,
		CDPURL: "ws://127.0.0.1:9222/devtools/browser/rod",
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if session.Mode() != "remote" {
		t.Fatalf("session.Mode() = %q, want remote", session.Mode())
	}
	if len(rodFactory.cfgs) != 1 {
		t.Fatalf("rod Open calls = %d, want 1", len(rodFactory.cfgs))
	}
	if len(chromedpFactory.cfgs) != 0 {
		t.Fatalf("chromedp Open calls = %d, want 0", len(chromedpFactory.cfgs))
	}
}

func TestRodSessionFactoryOpenUsesRemoteControlURL(t *testing.T) {
	var connectURL string
	var connectMode string

	factory := &RodSessionFactory{
		launchLocalControlURL: func(context.Context) (string, func(), error) {
			t.Fatal("launchLocalControlURL called for remote config")
			return "", nil, nil
		},
		connect: func(_ context.Context, controlURL string, mode string) (BrowserSession, error) {
			connectURL = controlURL
			connectMode = mode
			return &fakeBrowserSession{mode: mode}, nil
		},
	}

	session, err := factory.Open(context.Background(), BrowserSessionConfig{
		CDPURL: "ws://127.0.0.1:9222/devtools/browser/rod-remote",
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if connectURL != "ws://127.0.0.1:9222/devtools/browser/rod-remote" {
		t.Fatalf("connect URL = %q, want remote control URL", connectURL)
	}
	if connectMode != "remote" {
		t.Fatalf("connect mode = %q, want remote", connectMode)
	}
	if session.Mode() != "remote" {
		t.Fatalf("session.Mode() = %q, want remote", session.Mode())
	}
}

func TestRodSessionFactoryOpenLaunchesLocalBrowserAndClosesLauncher(t *testing.T) {
	session := &fakeBrowserSession{mode: "local"}
	cleanupCount := 0
	connectCount := 0

	factory := &RodSessionFactory{
		launchLocalControlURL: func(context.Context) (string, func(), error) {
			return "ws://127.0.0.1:9222/devtools/browser/rod-local", func() {
				cleanupCount++
			}, nil
		},
		connect: func(_ context.Context, controlURL string, mode string) (BrowserSession, error) {
			connectCount++
			if controlURL != "ws://127.0.0.1:9222/devtools/browser/rod-local" {
				t.Fatalf("connect URL = %q, want launched control URL", controlURL)
			}
			if mode != "local" {
				t.Fatalf("connect mode = %q, want local", mode)
			}
			return session, nil
		},
	}

	opened, err := factory.Open(context.Background(), BrowserSessionConfig{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if connectCount != 1 {
		t.Fatalf("connect calls = %d, want 1", connectCount)
	}
	if cleanupCount != 0 {
		t.Fatalf("launcher cleanup count before Close = %d, want 0", cleanupCount)
	}

	if err := opened.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if session.closeCount != 1 {
		t.Fatalf("session close count = %d, want 1", session.closeCount)
	}
	if cleanupCount != 1 {
		t.Fatalf("launcher cleanup count after Close = %d, want 1", cleanupCount)
	}

	if err := opened.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if session.closeCount != 1 {
		t.Fatalf("session close count after second Close = %d, want 1", session.closeCount)
	}
	if cleanupCount != 1 {
		t.Fatalf("launcher cleanup count after second Close = %d, want 1", cleanupCount)
	}
}

func TestRodSessionFactoryOpenClosesLauncherOnConnectError(t *testing.T) {
	cleanupCount := 0
	wantErr := errors.New("connect failed")

	factory := &RodSessionFactory{
		launchLocalControlURL: func(context.Context) (string, func(), error) {
			return "ws://127.0.0.1:9222/devtools/browser/rod-local", func() {
				cleanupCount++
			}, nil
		},
		connect: func(context.Context, string, string) (BrowserSession, error) {
			return nil, wantErr
		},
	}

	_, err := factory.Open(context.Background(), BrowserSessionConfig{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Open() error = %v, want %v", err, wantErr)
	}
	if cleanupCount != 1 {
		t.Fatalf("launcher cleanup count = %d, want 1", cleanupCount)
	}
}
