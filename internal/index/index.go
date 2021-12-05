package index

import (
	"encoding/json"
	"fmt"
	"os"
)

// Index maps package path roots to their domains
type Index map[string]Domain

func Load(path string) (Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load index at path %q: %w", path, err)
	}
	defer f.Close()

	var i Index
	if err := json.NewDecoder(f).Decode(&i); err != nil {
		return nil, fmt.Errorf("failed to parse index file at %q: %w", path, err)
	}
	return i, nil
}
