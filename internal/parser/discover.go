package parser

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	"target":       true,
	"build":        true,
	"dist":         true,
}

// DiscoverSecrets walks root recursively, parses all credential files,
// and returns a merged map of key -> secret value.
// Values that are too short or look like non-secrets are filtered out.
func DiscoverSecrets(root string) (map[string]string, error) {
	all := make(map[string]string)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		var parsed map[string]string
		var parseErr error

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".json":
			parsed, parseErr = ParseJSONFile(path)
		case ".yaml", ".yml":
			parsed, parseErr = ParseYAMLFile(path)
		default:
			if looksLikeEnvFile(path) {
				parsed, parseErr = ParseEnvFile(path)
			}
		}

		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "aibodyguard: warning: could not parse %s: %v\n", path, parseErr)
			return nil
		}

		for k, v := range parsed {
			if isLikelySecret(v) {
				all[k] = v
			}
		}
		return nil
	})

	return all, err
}

// looksLikeEnvFile returns true if the file contains at least one KEY=VALUE line.
func looksLikeEnvFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	count := 0
	for sc.Scan() {
		count++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		if strings.Contains(line, "=") {
			return true
		}
		if count > 20 {
			break
		}
	}
	return false
}

// isLikelySecret returns true if a value looks like a real secret.
func isLikelySecret(v string) bool {
	if len(v) < 8 {
		return false
	}
	lower := strings.ToLower(v)
	nonSecrets := []string{
		"true", "false", "null", "none", "undefined",
		"localhost", "127.0.0.1", "0.0.0.0",
	}
	for _, ns := range nonSecrets {
		if lower == ns {
			return false
		}
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return false
	}
	allDigits := true
	for _, c := range v {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	return !allDigits
}
