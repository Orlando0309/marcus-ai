package context

import (
	"bytes"
	"net/http"
	"strings"
)

// ProbablyBinary uses a fast heuristic (NUL bytes + MIME sniff on prefix).
func ProbablyBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	prefix := b
	if len(prefix) > 512 {
		prefix = prefix[:512]
	}
	if bytes.IndexByte(prefix, 0) >= 0 {
		return true
	}
	ct := http.DetectContentType(prefix)
	switch {
	case strings.HasPrefix(ct, "image/"),
		strings.HasPrefix(ct, "audio/"),
		strings.HasPrefix(ct, "video/"),
		ct == "application/octet-stream",
		ct == "application/zip",
		ct == "application/gzip",
		ct == "application/pdf":
		return true
	default:
		return false
	}
}
