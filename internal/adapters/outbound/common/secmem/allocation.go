// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package secmem

import "errors"

// MaxLockedSize limita una reserva estricta; el PIN actual necesita menos.
const MaxLockedSize = 64 * 1024

var (
	ErrInvalidSize     = errors.New("tamaño de memoria protegida no válido")
	ErrLockUnavailable = errors.New("no se pudo fijar la memoria protegida")
)

type SizeError struct{ Size int }

func (*SizeError) Error() string { return ErrInvalidSize.Error() }
func (*SizeError) Unwrap() error { return ErrInvalidSize }

type LockError struct{ Size int }

func (*LockError) Error() string { return ErrLockUnavailable.Error() }
func (*LockError) Unwrap() error { return ErrLockUnavailable }

// Una región interior alineada ocupa páginas enteras exclusivas del backing.
// mlock/munlock redondean a páginas y sus bloqueos no se apilan: nunca deben
// compartir páginas de otros Blobs/objetos Go.
func alignedSizes(size, page int) (region, backing int, err error) {
	maxInt := int(^uint(0) >> 1)
	if size <= 0 || page <= 0 || size > maxInt-(page-1) {
		return 0, 0, &SizeError{Size: size}
	}
	region = ((size + page - 1) / page) * page
	if region > maxInt-(page-1) {
		return 0, 0, &SizeError{Size: size}
	}
	return region, region + page - 1, nil
}
