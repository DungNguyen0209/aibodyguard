package watcher

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DungNguyen0209/aibodyguard/internal/detector"
	"github.com/DungNguyen0209/aibodyguard/internal/parser"
)

// Watcher monitors credential files for changes and reports newly
// discovered secrets. It tracks files by path + modtime + size, so
// unchanged files are skipped without re-parsing.
type Watcher struct {
	root    string
	det     *detector.Detector
	files   map[string]*cachedFile
	allSeen map[string]struct{}
}

// cachedFile holds the last known state of one credential file.
type cachedFile struct {
	modTime int64
	size    int64
	values  []string
}

// New returns a Watcher rooted at root. det is used for ML detection
// on changed files (may be nil). The initial scan populates the cache.
func New(root string, det *detector.Detector) (*Watcher, error) {
	w := &Watcher{
		root:    root,
		det:     det,
		files:   make(map[string]*cachedFile),
		allSeen: make(map[string]struct{}),
	}
	if _, err := w.Scan(); err != nil {
		return nil, err
	}
	return w, nil
}

// Scan walks the tree, re-parses changed or new credential files,
// and returns any secret values not seen in previous Scan calls.
func (w *Watcher) Scan() ([]string, error) {
	newFiles := make(map[string]bool)
	var newSecrets []string

	err := filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if containsSkippedSegment(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if containsSkippedSegment(path) {
			return nil
		}

		newFiles[path] = true
		ext := strings.ToLower(filepath.Ext(path))
		if sourceCodeExts[ext] {
			return nil
		}

		if !isCredentialFile(path, ext) {
			return nil
		}

		// Stat to check modtime
		info, err := d.Info()
		if err != nil {
			return nil
		}
		modTime := info.ModTime().UnixNano()
		size := info.Size()

		// Check cache
		if cached, ok := w.files[path]; ok && cached.modTime == modTime && cached.size == size {
			return nil // unchanged
		}

		// Parse file
		vals, err := parseCredentialFile(path, w.det)
		if err != nil {
			return nil // best-effort
		}

		w.files[path] = &cachedFile{
			modTime: modTime,
			size:    size,
			values:  vals,
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Collect all values from all cached files
	current := make(map[string]struct{})
	for _, cf := range w.files {
		for _, v := range cf.values {
			if v != "" {
				current[v] = struct{}{}
			}
		}
	}

	// Diff against seen history
	for v := range current {
		if _, seen := w.allSeen[v]; !seen {
			newSecrets = append(newSecrets, v)
		}
	}

	// Update seen history
	for v := range current {
		w.allSeen[v] = struct{}{}
	}

	// Clean up cache entries for deleted files
	for path := range w.files {
		if !newFiles[path] {
			delete(w.files, path)
		}
	}

	return newSecrets, nil
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

var skipPathSegments = []string{
	"locali", "i18n", "l10n",
	"translations", "translation",
	"messages", "intl",
}

func containsSkippedSegment(path string) bool {
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

// credential filename patterns

var credentialEnvNames = map[string]bool{
	".env": true, ".envrc": true,
}

var credentialJSONNames = map[string]bool{
	"config.json": true, "configuration.json": true,
	"secrets.json": true, "secret.json": true,
	"credentials.json": true, "credential.json": true,
	"service-account.json": true, "service_account.json": true,
	"keyfile.json": true, "key.json": true,
	"appsettings.json": true, "settings.json": true,
	"local.settings.json": true, "firebase.json": true,
	"auth.json": true, "vault.json": true,
	"terraform.tfvars.json": true, ".npmrc": true,
}

var credentialJSONPrefixes = []string{
	"config.", "configuration.", "settings.", "secrets.", "credentials.",
	"creds.", "cred.", "appsettings.", "env.", "environment.",
}

var credentialYAMLNames = map[string]bool{
	"application.yml": true, "application.yaml": true,
	"bootstrap.yml": true, "bootstrap.yaml": true,
	"config.yml": true, "config.yaml": true,
	"secrets.yml": true, "secrets.yaml": true,
	"credentials.yml": true, "credentials.yaml": true,
	"values.yml": true, "values.yaml": true,
	"vault.yml": true, "vault.yaml": true,
	"database.yml": true, "database.yaml": true,
	"local.yml": true, "local.yaml": true,
}

var credentialYAMLPrefixes = []string{
	"application-", "application.",
	"bootstrap-", "bootstrap.",
	"config.", "configuration.",
	"secrets.", "credentials.",
	"values.", "values-",
	"env.", "environment.",
}

func isCredentialFile(path, ext string) bool {
	base := strings.ToLower(filepath.Base(path))

	switch ext {
	case ".properties":
		return true
	case ".json":
		if credentialJSONNames[base] {
			return true
		}
		for _, prefix := range credentialJSONPrefixes {
			if strings.HasPrefix(base, prefix) {
				return true
			}
		}
		if strings.Contains(base, "setting") {
			return true
		}
		return false
	case ".yaml", ".yml":
		if credentialYAMLNames[base] {
			return true
		}
		for _, prefix := range credentialYAMLPrefixes {
			if strings.HasPrefix(base, prefix) {
				return true
			}
		}
		if strings.Contains(base, "value") || strings.Contains(base, "setting") {
			return true
		}
		return false
	default:
		if credentialEnvNames[base] {
			return true
		}
		if strings.HasPrefix(base, ".env.") || strings.HasPrefix(base, "env.") {
			return true
		}
		if base == ".netrc" {
			return true
		}
		return false
	}
}

// parseCredentialFile reads a credential file and returns all secret values.
// Uses heuristic + ML detection, same as parser.DiscoverSecrets.
func parseCredentialFile(path string, det *detector.Detector) ([]string, error) {
	var parsed map[string]string
	var commented map[string]string
	var parseErr error

	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".json":
		parsed, parseErr = parser.ParseJSONFile(path)
	case ".yaml", ".yml":
		parsed, parseErr = parser.ParseYAMLFile(path)
		if parseErr == nil {
			commented, _ = parser.ParseCommentedYAMLFile(path)
		}
	case ".properties":
		parsed, parseErr = parser.ParseEnvFile(path)
		if parseErr == nil {
			commented, _ = parser.ParseCommentedEnvFile(path)
		}
	default:
		parsed, parseErr = parser.ParseEnvFile(path)
		if parseErr == nil {
			commented, _ = parser.ParseCommentedEnvFile(path)
		}
	}

	if parseErr != nil {
		return nil, parseErr
	}

	seen := make(map[string]struct{})

	// Heuristic: collect values that pass IsLikelySecret
	for _, vals := range []map[string]string{parsed, commented} {
		for _, v := range vals {
			if parser.IsLikelySecret(v) {
				seen[v] = struct{}{}
			}
		}
	}

	// ML detection on raw content (only if file was actually parsed)
	if det != nil && det.Available() && parsed != nil {
		raw, readErr := os.ReadFile(path)
		if readErr == nil {
			mlSecrets, mlErr := det.DetectFromContent(string(raw))
			if mlErr == nil {
				for _, s := range mlSecrets {
					if s == "" || !parser.IsLikelySecret(s) {
						continue
					}
					seen[s] = struct{}{}
				}
			}
		}
	}

	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}
	return result, nil
}
