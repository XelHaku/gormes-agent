package autoloop

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func DigestLedger(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	counts := map[string]int{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event LedgerEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return "", err
		}
		counts[event.Event]++
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"runs: %d\nclaimed: %d\nsuccess: %d\npromoted: %d\n",
		counts["run_started"],
		counts["worker_claimed"],
		counts["worker_success"],
		counts["worker_promoted"],
	), nil
}
