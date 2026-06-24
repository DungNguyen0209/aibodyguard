package watcher

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DungNguyen0209/aibodyguard/internal/detector"
	"github.com/DungNguyen0209/aibodyguard/internal/parser"
)

// Watcher monitors credential files for changes and reports newly
// discovered secrets. It maintains a hashmap (absolutePath → modTime)
// and compares on each Check call, re-parsing only changed/new files.
type Watcher struct {
	root      string
	det       *detector.Detector
	files     map[string]*fileInfo // absolutePath → last known state
	allSeen   map[string]struct{}
	mu        sync.Mutex
	lastCheck time.Time
	checkMin  time.Duration
}

type fileInfo struct {
	modTime int64 // UnixNano
	values  []string
}

// New returns a Watcher rooted at root. det is used for ML detection
// (may be nil). The initial scan populates the hashmap and allSeen.
func New(root string, det *detector.Detector) (*Watcher, error) {
	w := &Watcher{
		root:     root,
		det:      det,
		files:    make(map[string]*fileInfo),
		allSeen:  make(map[string]struct{}),
		checkMin: 1 * time.Second,
	}
	w.mu.Lock()
	_, err := w.scanLocked()
	w.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return w, nil
}

// Check walks the tree and re-parses credential files whose modTime
// has changed or that are not yet tracked in the hashmap.
// Returns secret values not seen in previous Check calls.
// A 1-second debounce prevents redundant work on rapid successive calls.
func (w *Watcher) Check() ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if time.Since(w.lastCheck) < w.checkMin {
		return nil, nil
	}
	w.lastCheck = time.Now()
	return w.scanLocked()
}

// scanLocked walks the tree, re-parses changed/new credential files,
// and returns secret values not seen before.
// Caller must hold w.mu.
func (w *Watcher) scanLocked() ([]string, error) {
	seenPaths := make(map[string]bool)

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

		seenPaths[path] = true
		ext := strings.ToLower(filepath.Ext(path))
		if sourceCodeExts[ext] {
			return nil
		}

		if !isCredentialFile(path, ext) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		modTime := info.ModTime().UnixNano()

		if fi, ok := w.files[path]; ok && fi.modTime == modTime {
			return nil
		}

		vals, err := parseCredentialFile(path, w.det)
		if err != nil {
			return nil
		}

		w.files[path] = &fileInfo{
			modTime: modTime,
			values:  vals,
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	current := make(map[string]struct{})
	for _, fi := range w.files {
		for _, v := range fi.values {
			if v != "" {
				current[v] = struct{}{}
			}
		}
	}

	var newSecrets []string
	for v := range current {
		if _, seen := w.allSeen[v]; !seen {
			newSecrets = append(newSecrets, v)
		}
	}

	for v := range current {
		w.allSeen[v] = struct{}{}
	}

	for path := range w.files {
		if !seenPaths[path] {
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
	".java": true, ".kt": true, ".kts": true, ".groovy": true, ".scala": true,
	".cs": true, ".vb": true, ".fs": true, ".fsx": true, ".csproj": true,
	".vbproj": true, ".fsproj": true, ".sln": true,
	".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".mts": true,
	".cts": true, ".jsx": true, ".tsx": true, ".vue": true, ".svelte": true,
	".py": true, ".pyw": true, ".pyc": true, ".pyo": true,
	".rb": true, ".rake": true, ".gemspec": true,
	".php": true, ".phtml": true,
	".go": true,
	".rs": true,
	".c": true, ".h": true, ".cpp": true, ".cc": true, ".cxx": true,
	".hpp": true, ".hh": true,
	".swift": true, ".m": true, ".mm": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".ps1": true,
	".psm1": true, ".psd1": true,
	".html": true, ".htm": true, ".css": true, ".scss": true, ".sass": true,
	".less": true, ".xml": true, ".xhtml": true, ".xsl": true, ".xslt": true,
	".class": true, ".jar": true, ".war": true, ".ear": true,
	".o": true, ".obj": true, ".a": true,
	".lib": true, ".dll": true, ".so": true, ".dylib": true, ".exe": true,
	".lock": true, ".sum": true,
	".md": true, ".mdx": true, ".rst": true, ".txt": true, ".adoc": true,
	".tex": true, ".ipynb": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".ico": true, ".webp": true, ".mp4": true, ".mp3": true, ".pdf": true,
}

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

	for _, vals := range []map[string]string{parsed, commented} {
		for _, v := range vals {
			if parser.IsLikelySecret(v) {
				seen[v] = struct{}{}
			}
		}
	}

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
