package connector

import (
	"crypto/sha256"
	"encoding/hex"
)

// Raw capture and reading digests cover original bytes. Stable entry identity
// and source input use the frozen typed-LP projections in the web normalizer.
func knowledgeWebHash(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
