// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile

// ownerClass clasifica el propietario de la carpeta de ajustes o de una de sus
// carpetas superiores respecto del usuario del proceso.
type ownerClass int

const (
	// ownerCurrent: es el usuario del proceso.
	ownerCurrent ownerClass = iota
	// ownerNotPerson: un grupo o una cuenta del sistema (Administradores,
	// SYSTEM, TrustedInstaller). No dice de qué persona son los ajustes.
	ownerNotPerson
	// ownerOtherUser: la cuenta de otra persona.
	ownerOtherUser
	// ownerUnknown: parece una cuenta, pero Windows no sabe decir de qué tipo.
	ownerUnknown
)

// settingsOwnerDecision decide si el proceso puede escribir los ajustes como
// suyos. owners va de la carpeta de ajustes hacia la raíz y termina en la
// primera carpeta cuyo propietario no se pudo leer. Manda el primer
// propietario que sea una persona: si es el usuario del proceso, adelante; si
// es otra cuenta, o no se sabe, no se escribe, porque dar la carpeta al
// usuario del proceso dejaría fuera a su dueño. Si solo hay grupos y cuentas
// del sistema (la carpeta la creó un proceso elevado), los ajustes son del
// usuario del proceso.
func settingsOwnerDecision(owners []ownerClass) error {
	for _, owner := range owners {
		switch owner {
		case ownerCurrent:
			return nil
		case ownerOtherUser, ownerUnknown:
			return &Error{Key: KeyOtherUser}
		}
	}
	return nil
}
