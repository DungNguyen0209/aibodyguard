package parser

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DungNguyen0209/aibodyguard/internal/detector"
)

// fileParser is the concrete implementation of Parser.
type fileParser struct{}

// Discover walks root recursively, parses all credential files,
// and returns a merged map of key -> secret value.
func (p *fileParser) Discover(root string, det *detector.Detector) (map[string][]string, error) {
	return DiscoverSecrets(root, det)
}

var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	"target":       true,
	"build":        true,
	"dist":         true,
	".gradle":      true,
	".mvn":         true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	".tox":         true,
	"coverage":     true,
	".nyc_output":  true,
}

// skipPathSegments is a list of substrings checked against every path component.
// If any component matches, the file (or directory) is skipped entirely.
// This catches localization directories regardless of where they appear in the tree.
var skipPathSegments = []string{
	"locali", // localize, localization, locales, locale, localized
	"i18n",
	"l10n",
	"translations",
	"translation",
	"messages",  // Spring MessageSource bundles (messages_en.properties, etc.)
	"intl",
}

// containsSkippedSegment returns true if any path component matches a skipPathSegments entry.
func containsSkippedSegment(path string) bool {
	// Normalize to forward slashes for consistent splitting
	normalized := filepath.ToSlash(path)
	for _, part := range strings.Split(normalized, "/") {
		lower := strings.ToLower(part)
		for _, seg := range skipPathSegments {
			if strings.Contains(lower, seg) {
				return true
			}
		}
	}
	return false
}

// sourceCodeExts is the set of file extensions that belong to programming
// languages or build systems. These files are never credential stores and
// must be skipped to avoid false positives from string literals in code.
var sourceCodeExts = map[string]bool{
	// JVM
	".java": true, ".kt": true, ".kts": true, ".groovy": true, ".scala": true,
	// .NET
	".cs": true, ".vb": true, ".fs": true, ".fsx": true, ".csproj": true,
	".vbproj": true, ".fsproj": true, ".sln": true,
	// JavaScript / TypeScript
	".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".mts": true,
	".cts": true, ".jsx": true, ".tsx": true, ".vue": true, ".svelte": true,
	// Python
	".py": true, ".pyw": true, ".pyc": true, ".pyo": true,
	// Ruby
	".rb": true, ".rake": true, ".gemspec": true,
	// PHP
	".php": true, ".phtml": true,
	// Go
	".go": true,
	// Rust
	".rs": true,
	// C / C++
	".c": true, ".h": true, ".cpp": true, ".cc": true, ".cxx": true,
	".hpp": true, ".hh": true,
	// Swift / Objective-C
	".swift": true, ".m": true, ".mm": true,
	// Shell
	".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".ps1": true,
	".psm1": true, ".psd1": true,
	// Web / markup
	".html": true, ".htm": true, ".css": true, ".scss": true, ".sass": true,
	".less": true, ".xml": true, ".xhtml": true, ".xsl": true, ".xslt": true,
	// Compiled / binary artifacts
	".class": true, ".jar": true, ".war": true, ".ear": true,
	".o": true, ".obj": true, ".a": true,
	".lib": true, ".dll": true, ".so": true, ".dylib": true, ".exe": true,
	// Lock files / generated
	".lock": true, ".sum": true,
	// Docs / templates
	".md": true, ".mdx": true, ".rst": true, ".txt": true, ".adoc": true,
	".tex": true, ".ipynb": true,
	// Images / media
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".ico": true, ".webp": true, ".mp4": true, ".mp3": true, ".pdf": true,
}

// mergeInto adds values from src into dst, deduplicating per key.
// Values that do not pass isLikelySecret are skipped.
func mergeInto(dst map[string][]string, src map[string]string) {
	for k, v := range src {
		if !isLikelySecret(v) {
			continue
		}
		already := false
		for _, existing := range dst[k] {
			if existing == v {
				already = true
				break
			}
		}
		if !already {
			dst[k] = append(dst[k], v)
		}
	}
}

// DiscoverSecrets walks root recursively, parses all credential files,
// and returns a merged map of key -> secret value.
// Values that are too short or look like non-secrets are filtered out.
// If det is non-nil and available, ML-based detection is also run on each file.
func DiscoverSecrets(root string, det *detector.Detector) (map[string][]string, error) {
	all := make(map[string][]string)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			// Skip localization/i18n directory trees entirely.
			if containsSkippedSegment(path) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip files inside localization/i18n paths.
		if containsSkippedSegment(path) {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))

		// Skip source code and binary file types — never credential stores.
		if sourceCodeExts[ext] {
			return nil
		}

		var parsed map[string]string
		var commented map[string]string
		var parseErr error

		base := strings.ToLower(filepath.Base(path))

		switch ext {
		case ".json":
			if isCredentialJSON(base) {
				parsed, parseErr = ParseJSONFile(path)
				// JSON does not have a line-comment syntax — no commented parser
			}
		case ".yaml", ".yml":
			if isCredentialYAML(base) {
				parsed, parseErr = ParseYAMLFile(path)
				if parseErr == nil {
					commented, _ = ParseCommentedYAMLFile(path)
				}
			}
		case ".properties":
			if !isBinaryFile(path) {
				parsed, parseErr = ParseEnvFile(path)
				if parseErr == nil {
					commented, _ = ParseCommentedEnvFile(path)
				}
			}
		default:
			if isKnownEnvFile(base) && !isBinaryFile(path) {
				parsed, parseErr = ParseEnvFile(path)
				if parseErr == nil {
					commented, _ = ParseCommentedEnvFile(path)
				}
			}
		}

		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "aibodyguard: warning: could not parse %s: %v\n", path, parseErr)
			return nil
		}

		mergeInto(all, parsed)
		mergeInto(all, commented)

		// ML detection: only run on files we actually parsed (recognized credential stores).
		// Running ML on every file would be prohibitively slow.
		if det != nil && det.Available() && parsed != nil {
			raw, readErr := os.ReadFile(path)
			if readErr == nil {
				mlSecrets, mlErr := det.DetectFromContent(string(raw))
				if mlErr == nil {
					for _, s := range mlSecrets {
						if s == "" {
							continue
						}
						already := false
						for _, existing := range all["_ml"] {
							if existing == s {
								already = true
								break
							}
						}
						if !already {
							all["_ml"] = append(all["_ml"], s)
						}
					}
				}
			}
		}

		return nil
	})

	return all, err
}

// credentialJSONNames is the set of JSON filenames (lowercased, without path)
// that are likely to contain credentials or configuration secrets.
var credentialJSONNames = map[string]bool{
	// Generic config/secrets
	"config.json": true, "configuration.json": true,
	"secrets.json": true, "secret.json": true,
	"credentials.json": true, "credential.json": true,
	// Cloud / service accounts
	"service-account.json": true, "service_account.json": true,
	"keyfile.json": true, "key.json": true,
	"gcloud.json": true,
	// App-specific
	"appsettings.json": true, "appsettings.development.json": true,
	"appsettings.production.json": true, "appsettings.staging.json": true,
	"appsettings.test.json": true,
	"settings.json": true,
	"local.settings.json": true,   // Azure Functions
	"firebase.json": true,
	"firebase-adminsdk.json": true,
	".firebaserc": true,
	"auth.json": true,
	"vault.json": true,
	// Terraform / infra
	"terraform.tfvars.json": true,
	"variables.json": true,
	// npm / node (has tokens in some setups)
	".npmrc": true,
}

// credentialJSONPrefixes are name prefixes (lowercased) that suggest a JSON
// file holds environment-specific configuration.
var credentialJSONPrefixes = []string{
	"config.", "configuration.", "settings.", "secrets.", "credentials.",
	"creds.", "cred.",
	"appsettings.", "env.", "environment.",
}

// isCredentialJSON returns true if a JSON filename looks like a credential/config file.
func isCredentialJSON(base string) bool {
	if credentialJSONNames[base] {
		return true
	}
	for _, prefix := range credentialJSONPrefixes {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	// Any filename containing "setting" (e.g. appsettings-local.json, site-settings.json)
	if strings.Contains(base, "setting") {
		return true
	}
	return false
}

// credentialYAMLNames is the set of YAML filenames likely to hold secrets.
var credentialYAMLNames = map[string]bool{
	// Spring Boot
	"application.yml": true, "application.yaml": true,
	"bootstrap.yml": true, "bootstrap.yaml": true,
	// Profile-specific Spring Boot (matched by prefix below)
	// Generic
	"config.yml": true, "config.yaml": true,
	"configuration.yml": true, "configuration.yaml": true,
	"secrets.yml": true, "secrets.yaml": true,
	"credentials.yml": true, "credentials.yaml": true,
	// Docker Compose picks up .env already; docker-compose has no secrets directly
	// Kubernetes / Helm
	"values.yml": true, "values.yaml": true,
	"values-dev.yml": true, "values-dev.yaml": true,
	"values-prod.yml": true, "values-prod.yaml": true,
	"values-staging.yml": true, "values-staging.yaml": true,
	// Ansible
	"vault.yml": true, "vault.yaml": true,
	// Ruby on Rails
	"database.yml": true, "database.yaml": true,
	"credentials.yml.enc": true,
	// Generic env overrides
	"local.yml": true, "local.yaml": true,
	"override.yml": true, "override.yaml": true,
}

// credentialYAMLPrefixes are name prefixes that suggest a YAML file holds secrets.
var credentialYAMLPrefixes = []string{
	"application-", "application.", // Spring profiles: application-dev.yml
	"bootstrap-", "bootstrap.",
	"config.", "configuration.",
	"secrets.", "credentials.",
	"values.", "values-",
	"env.", "environment.",
}

// isCredentialYAML returns true if a YAML filename looks like a credential/config file.
func isCredentialYAML(base string) bool {
	if credentialYAMLNames[base] {
		return true
	}
	for _, prefix := range credentialYAMLPrefixes {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	// Any filename containing "value" or "setting" (e.g. gots-values.yaml, appsettings-prod.yml)
	if strings.Contains(base, "value") || strings.Contains(base, "setting") {
		return true
	}
	return false
}

// isBinaryFile returns true if the file appears to be binary (contains null bytes in first 512 bytes).
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false
	}
	for _, b := range buf[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}

// isKnownEnvFile returns true if the filename is a known env-style credentials file.
// This is intentionally strict: only .env, .env.*, and .envrc are accepted.
// We do NOT accept arbitrary files with KEY=VALUE lines (too many false positives).
func isKnownEnvFile(base string) bool {
	return base == ".env" || base == ".envrc" ||
		strings.HasPrefix(base, ".env.") ||
		strings.HasPrefix(base, "env.") ||
		base == ".netrc"
}

// looksLikeEnvFile returns true if the file contains at least one KEY=VALUE line.
func looksLikeEnvFile(path string) bool {
	if isBinaryFile(path) {
		return false
	}
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

// isLikelySecret returns true if a value looks like a real secret credential.
// It uses a positive signal approach: values must show characteristics of real
// secrets (sufficient length + entropy markers) rather than just "not obviously not a secret".
func isLikelySecret(v string) bool {
	// Must be printable ASCII only — binary data is never a secret we want to redact
	for _, c := range v {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}

	// Minimum length for a meaningful secret
	if len(v) < 10 {
		return false
	}

	// Skip Spring/shell variable placeholders like ${VAR} or ${VAR:default}
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
		return false
	}

	// Skip URL path templates like /v1/id/%s or /customer/v3/byId
	if strings.HasPrefix(v, "/") {
		return false
	}

	// Skip cron expressions (contain digits and */- patterns)
	if strings.Contains(v, "* * * *") || strings.Contains(v, "*/") {
		return false
	}

	lower := strings.ToLower(v)

	// Hard exclusions — known non-secret values
	hardExclusions := []string{
		"true", "false", "null", "none", "undefined", "enabled", "disabled",
		"localhost", "127.0.0.1", "0.0.0.0", "::1",
		"development", "production", "staging", "test", "testing",
		"frontend", "backend", "fullstack",
		"sameorigin", "nosniff", "strict-origin-when-cross-origin",
		"info", "debug", "warn", "error", "warning",
	}
	for _, ex := range hardExclusions {
		if lower == ex {
			return false
		}
	}

	// Skip URLs (but keep JDBC — they contain hostnames and credentials)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mongodb://") ||
		strings.HasPrefix(lower, "redis://") || strings.HasPrefix(lower, "amqp://") {
		return false
	}

	// Skip values that look like config paths or enum strings:
	// - Only contains letters, digits, dots, dashes, underscores, slashes, colons
	// - No mix of cases with digits → not a token/key
	onlyConfigChars := true
	hasDigit := false
	hasUpper := false
	hasLower := false
	hasSpecial := false // chars beyond [a-zA-Z0-9._\-/:@]
	for _, c := range v {
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c == '.' || c == '-' || c == '_' || c == '/' || c == ':' || c == '@':
			// allowed config chars — keep onlyConfigChars true
		default:
			hasSpecial = true
			onlyConfigChars = false
		}
	}

	// Pure config-path values (only letters+dots+dashes+underscores, no digits, no specials)
	if onlyConfigChars && !hasDigit && !hasSpecial {
		return false
	}

	// All-caps with underscores and no digits → looks like an env var name used as value
	if hasUpper && !hasLower && !hasDigit && !hasSpecial {
		return false
	}

	// Values containing spaces are likely descriptions/sentences, not secrets,
	// unless they have special characters (e.g. regex patterns like "Bearer (?<token>.*)$")
	if strings.Contains(v, " ") && !hasSpecial {
		return false
	}

	// Values that are all-lowercase letters/dots (config enum strings like "sameorigin", "exchangeScope")
	if !hasUpper && !hasDigit && !hasSpecial && len(v) < 30 {
		return false
	}

	// Short values with no special chars or digits are not secrets
	if !hasDigit && !hasSpecial && len(v) < 20 {
		return false
	}

	// Require at least some complexity: either special chars, or mixed case+digits, or long enough.
	// Real secrets: API keys, tokens, passwords typically have digits + mixed case or special chars.
	complexityScore := 0
	if hasDigit {
		complexityScore++
	}
	if hasUpper && hasLower {
		complexityScore++
	}
	if hasSpecial {
		complexityScore++
	}
	if len(v) >= 20 {
		complexityScore++
	}
	if len(v) >= 32 {
		complexityScore++
	}

	// Short values need higher complexity (2 signals).
	// Values ≥ 16 chars with at least one complexity signal are likely secrets.
	// Values ≥ 10 chars that mix letters AND digits (no pure words, no pure numbers)
	// are likely tokens/keys even if short (e.g. "abc12345678", "Mb2.r5oHf-0t").
	if len(v) >= 16 {
		return complexityScore >= 1
	}
	if len(v) >= 10 && hasDigit && (hasLower || hasUpper) {
		return true
	}
	return complexityScore >= 2
}
