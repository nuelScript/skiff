package router

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

// tokenAllowed reports whether the request carries one of the app's access tokens, given as SHA-256 hashes.
func tokenAllowed(r *http.Request, hashes []string) bool {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || strings.TrimSpace(tok) == "" {
		return false
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(tok)))
	got := hex.EncodeToString(sum[:])
	match := 0
	for _, h := range hashes {
		match |= subtle.ConstantTimeCompare([]byte(got), []byte(h))
	}
	return match == 1
}
