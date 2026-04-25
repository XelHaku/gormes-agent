package builderloop

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FailureRecord struct {
	Count           int      `json:"count"`
	LastRC          int      `json:"last_rc"`
	LastReason      string   `json:"last_reason"`
	LastStderrTail  string   `json:"last_stderr_tail"`
	LastFinalErrors []string `json:"last_final_errors"`
}

func ReadFailureRecord(root, slug string) (FailureRecord, error) {
	raw, err := os.ReadFile(failureRecordPath(root, slug))
	if err != nil {
		return FailureRecord{}, err
	}

	var record FailureRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return FailureRecord{}, err
	}
	return record, nil
}

func WriteFailureRecord(root, slug string, rc int, reason, stderrPath string, finalErrors []string) error {
	record, err := readFailureRecordForWrite(root, slug)
	if err != nil {
		return err
	}

	tail, err := tailFileLines(stderrPath, 40)
	if err != nil {
		return err
	}

	record.Count++
	record.LastRC = rc
	record.LastReason = reason
	record.LastStderrTail = tail
	record.LastFinalErrors = finalErrors

	path := failureRecordPath(root, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeFileAtomically(path, raw, 0o644)
}

func failureRecordPath(root, slug string) string {
	return filepath.Join(root, "task-failures", slug+".json")
}

func readFailureRecordForWrite(root, slug string) (FailureRecord, error) {
	record, err := ReadFailureRecord(root, slug)
	if err == nil {
		return record, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return FailureRecord{}, nil
	}

	var syntaxError *json.SyntaxError
	var unmarshalTypeError *json.UnmarshalTypeError
	if errors.As(err, &syntaxError) || errors.As(err, &unmarshalTypeError) || errors.Is(err, io.EOF) {
		return FailureRecord{}, nil
	}
	return FailureRecord{}, err
}

func tailFileLines(path string, maxLines int) (string, error) {
	if path == "" {
		return "", nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}

	text := strings.TrimRight(string(raw), "\n")
	if text == "" {
		return "", nil
	}

	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n"), nil
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Chmod(perm); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}
