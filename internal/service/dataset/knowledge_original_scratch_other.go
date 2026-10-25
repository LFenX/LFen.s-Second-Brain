//go:build !linux

package dataset

import "os"

func originalStagingOwned(os.FileInfo) bool { return false }
