package knowledgeaccess

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const ramInputTimeout = 10 * time.Second

// The go test driver does not supply a RAM session stream on its subprocess
// stdin. A separately admitted named pipe survives that process boundary.
// Regular files, symlinks and network URLs cannot carry test authentication.
func openRAMPipe(name string) (io.ReadCloser, error) {
	if runtime.GOOS == "windows" {
		const prefix = `\\.\pipe\lfen-access-s1-`
		if !strings.HasPrefix(name, prefix) || len(name) <= len(prefix) || strings.ContainsAny(strings.TrimPrefix(name, prefix), `/\:?#`) {
			return nil, errScope
		}
	} else {
		if !filepath.IsAbs(name) || filepath.Clean(name) != name {
			return nil, errScope
		}
		info, err := os.Lstat(name)
		if err != nil || info.Mode()&os.ModeNamedPipe == 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return nil, errScope
		}
	}
	type opened struct {
		file *os.File
		err  error
	}
	result := make(chan opened)
	cancel := make(chan struct{})
	go func() {
		file, err := os.Open(name)
		select {
		case result <- opened{file, err}:
		case <-cancel:
			if file != nil {
				_ = file.Close()
			}
		}
	}()
	timer := time.NewTimer(ramInputTimeout)
	defer timer.Stop()
	select {
	case value := <-result:
		if value.err != nil || value.file == nil {
			return nil, errScope
		}
		info, err := value.file.Stat()
		if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
			_ = value.file.Close()
			return nil, errScope
		}
		return value.file, nil
	case <-timer.C:
		close(cancel)
		return nil, errScope
	}
}

func readAdmittedRAM(source io.ReadCloser, expectedSHA string, c *contractSet) (*preparedRun, error) {
	defer source.Close()
	type loaded struct {
		run *preparedRun
		err error
	}
	result := make(chan loaded, 1)
	go func() { run, err := loadRun(source, expectedSHA, c); result <- loaded{run, err} }()
	timer := time.NewTimer(ramInputTimeout)
	defer timer.Stop()
	select {
	case value := <-result:
		return value.run, value.err
	case <-timer.C:
		_ = source.Close()
		return nil, errScope
	}
}
