package parser

import (
	"bufio"
	"os"
	"strings"
)

// ParseEnvFile parses a .env or .properties file and returns key=value pairs.
// Only active (non-commented) lines are parsed.
// Lines starting with # or ! are skipped entirely.
// Values are unquoted (strips surrounding " or '). Empty values are excluded.
func ParseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		if k, v, ok := parseKeyValue(line); ok {
			result[k] = v
		}
	}
	return result, scanner.Err()
}

// ParseCommentedEnvFile parses commented-out key=value lines from a .env or
// .properties file. Lines starting with # or ! are stripped of their comment
// marker and the remainder is attempted as key=value. Lines that do not parse
// as key=value (plain prose comments) are silently skipped.
// This function never affects the result of ParseEnvFile.
func ParseCommentedEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "!") {
			continue
		}
		// Strip all leading comment markers and whitespace
		line = strings.TrimLeft(line, "#!")
		line = strings.TrimSpace(line)
		if k, v, ok := parseKeyValue(line); ok {
			result[k] = v
		}
	}
	return result, scanner.Err()
}

// parseKeyValue splits "key=value", unquotes the value, and returns
// (key, value, true). Returns ("", "", false) if the line has no =,
// or produces an empty key or value.
func parseKeyValue(line string) (string, string, bool) {
	idx := strings.Index(line, "=")
	if idx < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	val := strings.TrimSpace(line[idx+1:])

	// strip surrounding quotes
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') ||
			(val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
	}
	if key == "" || val == "" {
		return "", "", false
	}
	return key, val, true
}
