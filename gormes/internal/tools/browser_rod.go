package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// DefaultBrowserSessionFactory routes browser sessions to the configured driver.
type DefaultBrowserSessionFactory struct {
	chromedp BrowserSessionFactory
	rod      BrowserSessionFactory
}

// NewDefaultBrowserSessionFactory returns the production browser driver switch.
func NewDefaultBrowserSessionFactory() *DefaultBrowserSessionFactory {
	return &DefaultBrowserSessionFactory{
		chromedp: NewChromedpSessionFactory(),
		rod:      NewRodSessionFactory(),
	}
}

// Open selects the requested browser driver and opens a session with it.
func (f *DefaultBrowserSessionFactory) Open(ctx context.Context, cfg BrowserSessionConfig) (BrowserSession, error) {
	driver, err := normalizeBrowserDriver(cfg.Driver)
	if err != nil {
		return nil, err
	}
	cfg.Driver = driver

	factory := f.withDefaults()
	switch driver {
	case browserDriverChromedp:
		return factory.chromedp.Open(ctx, cfg)
	case browserDriverRod:
		return factory.rod.Open(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported driver %q", driver)
	}
}

func (f *DefaultBrowserSessionFactory) withDefaults() *DefaultBrowserSessionFactory {
	defaults := NewDefaultBrowserSessionFactory()
	if f == nil {
		return defaults
	}
	out := *f
	if out.chromedp == nil {
		out.chromedp = defaults.chromedp
	}
	if out.rod == nil {
		out.rod = defaults.rod
	}
	return &out
}

// RodSessionFactory opens browser sessions backed by go-rod.
type RodSessionFactory struct {
	launchLocalControlURL func(context.Context) (string, func(), error)
	connect               func(context.Context, string, string) (BrowserSession, error)
}

// NewRodSessionFactory returns the default Rod-backed factory.
func NewRodSessionFactory() *RodSessionFactory {
	return &RodSessionFactory{
		launchLocalControlURL: func(ctx context.Context) (string, func(), error) {
			l := launcher.New().Context(ctx)
			controlURL, err := l.Launch()
			if err != nil {
				return "", nil, err
			}
			return controlURL, func() {
				l.Kill()
			}, nil
		},
		connect: func(_ context.Context, controlURL string, mode string) (BrowserSession, error) {
			browser := rod.New().ControlURL(controlURL)
			if err := browser.Connect(); err != nil {
				return nil, err
			}
			page, err := browser.Page(proto.TargetCreateTarget{})
			if err != nil {
				_ = browser.Close()
				return nil, err
			}
			return &rodSession{
				browser: browser,
				page:    page,
				mode:    mode,
			}, nil
		},
	}
}

// Open creates a browser session against either a remote CDP endpoint or a
// locally launched Chrome/Chromium process via Rod.
func (f *RodSessionFactory) Open(ctx context.Context, cfg BrowserSessionConfig) (BrowserSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	factory := f.withDefaults()

	mode := "remote"
	controlURL := strings.TrimSpace(cfg.CDPURL)
	cleanup := func() {}
	if controlURL == "" {
		mode = "local"
		var err error
		controlURL, cleanup, err = factory.launchLocalControlURL(ctx)
		if err != nil {
			return nil, err
		}
	}

	session, err := factory.connect(ctx, controlURL, mode)
	if err != nil {
		cleanup()
		return nil, err
	}
	return &browserSessionCloser{BrowserSession: session, cleanup: cleanup}, nil
}

func (f *RodSessionFactory) withDefaults() *RodSessionFactory {
	defaults := NewRodSessionFactory()
	if f == nil {
		return defaults
	}
	out := *f
	if out.launchLocalControlURL == nil {
		out.launchLocalControlURL = defaults.launchLocalControlURL
	}
	if out.connect == nil {
		out.connect = defaults.connect
	}
	return &out
}

type browserSessionCloser struct {
	BrowserSession
	cleanup  func()
	closeOne sync.Once
}

func (s *browserSessionCloser) Close() error {
	var err error
	s.closeOne.Do(func() {
		if s.BrowserSession != nil {
			err = s.BrowserSession.Close()
		}
		if s.cleanup != nil {
			s.cleanup()
		}
	})
	return err
}

type rodSession struct {
	browser  *rod.Browser
	page     *rod.Page
	mode     string
	closeOne sync.Once
}

func (s *rodSession) Mode() string { return s.mode }

func (s *rodSession) Navigate(url string) error {
	if err := s.page.Navigate(url); err != nil {
		return err
	}
	return s.page.WaitLoad()
}

func (s *rodSession) WaitVisible(selector string) error {
	el, err := s.page.Element(selector)
	if err != nil {
		return err
	}
	return el.WaitVisible()
}

func (s *rodSession) Title() (string, error) {
	info, err := s.pageInfo()
	if err != nil {
		return "", err
	}
	return info.Title, nil
}

func (s *rodSession) Location() (string, error) {
	info, err := s.pageInfo()
	if err != nil {
		return "", err
	}
	return info.URL, nil
}

func (s *rodSession) OuterHTML(selector string) (string, error) {
	el, err := s.page.Element(selector)
	if err != nil {
		return "", err
	}
	return el.HTML()
}

func (s *rodSession) Close() error {
	var err error
	s.closeOne.Do(func() {
		if s.page != nil {
			if pageErr := s.page.Close(); pageErr != nil {
				err = pageErr
			}
		}
		if s.browser != nil {
			if browserErr := s.browser.Close(); err == nil {
				err = browserErr
			}
		}
	})
	return err
}

func (s *rodSession) pageInfo() (*proto.TargetTargetInfo, error) {
	return s.page.Info()
}
