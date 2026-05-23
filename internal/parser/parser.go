package parser

// Parser discovers credential key/value pairs from files rooted at a directory.
type Parser interface {
	Discover(root string) (map[string]string, error)
}

// New returns a Parser backed by the filesystem implementation.
func New() Parser {
	return &fileParser{}
}
