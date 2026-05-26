package parser

import "github.com/DungNguyen0209/aibodyguard/internal/detector"

// Parser discovers credential key/value pairs from files rooted at a directory.
type Parser interface {
	Discover(root string, det *detector.Detector) (map[string][]string, error)
}

// New returns a Parser backed by the filesystem implementation.
func New() Parser {
	return &fileParser{}
}
