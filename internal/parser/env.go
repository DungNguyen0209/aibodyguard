package parser

import (
	"bufio"
	"os"
	"strings"
)

// ParseEnvFile parses a .env or .properties file and returns key=value pairs.
// Active lines and commented-out lines are both parsed — a commented-out
// credential is still a credential. Lines starting with # or ! that do not
// contain a valid key=value after stripping the comment marker are silently
// skipped. Values are unquoted (strips surrounding " or '). Empty values are
// excluded.
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
		if line == "" {
			continue
		}

		// Strip leading comment markers and try to parse as key=value.
		// Handles: #KEY=val, # KEY=val, ##KEY=val, !KEY=val
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			line = strings.TrimLeft(line, "#!")
			line = strings.TrimSpace(line)
			// After stripping, attempt to parse — skip if no = found
			if k, v, ok := parseKeyValue(line); ok {
				result[k] = v
			}
			continue
		}

		if k, v, ok := parseKeyValue(line); ok {
			result[k] = v
		}
	}
	return result, scanner.Err()
}

// parseKeyValue splits "key=value", unquotes the value, and returns
// (key, value, true). Returns ("", "", false) if the line has no = or
// produces an empty key or value.
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
