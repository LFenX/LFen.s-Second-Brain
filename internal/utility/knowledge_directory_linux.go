//go:build linux

package utility

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func KnowledgeDirectoryPlatformVerified() bool { return true }
func knowledgeDirectoryOpenFlags() int         { return os.O_RDONLY | syscall.O_NONBLOCK | syscall.O_NOFOLLOW }
func knowledgeDirectoryOpen(root *os.Root, name string) (*os.File, error) {
	base, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer base.Close()
	fd, err := unix.Openat2(int(base.Fd()), name, &unix.OpenHow{Flags: uint64(knowledgeDirectoryOpenFlags() | syscall.O_CLOEXEC), Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return nil, os.NewSyscallError("openat2", err)
	}
	return os.NewFile(uintptr(fd), name), nil
}
func knowledgeDirectoryIdentity(info os.FileInfo) (string, uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", 0, false
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), uint64(stat.Nlink), true
}

func knowledgeDirectoryStorageVerified(root *os.Root) bool {
	f, err := root.Open(".")
	if err != nil {
		return false
	}
	defer f.Close()
	var stat unix.Statfs_t
	if unix.Fstatfs(int(f.Fd()), &stat) != nil {
		return false
	}
	switch int64(stat.Type) {
	case 0xef53, 0x01021994, 0x794c7630, 0x58465342, 0x9123683e:
		return true
	}
	return false
}
