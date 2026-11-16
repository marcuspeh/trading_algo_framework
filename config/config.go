// Package config provides a file-backed implementation of the framework's
// job.Config interface.
//
// The file format is a flat "key = value" list, one per line. Blank
// lines and lines starting with '#' are ignored. Values may be quoted to
// preserve surrounding whitespace.
//
//	# strategy.conf
//	strategy = "momentum"
//	max_positions = 5
//	starting_cash = 10000
package config

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/marcuspeh/trading-framework/job"
)

// File is a job.Config backed by a key/value file. It is safe for
// concurrent use.
//
// The file is read once at construction. Use Reload to pick up
// changes.
type File struct {
	mu     sync.RWMutex
	values map[string]string
}

// NewFile reads the given file and returns a Config backed by it.
func NewFile(path string) (*File, error) {
	values, err := readFile(path)
	if err != nil {
		return nil, err
	}
	return &File{values: values}, nil
}

// NewMap returns a Config backed by an in-memory map. It is handy in
// tests and for strategies that build their configuration in code.
func NewMap(values map[string]string) *File {
	cp := make(map[string]string, len(values))
	for k, v := range values {
		cp[k] = v
	}
	return &File{values: cp}
}

// Get returns the value for key. It returns job.ErrNotFound when the
// key is absent.
func (f *File) Get(key string) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	v, ok := f.values[key]
	if !ok {
		return "", job.ErrNotFound
	}
	return v, nil
}

// Reload re-reads the backing file, replacing the current values.
func (f *File) Reload(path string) error {
	values, err := readFile(path)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values = values
	return nil
}

// Keys returns the configured keys, sorted.
func (f *File) Keys() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]string, 0, len(f.values))
	for k := range f.values {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// GetInt returns the value for key parsed as an integer.
func (f *File) GetInt(key string) (int, error) {
	raw, err := f.Get(key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("config: %q is not an integer: %w", key, err)
	}
	return n, nil
}

// GetFloat returns the value for key parsed as a float.
func (f *File) GetFloat(key string) (float64, error) {
	raw, err := f.Get(key)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("config: %q is not a number: %w", key, err)
	}
	return v, nil
}

// GetBool returns the value for key parsed as a boolean.
func (f *File) GetBool(key string) (bool, error) {
	raw, err := f.Get(key)
	if err != nil {
		return false, err
	}
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("config: %q is not a boolean: %w", key, err)
	}
	return v, nil
}

// readFile parses a key/value file.
func readFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	defer f.Close()

	values := map[string]string{}
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		key, value, found := strings.Cut(raw, "=")
		if !found {
			return nil, fmt.Errorf("config: %s:%d: expected key = value", path, line)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("config: %s:%d: empty key", path, line)
		}
		values[key] = unquote(strings.TrimSpace(value))
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return values, nil
}

// unquote strips matching surrounding quotes from a value.
func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}
