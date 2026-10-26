//go:build linux

package parser

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const documentChildOwnerFile = "owner.json"

type documentChildProcess struct {
	PID        int
	PGID       int
	ParentPID  int
	StartTicks uint64
	State      byte
}

func DocumentStructureChildAvailable() bool {
	fd, err := unix.PidfdOpen(os.Getpid(), 0)
	if err != nil {
		return false
	}
	return unix.Close(fd) == nil
}

func documentChildDirectory(task string, attempt int) string {
	return filepath.Join(os.TempDir(), "lfen-document-child-"+task+"-"+strconv.Itoa(attempt))
}

func documentChildBootID() (string, error) {
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil || len(raw) > 64 || strings.TrimSpace(string(raw)) == "" {
		return "", ErrDocumentChildUnresolved
	}
	return strings.TrimSpace(string(raw)), nil
}

func documentChildReadSmall(filename string, limit int64) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrDocumentChildUnresolved
	}
	return raw, nil
}

func documentChildProcessInfo(pid int) (documentChildProcess, error) {
	var info documentChildProcess
	raw, err := documentChildReadSmall("/proc/"+strconv.Itoa(pid)+"/stat", 8192)
	if err != nil {
		return info, err
	}
	end := bytes.LastIndexByte(raw, ')')
	if end < 2 || end+2 > len(raw) {
		return info, ErrDocumentChildUnresolved
	}
	fields := strings.Fields(string(raw[end+2:]))
	if len(fields) < 20 || len(fields[0]) != 1 {
		return info, ErrDocumentChildUnresolved
	}
	parent, parentErr := strconv.Atoi(fields[1])
	group, groupErr := strconv.Atoi(fields[2])
	start, startErr := strconv.ParseUint(fields[19], 10, 64)
	if parentErr != nil || groupErr != nil || startErr != nil || group < 1 || start == 0 {
		return info, ErrDocumentChildUnresolved
	}
	return documentChildProcess{PID: pid, PGID: group, ParentPID: parent, StartTicks: start, State: fields[0][0]}, nil
}

func documentChildExited(state byte) bool { return state == 'Z' || state == 'X' || state == 'x' }

func documentChildToken(pid int) (string, error) {
	raw, err := documentChildReadSmall("/proc/"+strconv.Itoa(pid)+"/environ", 8192)
	if err != nil {
		return "", err
	}
	var found string
	for _, item := range bytes.Split(raw, []byte{0}) {
		if bytes.HasPrefix(item, []byte("LFEN_DOCUMENT_CHILD_TOKEN=")) {
			if found != "" {
				return "", ErrDocumentChildUnresolved
			}
			found = string(bytes.TrimPrefix(item, []byte("LFEN_DOCUMENT_CHILD_TOKEN=")))
		}
	}
	return found, nil
}

func documentChildCommand(pid int) (bool, error) {
	raw, err := documentChildReadSmall("/proc/"+strconv.Itoa(pid)+"/cmdline", 8192)
	if err != nil {
		return false, err
	}
	parts := bytes.Split(bytes.TrimSuffix(raw, []byte{0}), []byte{0})
	return len(parts) == 2 && len(parts[0]) > 0 && string(parts[1]) == DocumentStructureChildFlag, nil
}

func documentChildExe(pid int) (uint64, uint64, error) {
	info, err := os.Stat("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return 0, 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() {
		return 0, 0, ErrDocumentChildUnresolved
	}
	return uint64(stat.Dev), stat.Ino, nil
}

func documentChildBaseValid(owner DocumentChildOwnership) bool {
	return owner.Version == documentChildVersion && documentChildHash(owner.TaskID, 32) && owner.Attempt >= 1 && owner.Attempt <= 3 && documentChildHash(owner.Token, 64) && documentChildHash(owner.InputSHA256, 64) && owner.InputBytes > 0 && owner.InputBytes <= documentChildInputLimit && owner.Directory == documentChildDirectory(owner.TaskID, owner.Attempt) && owner.ParentPID > 0 && owner.ParentStartTicks > 0 && owner.BootID != "" && len(owner.BootID) <= 64 && owner.ExecutableDevice > 0 && owner.ExecutableInode > 0
}

func documentChildOwnerBaseEqual(left, right DocumentChildOwnership) bool {
	left.PID, left.PGID, left.StartTicks = 0, 0, 0
	right.PID, right.PGID, right.StartTicks = 0, 0, 0
	return left == right
}

func documentChildDirectoryInfo(owner DocumentChildOwnership) (os.FileInfo, error) {
	if !documentChildBaseValid(owner) {
		return nil, ErrDocumentChildUnresolved
	}
	info, err := os.Lstat(owner.Directory)
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || stat.Uid != uint32(os.Geteuid()) {
		return nil, ErrDocumentChildUnresolved
	}
	return info, nil
}

func documentChildLoadOwner(directory string) (DocumentChildOwnership, error) {
	var owner DocumentChildOwnership
	info, err := os.Lstat(directory)
	if err != nil {
		return owner, err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 {
		return owner, ErrDocumentChildUnresolved
	}
	fd, err := unix.Open(filepath.Join(directory, documentChildOwnerFile), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return owner, ErrDocumentChildUnresolved
	}
	file := os.NewFile(uintptr(fd), documentChildOwnerFile)
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm() != 0600 || stat.Size() > documentChildControlLimit {
		return owner, ErrDocumentChildUnresolved
	}
	raw, err := io.ReadAll(io.LimitReader(file, documentChildControlLimit+1))
	if err != nil || len(raw) > documentChildControlLimit || documentChildDecode(raw, &owner) != nil || !documentChildBaseValid(owner) || owner.Directory != directory {
		return owner, ErrDocumentChildUnresolved
	}
	if _, err := documentChildDirectoryInfo(owner); err != nil {
		return owner, ErrDocumentChildUnresolved
	}
	return owner, nil
}

func documentChildWriteOwner(owner DocumentChildOwnership) error {
	if _, err := documentChildDirectoryInfo(owner); err != nil {
		return ErrDocumentChildUnresolved
	}
	raw, err := json.Marshal(owner)
	if err != nil || len(raw) > documentChildControlLimit {
		return ErrDocumentChildUnresolved
	}
	filename := filepath.Join(owner.Directory, "owner.next")
	fd, err := unix.Open(filename, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return ErrDocumentChildUnresolved
	}
	file := os.NewFile(uintptr(fd), "owner.next")
	_, writeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || os.Rename(filename, filepath.Join(owner.Directory, documentChildOwnerFile)) != nil {
		return ErrDocumentChildUnresolved
	}
	directory, err := os.Open(owner.Directory)
	if err != nil {
		return ErrDocumentChildUnresolved
	}
	defer directory.Close()
	if directory.Sync() != nil {
		return ErrDocumentChildUnresolved
	}
	return nil
}

func documentChildPrepare(ctx context.Context, input []byte, control DocumentChildControl) (DocumentChildOwnership, *os.File, error) {
	var owner DocumentChildOwnership
	token := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, token); err != nil {
		return owner, nil, documentChildError("native_process_failed", nil)
	}
	boot, err := documentChildBootID()
	parent, parentErr := documentChildProcessInfo(os.Getpid())
	device, inode, exeErr := documentChildExe(os.Getpid())
	if err != nil || parentErr != nil || exeErr != nil {
		return owner, nil, ErrDocumentChildUnavailable
	}
	owner = DocumentChildOwnership{Version: documentChildVersion, TaskID: control.TaskID, Attempt: control.Attempt, Token: hex.EncodeToString(token), BootID: boot, Directory: documentChildDirectory(control.TaskID, control.Attempt), InputSHA256: control.InputSHA256, InputBytes: control.InputBytes, ParentPID: os.Getpid(), ParentStartTicks: parent.StartTicks, ExecutableDevice: device, ExecutableInode: inode}
	if err := os.Mkdir(owner.Directory, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return owner, nil, ErrDocumentChildUnresolved
		}
		return owner, nil, documentChildError("native_process_failed", nil)
	}
	// Publish the durable marker before Start. The deterministic task/attempt
	// directory allows recovery even if the parent dies before onStarted.
	if err := documentChildWriteOwner(owner); err != nil {
		return owner, nil, ErrDocumentChildUnresolved
	}
	filename := filepath.Join(owner.Directory, "input")
	fd, err := unix.Open(filename, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return owner, nil, documentChildError("native_process_failed", nil)
	}
	file := os.NewFile(uintptr(fd), "input")
	hash := sha256.New()
	for offset := 0; offset < len(input); {
		if ctx.Err() != nil {
			_ = file.Close()
			return owner, nil, documentChildError(documentChildFailure(ctx.Err()), ctx.Err())
		}
		end := min(offset+64*1024, len(input))
		part := input[offset:end]
		written, writeErr := file.Write(part)
		if writeErr != nil || written != len(part) {
			_ = file.Close()
			return owner, nil, documentChildError("native_process_failed", nil)
		}
		_, _ = hash.Write(part)
		offset = end
	}
	syncErr, closeErr := file.Sync(), file.Close()
	if hex.EncodeToString(hash.Sum(nil)) != control.InputSHA256 {
		return owner, nil, documentChildError("original_integrity", nil)
	}
	if syncErr != nil || closeErr != nil {
		return owner, nil, documentChildError("native_process_failed", nil)
	}
	fd, err = unix.Open(filename, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return owner, nil, documentChildError("native_process_failed", nil)
	}
	return owner, os.NewFile(uintptr(fd), "input"), nil
}

func documentChildCheckProcess(owner DocumentChildOwnership) (documentChildProcess, error) {
	info, _, err := documentChildInspectProcess(owner)
	return info, err
}

// Inspect preserves the original ownership checks and error results, while
// identifying the exact failed proof with fixed labels for private tests.
func documentChildInspectProcess(owner DocumentChildOwnership) (documentChildProcess, string, error) {
	info, err := documentChildProcessInfo(owner.PID)
	if err != nil {
		return info, "stat_unavailable", err
	}
	if info.StartTicks != owner.StartTicks || owner.PID <= 1 || owner.PGID != owner.PID || info.PGID != owner.PGID {
		reason := "pgid_mismatch"
		if info.StartTicks != owner.StartTicks {
			reason = "start_ticks_mismatch"
		}
		return info, reason, ErrDocumentChildUnresolved
	}
	if documentChildExited(info.State) {
		return info, "already_exited", nil
	}
	token, tokenErr := documentChildToken(owner.PID)
	command, commandErr := documentChildCommand(owner.PID)
	device, inode, exeErr := documentChildExe(owner.PID)
	if tokenErr != nil || commandErr != nil || exeErr != nil || token != owner.Token || !command || device != owner.ExecutableDevice || inode != owner.ExecutableInode {
		reason := "exe_mismatch"
		switch {
		case tokenErr != nil:
			reason = "token_unavailable"
		case commandErr != nil:
			reason = "command_unavailable"
		case exeErr != nil:
			reason = "exe_unavailable"
		case token != owner.Token:
			reason = "token_mismatch"
		case !command:
			reason = "command_mismatch"
		}
		return info, reason, ErrDocumentChildUnresolved
	}
	return info, "matched", nil
}

// Start may return when the close-on-exec status pipe closes, before /proc has
// exposed the complete new argument/environment image. Startup waits for the
// same complete ownership proof; it never authorizes partial or wrong proof.
// Recovery/stop checks deliberately keep their existing strict single proof.
func documentChildAuthorizeStart(ctx context.Context, owner DocumentChildOwnership, ready func() error) (string, error) {
	settle, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	probe := "not_checked"
	for {
		if ctx.Err() != nil {
			return probe, ctx.Err()
		}
		info, observed, err := documentChildInspectProcess(owner)
		probe = observed
		if err == nil && !documentChildExited(info.State) {
			if settle.Err() != nil {
				if ctx.Err() != nil {
					return probe, ctx.Err()
				}
				return probe, documentChildError("native_process_failed", nil)
			}
			return probe, ready()
		}
		// A different start identity, a missing PID or confirmed exit cannot
		// become the same owned child by waiting. No callback is authorized.
		if errors.Is(err, os.ErrNotExist) || info.PID != 0 && (info.StartTicks != owner.StartTicks || documentChildExited(info.State)) {
			return probe, documentChildError("native_process_failed", nil)
		}
		select {
		case <-settle.Done():
			if ctx.Err() != nil {
				return probe, ctx.Err()
			}
			return probe, documentChildError("native_process_failed", nil)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func documentChildGroupExited(owner DocumentChildOwnership) (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, ErrDocumentChildUnresolved
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 {
			continue
		}
		info, err := documentChildProcessInfo(pid)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, ErrDocumentChildUnresolved
		}
		if info.PGID == owner.PGID && !documentChildExited(info.State) {
			return false, nil
		}
	}
	return true, nil
}

// Stop first anchors the leader with pidfd and SIGSTOP. A stopped, verified
// leader reserves its PGID, so a group SIGKILL cannot race PGID recycling.
func documentChildStop(ctx context.Context, owner DocumentChildOwnership) error {
	info, err := documentChildCheckProcess(owner)
	if errors.Is(err, os.ErrNotExist) || err == nil && documentChildExited(info.State) {
		exited, checkErr := documentChildGroupExited(owner)
		if checkErr == nil && exited {
			return nil
		}
		return ErrDocumentChildUnresolved
	}
	if err != nil {
		return ErrDocumentChildUnresolved
	}
	fd, err := unix.PidfdOpen(owner.PID, 0)
	if err != nil {
		return ErrDocumentChildUnresolved
	}
	defer unix.Close(fd)
	if _, err := documentChildCheckProcess(owner); err != nil {
		return ErrDocumentChildUnresolved
	}
	if err := unix.PidfdSendSignal(fd, unix.SIGSTOP, nil, 0); err != nil {
		if !errors.Is(err, unix.ESRCH) {
			return ErrDocumentChildUnresolved
		}
	} else {
		for {
			current, checkErr := documentChildCheckProcess(owner)
			if errors.Is(checkErr, os.ErrNotExist) || checkErr == nil && documentChildExited(current.State) {
				break
			}
			if checkErr != nil {
				return ErrDocumentChildUnresolved
			}
			if current.State == 'T' || current.State == 't' {
				if err := unix.Kill(-owner.PGID, unix.SIGKILL); err != nil && !errors.Is(err, unix.ESRCH) {
					return ErrDocumentChildUnresolved
				}
				break
			}
			select {
			case <-ctx.Done():
				return ErrDocumentChildUnresolved
			case <-time.After(5 * time.Millisecond):
			}
		}
	}
	for {
		exited, err := documentChildGroupExited(owner)
		if err != nil {
			return ErrDocumentChildUnresolved
		}
		if exited {
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrDocumentChildUnresolved
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func documentChildCleanup(owner DocumentChildOwnership) error {
	if _, err := documentChildDirectoryInfo(owner); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return ErrDocumentChildUnresolved
	}
	stored, err := documentChildLoadOwner(owner.Directory)
	if err != nil || !documentChildOwnerBaseEqual(owner, stored) {
		return ErrDocumentChildUnresolved
	}
	entries, err := os.ReadDir(owner.Directory)
	if err != nil {
		return ErrDocumentChildUnresolved
	}
	for _, entry := range entries {
		if entry.Name() != "input" && entry.Name() != documentChildOwnerFile && entry.Name() != "owner.next" {
			return ErrDocumentChildUnresolved
		}
		info, err := os.Lstat(filepath.Join(owner.Directory, entry.Name()))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return ErrDocumentChildUnresolved
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
			return ErrDocumentChildUnresolved
		}
	}
	// Only known regular private files are removed, never recursive traversal.
	for _, name := range []string{"input", "owner.next", documentChildOwnerFile} {
		if err := os.Remove(filepath.Join(owner.Directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return ErrDocumentChildUnresolved
		}
	}
	if err := os.Remove(owner.Directory); err != nil {
		return ErrDocumentChildUnresolved
	}
	return nil
}

func RunDocumentStructureChild(ctx context.Context, input []byte, control DocumentChildControl, onStarted func(DocumentChildOwnership) error) (result json.RawMessage, resultErr error) {
	stage, probe, cleanupStage := "platform_ready", "", ""
	var cmd *exec.Cmd
	var stdout *documentChildBuffer
	var callbackCalled atomic.Bool
	var waitFinished atomic.Bool
	defer func() {
		var failure *DocumentChildError
		if errors.As(resultErr, &failure) {
			failure.stage, failure.probe, failure.cleanupStage = stage, probe, cleanupStage
			failure.callbackCalled, failure.exitCode = callbackCalled.Load(), -2
			if stdout != nil {
				failure.stdoutBytes = stdout.length()
			}
			if cmd != nil && waitFinished.Load() && cmd.ProcessState != nil {
				failure.exitCode = cmd.ProcessState.ExitCode()
				if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
					failure.exitSignal = int(status.Signal())
				}
			}
		}
	}()
	if !DocumentStructureChildAvailable() {
		return nil, ErrDocumentChildUnavailable
	}
	stage = "validate_input"
	if len(input) > documentChildInputLimit || control.InputBytes > documentChildInputLimit {
		return nil, documentChildError("budget_exceeded", nil)
	}
	if int64(len(input)) != control.InputBytes {
		return nil, documentChildError("original_integrity", nil)
	}
	if !documentChildControlValid(control, false) || onStarted == nil {
		return nil, documentChildError("document_integrity", nil)
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if work.Err() != nil {
		return nil, documentChildError(documentChildFailure(work.Err()), work.Err())
	}
	stage = "prepare_input"
	owner, file, err := documentChildPrepare(work, input, control)
	if err != nil {
		if documentChildBaseValid(owner) {
			if errors.Is(err, ErrDocumentChildUnresolved) {
				return nil, err
			}
			if cleanupErr := documentChildCleanup(owner); cleanupErr != nil {
				return nil, documentChildError("cleanup_failed", ErrDocumentChildUnresolved)
			}
		}
		return nil, err
	}
	defer file.Close()
	// Pdeathsig is tied to the thread which creates the process. Install this
	// defer before the stop/wait/cleanup defer so the thread stays pinned through
	// the entire ownership cleanup path.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	exitKnown := true // No child exists before cmd.Start succeeds.
	var wait chan error
	var waited bool
	var waitErr error
	defer func() {
		if !exitKnown {
			stop, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = documentChildStop(stop, owner)
			stopCancel()
			if !waited {
				select {
				case waitErr = <-wait:
					waited = true
				case <-time.After(2 * time.Second):
				}
			}
			if waited && cmd.ProcessState != nil {
				exited, groupErr := documentChildGroupExited(owner)
				if groupErr == nil && exited {
					exitKnown = true
				}
			}
		}
		if !exitKnown {
			cleanupStage = "exit_unproven"
			result = nil
			resultErr = documentChildError("native_process_failed", ErrDocumentChildUnresolved)
			return
		}
		if cleanupErr := documentChildCleanup(owner); cleanupErr != nil {
			cleanupStage = "cleanup_unproven"
			result = nil
			resultErr = documentChildError("cleanup_failed", ErrDocumentChildUnresolved)
		} else {
			cleanupStage = "cleaned"
		}
	}()
	// /proc/self/exe is this exact running executable, even if deployment has
	// replaced the pathname returned by os.Executable since this parent started.
	cmd = exec.Command("/proc/self/exe", DocumentStructureChildFlag)
	cmd.Dir = owner.Directory
	cmd.Env = []string{"LANG=C.UTF-8", "LC_ALL=C.UTF-8", "TZ=UTC", "HOME=/nonexistent", "PATH=/nonexistent", "TMPDIR=" + owner.Directory, "LFEN_DOCUMENT_CHILD_TOKEN=" + owner.Token}
	cmd.ExtraFiles = []*os.File{file}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.WaitDelay = 2 * time.Second
	stdout = newDocumentChildBuffer(documentChildOutputLimit, false)
	stderr := newDocumentChildBuffer(documentChildStderrLimit, true)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	stage = "pipe_setup"
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, documentChildError("native_process_failed", nil)
	}
	defer stdin.Close()
	stage = "process_start"
	if err := cmd.Start(); err != nil {
		return nil, documentChildError("native_process_failed", nil)
	}
	exitKnown = false
	wait = make(chan error, 1)
	go func() {
		err := cmd.Wait()
		waitFinished.Store(true)
		wait <- err
	}()
	owner.PID, owner.PGID = cmd.Process.Pid, cmd.Process.Pid
	stage = "started_stat"
	info, err := documentChildProcessInfo(owner.PID)
	if err != nil {
		// The child is our direct unreused handle even before a complete marker.
		_ = cmd.Process.Kill()
		select {
		case waitErr = <-wait:
			waited, exitKnown = true, true
		case <-time.After(2 * time.Second):
		}
		return nil, documentChildError("native_process_failed", nil)
	}
	owner.StartTicks = info.StartTicks
	stage = "owned_check"
	probe, err = documentChildAuthorizeStart(work, owner, func() error {
		stage = "owner_write"
		return documentChildWriteOwner(owner)
	})
	if err != nil {
		if work.Err() != nil {
			return nil, documentChildError(documentChildFailure(work.Err()), work.Err())
		}
		return nil, documentChildError("native_process_failed", nil)
	}
	if work.Err() != nil {
		return nil, documentChildError(documentChildFailure(work.Err()), work.Err())
	}
	stage = "persist_owner"
	started := make(chan error, 1)
	go func() {
		callbackCalled.Store(true)
		var callbackErr error
		defer func() {
			if recover() != nil {
				callbackErr = documentChildError("native_process_failed", nil)
			}
			started <- callbackErr
		}()
		callbackErr = onStarted(owner)
	}()
	select {
	case err := <-started:
		if err != nil {
			return nil, documentChildError("native_process_failed", nil)
		}
	case <-work.Done():
		return nil, documentChildError(documentChildFailure(work.Err()), work.Err())
	case waitErr = <-wait:
		waited = true
		return nil, documentChildError("native_process_failed", nil)
	}
	control.Version, control.Token, control.ParentPID = documentChildVersion, owner.Token, os.Getpid()
	stage = "control_pack"
	raw, err := json.Marshal(control)
	if err != nil || len(raw) > documentChildControlLimit {
		return nil, documentChildError("budget_exceeded", nil)
	}
	stage = "control_write"
	written := make(chan error, 1)
	go func() {
		_, err := stdin.Write(raw)
		closeErr := stdin.Close()
		if err == nil {
			err = closeErr
		}
		written <- err
	}()
	select {
	case err := <-written:
		if err != nil {
			return nil, documentChildError("native_process_failed", nil)
		}
	case <-work.Done():
		return nil, documentChildError(documentChildFailure(work.Err()), work.Err())
	}
	stage = "wait_exit"
	select {
	case waitErr = <-wait:
		waited = true
	case <-stdout.overflow:
		return nil, documentChildError("budget_exceeded", nil)
	case <-work.Done():
		return nil, documentChildError(documentChildFailure(work.Err()), work.Err())
	}
	stage = "wait_status"
	if stdout.isExceeded() {
		return nil, documentChildError("budget_exceeded", nil)
	}
	if work.Err() != nil {
		return nil, documentChildError(documentChildFailure(work.Err()), work.Err())
	}
	if waitErr != nil || cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		return nil, documentChildError("native_process_failed", nil)
	}
	stage = "group_exit"
	exited, checkErr := documentChildGroupExited(owner)
	if checkErr != nil || !exited {
		return nil, documentChildError("native_process_failed", ErrDocumentChildUnresolved)
	}
	exitKnown = true
	stage = "reply_decode"
	return documentChildReadReply(stdout.bytes(), control)
}

func RecoverDocumentStructureChild(ctx context.Context, owner DocumentChildOwnership) error {
	if !documentChildBaseValid(owner) || owner.PID <= 1 || owner.PGID != owner.PID || owner.StartTicks == 0 {
		return ErrDocumentChildUnresolved
	}
	stored, err := documentChildLoadOwner(owner.Directory)
	if errors.Is(err, os.ErrNotExist) {
		_, processErr := documentChildProcessInfo(owner.PID)
		if errors.Is(processErr, os.ErrNotExist) {
			exited, groupErr := documentChildGroupExited(owner)
			if groupErr == nil && exited {
				return nil
			}
		}
		return ErrDocumentChildUnresolved
	}
	if err != nil || stored != owner {
		return ErrDocumentChildUnresolved
	}
	boot, err := documentChildBootID()
	if err != nil {
		return ErrDocumentChildUnresolved
	}
	if boot == owner.BootID {
		stop, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := documentChildStop(stop, owner)
		cancel()
		if err != nil {
			return ErrDocumentChildUnresolved
		}
	}
	if err := documentChildCleanup(owner); err != nil {
		return documentChildError("cleanup_failed", ErrDocumentChildUnresolved)
	}
	return nil
}

// RecoverDocumentStructureAttempt closes the Start -> durable onStarted window.
// An incomplete marker never permits a second execution or blind PID signaling.
func RecoverDocumentStructureAttempt(ctx context.Context, task string, attempt int) error {
	if !documentChildHash(task, 32) || attempt < 1 || attempt > 3 {
		return ErrDocumentChildUnresolved
	}
	owner, err := documentChildLoadOwner(documentChildDirectory(task, attempt))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return ErrDocumentChildUnresolved
	}
	if owner.PID != 0 {
		return RecoverDocumentStructureChild(ctx, owner)
	}
	boot, err := documentChildBootID()
	if err != nil {
		return ErrDocumentChildUnresolved
	}
	if boot != owner.BootID {
		return documentChildCleanup(owner)
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return ErrDocumentChildUnresolved
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 {
			continue
		}
		command, err := documentChildCommand(pid)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return ErrDocumentChildUnresolved
		}
		if !command {
			continue
		}
		token, err := documentChildToken(pid)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return ErrDocumentChildUnresolved
		}
		if token != owner.Token {
			continue
		}
		info, err := documentChildProcessInfo(pid)
		if err != nil || info.PGID != pid {
			return ErrDocumentChildUnresolved
		}
		owner.PID, owner.PGID, owner.StartTicks = pid, pid, info.StartTicks
		if _, err := documentChildCheckProcess(owner); err != nil || documentChildWriteOwner(owner) != nil {
			return ErrDocumentChildUnresolved
		}
		return RecoverDocumentStructureChild(ctx, owner)
	}
	parent, err := documentChildProcessInfo(owner.ParentPID)
	if err == nil && parent.StartTicks == owner.ParentStartTicks && !documentChildExited(parent.State) {
		return ErrDocumentChildUnresolved
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrDocumentChildUnresolved
	}
	return documentChildCleanup(owner)
}

func documentChildMainInput(control DocumentChildControl) ([]byte, error) {
	fd := 3
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY {
		return nil, documentChildError("original_integrity", nil)
	}
	file := os.NewFile(uintptr(fd), "fixed-original")
	if file == nil {
		return nil, documentChildError("original_unavailable", nil)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm() != 0600 || stat.Size() != control.InputBytes {
		return nil, documentChildError("original_integrity", nil)
	}
	input, err := io.ReadAll(io.LimitReader(file, documentChildInputLimit+1))
	if err != nil || int64(len(input)) != control.InputBytes || len(input) > documentChildInputLimit {
		return nil, documentChildError("original_integrity", nil)
	}
	hash := sha256.Sum256(input)
	if hex.EncodeToString(hash[:]) != control.InputSHA256 {
		return nil, documentChildError("original_integrity", nil)
	}
	return input, nil
}

func documentChildEnvironmentValid(control DocumentChildControl) bool {
	directory, err := os.Getwd()
	if err != nil || filepath.Base(directory) != "lfen-document-child-"+control.TaskID+"-"+strconv.Itoa(control.Attempt) {
		return false
	}
	expected := map[string]string{"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8", "TZ": "UTC", "HOME": "/nonexistent", "PATH": "/nonexistent", "TMPDIR": directory, "LFEN_DOCUMENT_CHILD_TOKEN": control.Token}
	seen := map[string]bool{}
	for _, item := range os.Environ() {
		name, value, ok := strings.Cut(item, "=")
		want, allowed := expected[name]
		if !ok || !allowed || seen[name] || value != want {
			return false
		}
		seen[name] = true
	}
	return len(seen) == len(expected)
}

// DocumentStructureChildMain runs before server argument parsing/bootstrap. Its
// callback may use only local parsers. It must verify the actual profile/producer
// against control rather than substituting a synthetic example producer hash.
func DocumentStructureChildMain(ctx context.Context, parse DocumentChildParser) (exitCode int) {
	exitCode = 70
	if !IsDocumentStructureChild(os.Args[1:]) || parse == nil {
		return 64
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, documentChildControlLimit+1))
	var control DocumentChildControl
	if err != nil || len(raw) > documentChildControlLimit || documentChildDecode(raw, &control) != nil || !documentChildControlValid(control, true) || !documentChildEnvironmentValid(control) || os.Getppid() != control.ParentPID {
		return 65
	}
	// Preserve the private protocol descriptor, then silence fd1 during native
	// parsing. Native/Go diagnostic writes cannot become a second protocol frame.
	outputFD, err := unix.FcntlInt(os.Stdout.Fd(), unix.F_DUPFD_CLOEXEC, 4)
	if err != nil {
		return 70
	}
	output := os.NewFile(uintptr(outputFD), "document-child-pack")
	defer output.Close()
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return 70
	}
	if err := unix.Dup2(int(null.Fd()), 1); err != nil {
		_ = null.Close()
		return 70
	}
	_ = null.Close()
	reply := documentChildReply{Version: documentChildVersion, TaskID: control.TaskID, Attempt: control.Attempt, Token: control.Token, ProducerSHA256: control.ProducerSHA256, InputSHA256: control.InputSHA256, InputBytes: control.InputBytes, Result: json.RawMessage("null")}
	writeReply := func(payload json.RawMessage, parseErr error) int {
		if parseErr != nil {
			code := documentChildFailure(parseErr)
			reply.FailureCode, reply.Result = &code, json.RawMessage("null")
		} else {
			trimmed := bytes.TrimSpace(payload)
			if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
				code := "native_process_failed"
				reply.FailureCode, reply.Result = &code, json.RawMessage("null")
			} else {
				reply.Result = payload
			}
		}
		pack, err := json.Marshal(reply)
		if err != nil || len(pack) > documentChildOutputLimit {
			code := "budget_exceeded"
			reply.FailureCode, reply.Result = &code, json.RawMessage("null")
			pack, err = json.Marshal(reply)
		}
		if err != nil {
			return 70
		}
		written, err := output.Write(pack)
		if err != nil || written != len(pack) {
			return 74
		}
		return 0
	}
	defer func() {
		if recover() != nil {
			exitCode = writeReply(nil, documentChildError("native_process_failed", nil))
		}
	}()
	input, err := documentChildMainInput(control)
	if err != nil {
		return writeReply(nil, err)
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	payload, err := parse(work, control, input)
	if work.Err() != nil {
		err = work.Err()
	}
	return writeReply(payload, err)
}
