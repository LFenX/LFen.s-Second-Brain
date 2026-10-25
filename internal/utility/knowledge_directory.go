package utility

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// IsKnowledgeDirectoryPath validates an exact relative path, never a glob.
func IsKnowledgeDirectoryPath(value string, directory bool) bool {
	if directory && value == "." {
		return true
	}
	if value == "" || len(value) > 512 || !utf8.ValidString(value) || strings.ContainsAny(value, "\\:*?[]{}") || strings.ContainsFunc(value, unicode.IsControl) {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) > 32 {
		return false
	}
	for _, part := range parts {
		lower := strings.ToLower(part)
		base := strings.Split(lower, ".")[0]
		device := slices.Contains([]string{"con", "prn", "aux", "nul"}, base) || (len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '1' && base[3] <= '9')
		if part == "" || part == "." || part == ".." || part != strings.TrimSpace(part) || strings.HasSuffix(part, ".") || device || strings.HasPrefix(lower, ".env") || slices.Contains([]string{".git", ".svn", "node_modules", "vendor", "dist", "build", ".local-data", ".ssh", ".aws", "credentials", "id_rsa", "id_ed25519"}, lower) {
			return false
		}
	}
	return !slices.Contains([]string{".pem", ".key", ".p12", ".pfx"}, strings.ToLower(path.Ext(value)))
}

// OpenKnowledgeDirectoryRoot pins the opened directory rather than trusting its name alone.
func OpenKnowledgeDirectoryRoot(absolute string) (*os.Root, string, error) {
	if !KnowledgeDirectoryPlatformVerified() || !filepath.IsAbs(absolute) {
		return nil, "", errors.New("unverified root")
	}
	clean := filepath.Clean(absolute)
	for p := clean; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, "", errors.New("unsafe root")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	before, err := os.Lstat(clean)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(clean)
	if err != nil {
		return nil, "", err
	}
	f, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, "", err
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		root.Close()
		return nil, "", errors.New("root changed")
	}
	identity, _, ok := knowledgeDirectoryIdentity(after)
	if !ok {
		root.Close()
		return nil, "", errors.New("root identity unavailable")
	}
	probe, err := knowledgeDirectoryOpen(root, ".")
	if err != nil {
		root.Close()
		return nil, "", err
	}
	probe.Close()
	if !knowledgeDirectoryStorageVerified(root) {
		root.Close()
		return nil, "", errors.New("unverified filesystem")
	}
	current, err := os.Lstat(clean)
	if err != nil || !os.SameFile(after, current) {
		root.Close()
		return nil, "", errors.New("root changed")
	}
	for p := clean; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			root.Close()
			return nil, "", errors.New("unsafe root")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return root, identity, nil
}

// OpenKnowledgeDirectoryEntry rejects links, aliases and hardlinks before reading any body.
func OpenKnowledgeDirectoryEntry(ctx context.Context, root *os.Root, relative string, directory bool) (*os.File, os.FileInfo, string, error) {
	if !IsKnowledgeDirectoryPath(relative, directory) {
		return nil, nil, "", errors.New("invalid path")
	}
	if relative != "." {
		parts := strings.Split(relative, "/")
		for i, part := range parts {
			if err := ctx.Err(); err != nil {
				return nil, nil, "", err
			}
			parent := "."
			if i > 0 {
				parent = strings.Join(parts[:i], "/")
			}
			folder, err := knowledgeDirectoryOpen(root, parent)
			if err != nil {
				return nil, nil, "", err
			}
			found := false
			for !found {
				entries, e := folder.ReadDir(128)
				for _, entry := range entries {
					if entry.Name() == part {
						found = true
						break
					}
				}
				if e != nil || ctx.Err() != nil {
					break
				}
			}
			folder.Close()
			if !found {
				return nil, nil, "", os.ErrNotExist
			}
			current := strings.Join(parts[:i+1], "/")
			info, err := root.Lstat(current)
			if err != nil {
				return nil, nil, "", err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, nil, "", errors.New("link")
			}
			if i < len(parts)-1 && !info.IsDir() {
				return nil, nil, "", errors.New("not_regular")
			}
		}
	}
	before, err := root.Lstat(relative)
	if err != nil {
		return nil, nil, "", err
	}
	if (directory && !before.IsDir()) || (!directory && !before.Mode().IsRegular()) {
		return nil, nil, "", errors.New("not_regular")
	}
	_, links, ok := knowledgeDirectoryIdentity(before)
	if !ok {
		return nil, nil, "", errors.New("identity_unverified")
	}
	if !directory && links != 1 {
		return nil, nil, "", errors.New("hardlink")
	}
	f, err := knowledgeDirectoryOpen(root, relative)
	if err != nil {
		return nil, nil, "", err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		f.Close()
		return nil, nil, "", errors.New("changed_during_read")
	}
	identity, links, ok := knowledgeDirectoryIdentity(after)
	if !ok || (!directory && links != 1) {
		f.Close()
		return nil, nil, "", errors.New("identity_unverified")
	}
	return f, after, identity, nil
}

// ReadKnowledgeDirectoryFile preserves original UTF-8 bytes and detects changes during capture.
func ReadKnowledgeDirectoryFile(ctx context.Context, root *os.Root, relative string, limit int64) ([]byte, string, error) {
	f, before, identity, err := OpenKnowledgeDirectoryEntry(ctx, root, relative, false)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	if before.Size() > limit {
		return nil, "", errors.New("too_large")
	}
	content := make([]byte, 0, before.Size())
	buffer := make([]byte, 32768)
	for {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		n, e := f.Read(buffer)
		content = append(content, buffer[:n]...)
		if int64(len(content)) > limit {
			return nil, "", errors.New("too_large")
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, "", e
		}
	}
	after, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	other, links, ok := knowledgeDirectoryIdentity(after)
	current, err := root.Lstat(relative)
	if err != nil || !ok || links != 1 || identity != other || !os.SameFile(after, current) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(content)) != after.Size() {
		return nil, "", errors.New("changed_during_read")
	}
	return content, fmt.Sprintf("%s:%d:%d", identity, after.Size(), after.ModTime().UnixNano()), nil
}

// InspectKnowledgeDirectoryEntry reads only metadata through the no-link root capability.
func InspectKnowledgeDirectoryEntry(root *os.Root, relative string) (os.FileInfo, string) {
	f, err := knowledgeDirectoryOpen(root, relative)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "unreadable"
		}
		return nil, "link"
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, "unreadable"
	}
	_, links, ok := knowledgeDirectoryIdentity(info)
	if !ok {
		return nil, "identity_unverified"
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, "not_regular"
	}
	if !info.IsDir() && links != 1 {
		return nil, "hardlink"
	}
	return info, ""
}
