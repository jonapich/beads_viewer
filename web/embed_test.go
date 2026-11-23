package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedFSHasIndex(t *testing.T) {
	data, err := fs.ReadFile(FS, "index.html")
	if err != nil {
		t.Fatalf("embedded web/dist must contain index.html (committed placeholder): %v", err)
	}
	if !strings.Contains(strings.ToLower(string(data)), "<html") {
		t.Fatalf("index.html is not an HTML document: %q", data)
	}
}
