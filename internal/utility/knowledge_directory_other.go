//go:build !linux

package utility

import (
	"errors"
	"os"
)

// Unverified platforms never expose a readable root.
func KnowledgeDirectoryPlatformVerified() bool                      { return false }
func knowledgeDirectoryOpenFlags() int                              { return os.O_RDONLY }
func knowledgeDirectoryIdentity(os.FileInfo) (string, uint64, bool) { return "", 0, false }
func knowledgeDirectoryOpen(*os.Root, string) (*os.File, error) {
	return nil, errors.New("unverified platform")
}
func knowledgeDirectoryStorageVerified(*os.Root) bool { return false }
