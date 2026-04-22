package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadedHook captures one successfully discovered hook directory.
type LoadedHook struct {
	Name        string
	Description string
	Events      []string
	Dir         string
	Command     []string
}

type hookManifest struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Events      []string `yaml:"events"`
	Command     []string `yaml:"command"`
}

type hookPayload struct {
	Point    HookPoint     `json:"point"`
	Platform string        `json:"platform,omitempty"`
	ChatID   string        `json:"chat_id,omitempty"`
	MsgID    string        `json:"msg_id,omitempty"`
	Kind     EventKind     `json:"kind,omitempty"`
	Text     string        `json:"text,omitempty"`
	Inbound  *InboundEvent `json:"inbound,omitempty"`
	Error    string        `json:"error,omitempty"`
}

var allHookPoints = []HookPoint{
	HookBeforeReceive,
	HookAfterReceive,
	HookBeforeSend,
	HookAfterSend,
	HookOnError,
}

// LoadHookScripts discovers hook directories under root, validates their
// HOOK.yaml manifests, and registers executable handlers on a Hooks registry.
//
// Invalid hook directories are skipped with a warning so one bad hook does not
// block the rest of gateway startup.
func LoadHookScripts(root string, log *slog.Logger) (*Hooks, []LoadedHook, error) {
	hooks := NewHooks()
	if log == nil {
		log = slog.Default()
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return hooks, nil, nil
	}

	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return hooks, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read hooks root %q: %w", root, err)
	}

	loaded := make([]LoadedHook, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		manifestPath := filepath.Join(dir, "HOOK.yaml")
		manifestBytes, err := os.ReadFile(manifestPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			log.Warn("skip hook: read manifest failed", "dir", dir, "err", err)
			continue
		}

		var manifest hookManifest
		if err := yaml.Unmarshal(manifestBytes, &manifest); err != nil {
			log.Warn("skip hook: parse manifest failed", "dir", dir, "err", err)
			continue
		}

		spec, points, err := buildLoadedHook(dir, entry.Name(), manifest)
		if err != nil {
			log.Warn("skip hook: invalid manifest", "dir", dir, "err", err)
			continue
		}

		registerLoadedHook(hooks, spec, points, log)
		loaded = append(loaded, spec)
	}

	return hooks, loaded, nil
}

func buildLoadedHook(dir, fallbackName string, manifest hookManifest) (LoadedHook, []HookPoint, error) {
	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		name = fallbackName
	}
	if len(manifest.Command) == 0 {
		return LoadedHook{}, nil, errors.New("command is required")
	}

	command := append([]string(nil), manifest.Command...)
	if !filepath.IsAbs(command[0]) {
		command[0] = filepath.Join(dir, command[0])
	}
	if _, err := os.Stat(command[0]); err != nil {
		return LoadedHook{}, nil, fmt.Errorf("command %q: %w", command[0], err)
	}

	points, err := expandHookEvents(manifest.Events)
	if err != nil {
		return LoadedHook{}, nil, err
	}

	return LoadedHook{
		Name:        name,
		Description: strings.TrimSpace(manifest.Description),
		Events:      append([]string(nil), manifest.Events...),
		Dir:         dir,
		Command:     command,
	}, points, nil
}

func expandHookEvents(events []string) ([]HookPoint, error) {
	if len(events) == 0 {
		return nil, errors.New("events are required")
	}

	seen := make(map[HookPoint]struct{}, len(allHookPoints))
	out := make([]HookPoint, 0, len(allHookPoints))

	for _, raw := range events {
		event := strings.TrimSpace(raw)
		if event == "" {
			return nil, errors.New("event names must be non-empty")
		}
		switch {
		case event == "*":
			for _, point := range allHookPoints {
				if _, ok := seen[point]; ok {
					continue
				}
				seen[point] = struct{}{}
				out = append(out, point)
			}
		case slices.Contains(allHookPoints, HookPoint(event)):
			point := HookPoint(event)
			if _, ok := seen[point]; ok {
				continue
			}
			seen[point] = struct{}{}
			out = append(out, point)
		case strings.HasSuffix(event, "*"):
			prefix := strings.TrimSuffix(event, "*")
			matched := false
			for _, point := range allHookPoints {
				if !strings.HasPrefix(string(point), prefix) {
					continue
				}
				matched = true
				if _, ok := seen[point]; ok {
					continue
				}
				seen[point] = struct{}{}
				out = append(out, point)
			}
			if !matched {
				return nil, fmt.Errorf("event pattern %q matches no hook points", event)
			}
		default:
			return nil, fmt.Errorf("unsupported hook event %q", event)
		}
	}

	return out, nil
}

func registerLoadedHook(hooks *Hooks, spec LoadedHook, points []HookPoint, log *slog.Logger) {
	for _, point := range points {
		point := point
		hooks.Add(point, func(ctx context.Context, ev HookEvent) {
			if err := runHookCommand(ctx, spec, ev); err != nil {
				log.Warn("hook command failed", "hook", spec.Name, "point", point, "err", err)
			}
		})
	}
}

func runHookCommand(ctx context.Context, spec LoadedHook, ev HookEvent) error {
	payload, err := json.Marshal(hookPayload{
		Point:    ev.Point,
		Platform: ev.Platform,
		ChatID:   ev.ChatID,
		MsgID:    ev.MsgID,
		Kind:     ev.Kind,
		Text:     ev.Text,
		Inbound:  ev.Inbound,
		Error:    errorString(ev.Err),
	})
	if err != nil {
		return fmt.Errorf("marshal hook payload: %w", err)
	}

	cmd := exec.CommandContext(ctx, spec.Command[0], spec.Command[1:]...)
	cmd.Dir = spec.Dir
	cmd.Stdin = bytes.NewReader(payload)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) == 0 {
			return err
		}
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
