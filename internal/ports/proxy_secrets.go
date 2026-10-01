// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import "context"

// ProxySecretDescriptor describe una entrada de secreto de proxy sin exponer el
// material sensible en contratos de preferencias ni en adaptadores de entrada.
type ProxySecretDescriptor struct {
	ID       string
	Realm    string
	Username string
}

// ProxySecretMaterial representa el material sensible ya resuelto desde un
// almacen seguro del sistema operativo. No debe persistirse en claro.
type ProxySecretMaterial struct {
	Realm    string
	Username string
	Password []byte
}

// ProxySecretStoreStatus resume el estado observable del backend seguro del SO
// sin exponer material sensible ni forzar todavía su consumo en red.
type ProxySecretStoreStatus struct {
	Available bool
	Platform  string
	Backend   string
	Reason    string
}

// ProxySecretStore abstrae el backend seguro del sistema operativo para
// credenciales de proxy. Su implementacion futura debera apoyarse en:
// - Linux: Secret Service / libsecret
// - macOS: Keychain
// - Windows: Credential Manager / DPAPI
//
// Este puerto existe para desacoplar el nucleo y la configuracion tipada del
// detalle del almacen seguro. Mientras no haya implementacion real, las
// preferencias no deben exponer proxyPassword.
type ProxySecretStore interface {
	Store(ctx context.Context, realm string, material ProxySecretMaterial) (ProxySecretDescriptor, error)
	Load(ctx context.Context, id string) (ProxySecretMaterial, error)
	Delete(ctx context.Context, id string) error
	Status(ctx context.Context) (ProxySecretStoreStatus, error)
}
