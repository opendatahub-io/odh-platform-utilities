package params

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// Mapper applies transformations to a parsed params map.
// It returns true if any value was changed.
type Mapper func(params map[string]string) bool

// FromEnv resolves a parameter-to-env-var mapping into concrete values
// by looking up each env var. Entries whose env var is unset are omitted;
// vars that are set but empty are included as empty strings.
func FromEnv(mapping map[string]string) map[string]string {
	result := make(map[string]string, len(mapping))

	for param, envVar := range mapping {
		if val, ok := os.LookupEnv(envVar); ok {
			result[param] = val
		}
	}

	return result
}

// Replacement returns a Mapper that only updates keys already present
// in the params map. Keys in values that don't exist in the file are skipped.
func Replacement(values map[string]string) Mapper {
	return func(params map[string]string) bool {
		changed := false

		for key := range params {
			val, ok := values[key]
			if !ok || params[key] == val {
				continue
			}

			params[key] = val
			changed = true
		}

		return changed
	}
}

// Values returns a Mapper that merges the provided key-value pairs
// into the params map, adding or updating keys as needed.
func Values(values map[string]string) Mapper {
	return func(params map[string]string) bool {
		changed := false

		for k, v := range values {
			if params[k] != v {
				params[k] = v
				changed = true
			}
		}

		return changed
	}
}

// Apply reads, transforms, and rewrites a params.env file inside a filesys.FileSystem.
// If the file does not exist, Apply is a no-op.
// The file is only rewritten when at least one Mapper reports a change.
// When used with a union filesystem, writes go exclusively to the in-memory overlay,
// leaving the underlying base filesystem (disk or embedded) unmodified.
//
// Write semantics: the output is a clean, sorted key=value file; comments,
// blank lines, and original ordering are not preserved, by design, as Apply
// targets ephemeral runtime injection, not in-place file editing.
func Apply(
	fs filesys.FileSystem,
	file string,
	mappers ...Mapper,
) error {
	if !fs.Exists(file) {
		return nil
	}

	content, err := fs.ReadFile(file)
	if err != nil {
		return fmt.Errorf("reading %s: %w", file, err)
	}

	paramsMap, err := Unmarshal(content)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", file, err)
	}

	changed := false

	for _, m := range mappers {
		if m(paramsMap) {
			changed = true
		}
	}

	if !changed {
		return nil
	}

	data, err := Marshal(paramsMap)
	if err != nil {
		return fmt.Errorf("serializing %s: %w", file, err)
	}

	if err := fs.WriteFile(file, data); err != nil {
		return fmt.Errorf("writing %s: %w", file, err)
	}

	return nil
}

// ApplyAtPath is a convenience wrapper with the same signature as the path-based
// variant. It opens path/file on the OS filesystem and delegates to Apply.
func ApplyAtPath(
	path string,
	file string,
	mappers ...Mapper,
) error {
	return Apply(filesys.MakeFsOnDisk(), filepath.Join(path, file), mappers...)
}

// Unmarshal reads key=value lines from raw bytes.
// Blank lines and comment lines (starting with '#') are skipped; keys are trimmed.
// Inline comments within values are NOT stripped — '#' is treated as a literal
// character so that mapper-injected values containing '#' survive round-trips.
// Lines without '=' (bare keys) are not supported and are silently ignored,
// as the write-back via Marshal would otherwise drop them.
func Unmarshal(content []byte) (map[string]string, error) {
	result := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if found {
			result[strings.TrimSpace(key)] = value
		}
	}

	return result, scanner.Err()
}

// Marshal encodes a params map to sorted key=value lines.
func Marshal(params map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var buf bytes.Buffer
	for _, k := range keys {
		if _, err := fmt.Fprintf(&buf, "%s=%s\n", k, params[k]); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}
