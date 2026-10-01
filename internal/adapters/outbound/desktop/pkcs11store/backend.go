// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11store

// Solo nativeModule traduce estos tipos a llamadas del driver. La lógica de
// identidad y sesión se comprueba con dobles sin CGo ni hardware.
type sessionHandle uint
type objectHandle uint
type mechanism uint

const (
	rsaPKCS mechanism = iota + 1
	ecdsaRaw
)

type objectQuery struct {
	private bool
	id      []byte
	signing bool
}
type tokenInfo struct {
	label, manufacturer, model, serial     string
	initialized                            bool
	loginRequired, protectedAuthentication bool
	minPIN, maxPIN                         uint
}
type module interface {
	Initialize() error
	Finalize() error
	Destroy()
	Slots() ([]uint, error)
	TokenInfo(uint) (tokenInfo, error)
	OpenSession(uint) (sessionHandle, error)
	CloseSession(sessionHandle) error
	FindInit(sessionHandle, objectQuery) error
	Find(sessionHandle, int) ([]objectHandle, error)
	FindFinal(sessionHandle) error
	Certificate(sessionHandle, objectHandle) (der, id []byte, err error)
	AlwaysAuthenticate(sessionHandle, objectHandle) (bool, error)
	Mechanisms(uint) ([]mechanism, error)
	Login(sessionHandle, []byte, bool) error
	Logout(sessionHandle) error
	SignInit(sessionHandle, mechanism, objectHandle) error
	Sign(sessionHandle, []byte) ([]byte, error)
}

const (
	maxSlots            = 128
	maxObjects          = 4096
	maxCertificateBytes = 256 << 10
	maxObjectIDBytes    = 1024
	maxPINBytes         = 1024
)
