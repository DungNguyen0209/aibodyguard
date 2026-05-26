package uninstall

import (
	"os"
)

// RemoveCacheDir removes the entire cacheDir tree.
// Returns (true, nil) if removed, (false, nil) if it didn't exist, or (false, err) on failure.
func RemoveCacheDir(cacheDir string) (bool, error) {
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return false, nil
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveTempFiles removes each path in the list if it exists.
// Returns the list of paths that were actually removed.
func RemoveTempFiles(paths []string) []string {
	var removed []string
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			removed = append(removed, p)
		}
	}
	return removed
}

// RemoveBinary removes the file at binaryPath.
// Returns (true, nil) if removed, (false, nil) if it didn't exist, or (false, err) on failure.
func RemoveBinary(binaryPath string) (bool, error) {
	if err := os.Remove(binaryPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
