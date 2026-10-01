// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"strings"

	"golang.org/x/sys/unix"
)

// ValidateIsolatedModulePath is the worker-side mount contract, not a host
// ownership validator. Call only after applying the all-thread restrictions
// that forbid mounting/unsharing, and before loading any native module.
// NewSandboxCommand must already have validated the host owners/ancestors and
// pinned this exact file into an individual read-only mount. User namespaces
// make unmapped host owners appear as overflow UIDs; those UIDs are NOT root.
// This check proves neither the original owner nor immutable backing bytes.
// Never replace the parent's ValidateModulePath with this function.
func ValidateIsolatedModulePath(path string) (string, error) {
	if !sandboxValidPath(path) || path == "/" || sandboxReservedDestination(path) {
		return "", ErrSandboxPolicy
	}
	parent, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", ErrSandboxPolicy
	}
	defer func() { _ = unix.Close(parent) }()
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, component := range parts[:len(parts)-1] {
		next, err := unix.Openat(parent, component, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return "", ErrSandboxPolicy
		}
		_ = unix.Close(parent)
		parent = next
	}
	leaf, err := unix.Openat(parent, parts[len(parts)-1], unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", ErrSandboxPolicy
	}
	defer unix.Close(leaf)
	var info unix.Stat_t
	var fs unix.Statfs_t
	var leafMount, parentMount unix.Statx_t
	if unix.Fstat(leaf, &info) != nil || info.Mode&unix.S_IFMT != unix.S_IFREG ||
		info.Mode&06022 != 0 || info.Size <= 0 || info.Size > sandboxMaxFile || info.Nlink != 1 ||
		unix.Fstatfs(leaf, &fs) != nil || fs.Flags&unix.ST_RDONLY == 0 ||
		unix.Statx(leaf, "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &leafMount) != nil ||
		unix.Statx(parent, "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &parentMount) != nil ||
		leafMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mask&unix.STATX_MNT_ID == 0 ||
		leafMount.Mnt_id == parentMount.Mnt_id {
		return "", ErrSandboxPolicy
	}
	return path, nil
}
