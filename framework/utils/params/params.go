package params

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mapper applies transformations to a parsed params map.
// It returns true if any value was changed.
type Mapper func(params map[string]string) bool

// FromEnv resolves a parameter-to-env-var mapping into concrete values
// by looking up each env var. Entries whose env var is unset are omitted.
func FromEnv(mapping map[string]string) map[string]string {
	result := make(map[string]string, len(mapping))

	for param, envVar := range mapping {
		if val := os.Getenv(envVar); val != "" {
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

// Apply applies the provided Mappers to a params.env file at path/file.
// If the file does not exist, Apply is a no-op.
// The file is only rewritten when at least one Mapper reports a change.
func Apply(
	path string,
	file string,
	mappers ...Mapper,
) error {
	paramsFile := filepath.Join(path, file)

	paramsEnvMap, err := parseParams(paramsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	changed := false

	for _, m := range mappers {
		if m(paramsEnvMap) {
			changed = true
		}
	}

	if !changed {
		return nil
	}

	tmp, err := writeParamsToTmp(paramsEnvMap, path)
	if err != nil {
		return err
	}

	if err = os.Rename(tmp, paramsFile); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed rename %s to %s: %w", tmp, paramsFile, err)
	}

	return nil
}

// parseParams reads a key=value file. Lines without '=' are silently skipped.
func parseParams(fileName string) (map[string]string, error) {
	f, err := os.Open(fileName) //nolint:gosec // fileName is a trusted internal path
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if found {
			result[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func writeParamsToTmp(params map[string]string, tmpDir string) (string, error) {
	tmp, err := os.CreateTemp(tmpDir, "params.env-")
	if err != nil {
		return "", err
	}
	defer func() { _ = tmp.Close() }()

	writer := bufio.NewWriter(tmp)
	for key, value := range params {
		if _, err := fmt.Fprintf(writer, "%s=%s\n", key, value); err != nil {
			_ = os.Remove(tmp.Name())
			return "", err
		}
	}

	if err := writer.Flush(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to write to file: %w", err)
	}

	return tmp.Name(), nil
}
