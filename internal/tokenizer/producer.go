package tokenizer

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// ProducerSHA256 identifies this executable (including the linked native
// analyzer) and the lexical resources captured when the live pool was loaded.
// It is intentionally coarse: a different executable invalidates old profiles.
func ProducerSHA256() (string, error) {
	p := globalPool.Load()
	if p == nil {
		return "", errors.New("tokenizer not initialized")
	}
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	if !p.initialized || p.producerErr != nil || len(p.producerSHA256) != 64 {
		return "", errors.New("tokenizer producer unavailable")
	}
	return p.producerSHA256, nil
}

func lexicalProducerFingerprint(root string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "linux" {
		exe = "/proc/self/exe"
	}
	return fingerprintLexicalFiles(exe, root)
}

func fingerprintLexicalFiles(exe, root string) (string, error) {
	h := sha256.New()
	write := func(s string) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.Write([]byte(s))
	}
	write("ragflow-native-lexical-producer/1")
	add := func(name, file string) error {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		before, err := f.Stat()
		if err != nil || !before.Mode().IsRegular() {
			return fmt.Errorf("invalid producer file: %s", name)
		}
		d := sha256.New()
		n, err := io.Copy(d, f)
		if err != nil {
			return err
		}
		after, err := f.Stat()
		if err != nil || n != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			return errors.New("producer changed during hashing")
		}
		write(name)
		write(hex.EncodeToString(d.Sum(nil)))
		return nil
	}
	if err := add("executable", exe); err != nil {
		return "", err
	}
	var files []string
	for _, dir := range []string{"rag", "wordnet", "opencc"} {
		count := 0
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.Type()&os.ModeSymlink != 0 {
				return errors.New("symlink in lexical resources")
			}
			if !e.IsDir() {
				rel, err := filepath.Rel(root, p)
				if err != nil {
					return err
				}
				files = append(files, rel)
				count++
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		if count == 0 {
			return "", errors.New("missing lexical resources")
		}
	}
	sort.Strings(files)
	for _, file := range files {
		if err := add(filepath.ToSlash(file), filepath.Join(root, file)); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
