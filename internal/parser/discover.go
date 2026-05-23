package parser

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// fileParser is the concrete implementation of Parser.
type fileParser struct{}

// Discover walks root recursively, parses all credential files,
// and returns a merged map of key -> secret value.
func (p *fileParser) Discover(root string) (map[string]string, error) {
	return DiscoverSecrets(root)
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
		var parseErr error

		base := strings.ToLower(filepath.Base(path))

		switch ext {
		case ".json":
			// Only parse JSON files whose name suggests they hold config/secrets.
			if isCredentialJSON(base) {
				parsed, parseErr = ParseJSONFile(path)
			}
		case ".yaml", ".yml":
			// Only parse YAML files whose name suggests they hold config/secrets.
			if isCredentialYAML(base) {
				parsed, parseErr = ParseYAMLFile(path)
			}
		case ".properties":
			parsed, parseErr = ParseEnvFile(path)
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
