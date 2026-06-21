// Package grant provides cryptographic sign/verify primitives for
// permission grants that authenticate agent↔resource access.
package grant

import (
	"crypto/hmac"
	"crypto/sha256"
)

// Sign creates an HMAC-SHA256 signature of data using the provided key.
func Sign(data, key []byte) ([]byte, error) {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil), nil
}

// Verify checks whether sig is a valid HMAC-SHA256 signature of data
// under the given key.
func Verify(sig, data, key []byte) bool {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}
