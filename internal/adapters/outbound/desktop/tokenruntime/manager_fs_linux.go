// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && pkcs11_preview && !production

package tokenruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/ports"
)

type managerOperations struct {
	fstat        func(int, *unix.Stat_t) error
	syncFile     func(*os.File) error
	syncDir      func(int) error
	syncParent   func(int) error
	rename       func(int, string, string) error
	beforeCommit func()
	executable   func() (string, error)
	checkPath    func(string) (string, error)
	allocate     func(int) (*secmem.Blob, error)
}

type settingsDirectory struct {
	fd       int
	writable bool
	missing  bool
}

func (d settingsDirectory) close() { _ = unix.Close(d.fd) }

func (m *Manager) stat(fd int, stat *unix.Stat_t) error {
	if m.ops.fstat != nil {
		return m.ops.fstat(fd, stat)
	}
	return unix.Fstat(fd, stat)
}

func controlledSettingsDirectory(stat unix.Stat_t, user uint32) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && (stat.Uid == user || stat.Uid == 0) &&
		(stat.Mode&0022 == 0 || stat.Uid == 0 && stat.Mode&unix.S_ISVTX != 0)
}

func writableSettingsDirectory(stat unix.Stat_t, user uint32) bool {
	return user != 0 && stat.Uid == user && stat.Mode&0777 == 0700
}

// Walk using directory descriptors and no-follow on EVERY component. The
// final private directory may be created on an explicit Save, never on Load.
func (m *Manager) openDirectory(create bool) (settingsDirectory, error) {
	bad := settingsDirectory{fd: -1}
	path := filepath.Clean(m.configDir)
	if !filepath.IsAbs(m.configDir) || len(m.configDir) > 4096 || strings.ContainsRune(m.configDir, 0) || path == "/" {
		return bad, ports.ErrTokenSettingsUnsafe
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return bad, ports.ErrTokenSettingsUnsafe
	}
	user := uint32(os.Geteuid())
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		var parent unix.Stat_t
		if m.stat(fd, &parent) != nil || !controlledSettingsDirectory(parent, user) {
			_ = unix.Close(fd)
			return bad, ports.ErrTokenSettingsUnsafe
		}
		last := i == len(parts)-1
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if errors.Is(err, unix.ENOENT) && last {
			writable := writableSettingsDirectory(parent, user)
			if !create {
				return settingsDirectory{fd: fd, missing: true, writable: writable}, nil
			}
			mkdirErr := error(nil)
			if writable {
				mkdirErr = unix.Mkdirat(fd, part, 0700)
			}
			if !writable || (mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST)) {
				_ = unix.Close(fd)
				return bad, ports.ErrTokenSettingsUnsafe
			}
			// Persist the new directory entry before creating its configuration.
			// Also sync after EEXIST from a concurrent creator, whose directory
			// may not yet have reached stable storage. Failure leaves at most an
			// empty directory, never a falsely reported saved configuration.
			syncParent := m.ops.syncParent
			if syncParent == nil {
				syncParent = unix.Fsync
			}
			if err := syncParent(fd); err != nil {
				_ = unix.Close(fd)
				return bad, ports.ErrTokenSettingsWrite
			}
			next, err = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		_ = unix.Close(fd)
		if err != nil {
			return bad, ports.ErrTokenSettingsUnsafe
		}
		fd = next
	}
	var stat unix.Stat_t
	if m.stat(fd, &stat) != nil || !controlledSettingsDirectory(stat, user) {
		_ = unix.Close(fd)
		return bad, ports.ErrTokenSettingsUnsafe
	}
	return settingsDirectory{fd: fd, writable: writableSettingsDirectory(stat, user)}, nil
}

func (m *Manager) readSnapshot(dir int, directoryWritable bool) (ports.TokenSettingsSnapshot, error) {
	snapshot := m.initialSnapshot()
	fd, err := unix.Openat(dir, "tokens.json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return m.missingSnapshot(directoryWritable), nil
	}
	snapshot.Exists = true
	if err != nil {
		return snapshot, ports.ErrTokenSettingsUnsafe
	}
	f := os.NewFile(uintptr(fd), "local-token-settings")
	defer f.Close()
	var stat unix.Stat_t
	user := uint32(os.Geteuid())
	if m.stat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Size < 0 || stat.Size > maxConfigBytes ||
		(stat.Uid != user && stat.Uid != 0) || stat.Mode&0022 != 0 {
		return snapshot, ports.ErrTokenSettingsUnsafe
	}
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil || len(data) > maxConfigBytes {
		return snapshot, ports.ErrTokenSettingsUnsafe
	}
	editable := directoryWritable && user != 0 && stat.Uid == user
	return m.snapshotFromData(data, editable), nil
}

func lockSettingsDirectory(ctx context.Context, fd int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Advisory serialization among managers, with no extra lockfile. An
	// external editor that ignores flock is not a transactional participant.
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return ports.ErrTokenSettingsConflict
		}
		return ports.ErrTokenSettingsUnsafe
	}
	return nil
}

func unlockSettingsDirectory(fd int) { _ = unix.Flock(fd, unix.LOCK_UN) }

func (m *Manager) writeSnapshot(ctx context.Context, dir int, data []byte, revision string) (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", ports.ErrTokenSettingsWrite
	}
	name := ".tokens.json.tmp-" + hex.EncodeToString(nonce[:])
	fd, err := unix.Openat(dir, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return "", fixedSettingsIOError(err)
	}
	f := os.NewFile(uintptr(fd), "local-token-settings-temporary")
	defer f.Close()
	defer unix.Unlinkat(dir, name, 0)
	if err := f.Chmod(0600); err != nil {
		return "", fixedSettingsIOError(err)
	}
	if _, err := f.Write(data); err != nil {
		return "", fixedSettingsIOError(err)
	}
	syncFile := m.ops.syncFile
	if syncFile == nil {
		syncFile = func(f *os.File) error { return f.Sync() }
	}
	if err := syncFile(f); err != nil {
		return "", fixedSettingsIOError(err)
	}
	if err := f.Close(); err != nil {
		return "", fixedSettingsIOError(err)
	}
	if m.ops.beforeCommit != nil {
		m.ops.beforeCommit()
	}
	// Recheck both the directory capability and leaf immediately before the
	// atomic replacement; a stale revision never knowingly overwrites edits.
	var stat unix.Stat_t
	if m.stat(dir, &stat) != nil || !writableSettingsDirectory(stat, uint32(os.Geteuid())) {
		return "", ports.ErrTokenSettingsUnsafe
	}
	pathDir, err := m.openDirectory(false)
	if err != nil {
		return "", ports.ErrTokenSettingsUnsafe
	}
	var pathStat unix.Stat_t
	pathChanged := pathDir.missing || m.stat(pathDir.fd, &pathStat) != nil || pathStat.Dev != stat.Dev || pathStat.Ino != stat.Ino
	pathDir.close()
	if pathChanged {
		return "", ports.ErrTokenSettingsConflict
	}
	current, err := m.readSnapshot(dir, true)
	if err != nil || !current.Editable {
		return "", ports.ErrTokenSettingsUnsafe
	}
	if current.Revision != revision {
		return "", ports.ErrTokenSettingsConflict
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	rename := m.ops.rename
	if rename == nil {
		rename = func(fd int, from, to string) error { return unix.Renameat(fd, from, fd, to) }
	}
	if err := rename(dir, name, "tokens.json"); err != nil {
		return "", fixedSettingsIOError(err)
	}
	syncDir := m.ops.syncDir
	if syncDir == nil {
		syncDir = unix.Fsync
	}
	if err := syncDir(dir); err != nil {
		return "durability_unconfirmed", nil
	}
	return "", nil
}
