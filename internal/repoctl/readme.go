package repoctl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

type ReadmeOptions struct {
	Root string
}

func UpdateReadme(opts ReadmeOptions) error {
	if opts.Root == "" {
		return fmt.Errorf("repo root is required")
	}
	benchPath := filepath.Join(opts.Root, "benchmarks.json")
	raw, err := os.ReadFile(benchPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "update-readme: benchmarks.json not found; skipping")
			return nil
		}
		return err
	}
	var data struct {
		Binary struct {
			SizeMB json.RawMessage `json:"size_mb"`
		} `json:"binary"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	sizeMB, err := benchmarkSizeMB(data.Binary.SizeMB)
	if err != nil {
		return err
	}
	if sizeMB == "" {
		return fmt.Errorf("benchmarks.json missing binary.size_mb")
	}
	readmePath := filepath.Join(opts.Root, "README.md")
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`~[0-9.]+ MB`)
	updated := re.ReplaceAllString(string(readme), "~"+sizeMB+" MB")
	return os.WriteFile(readmePath, []byte(updated), 0o644)
}

func benchmarkSizeMB(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, nil
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return strconv.FormatFloat(number, 'f', -1, 64), nil
	}
	return "", fmt.Errorf("benchmarks.json binary.size_mb has unsupported type")
}
