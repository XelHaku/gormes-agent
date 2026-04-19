package cli

import (
	"encoding/json"
	"os"
)

type CLISurface struct {
	Commands []string `json:"commands"`
}

type DocsSurface struct {
	Paths []string `json:"paths"`
}

func LoadCLISurface(path string) (CLISurface, error) {
	var out CLISurface
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(b, &out)
	return out, err
}

func LoadDocsSurface(path string) (DocsSurface, error) {
	var out DocsSurface
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(b, &out)
	return out, err
}
