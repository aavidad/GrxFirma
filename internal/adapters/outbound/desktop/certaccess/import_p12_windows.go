// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package certaccess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	maxP12Bytes             = 16 << 20
	certKeyProvInfoPropID   = 2
	certStoreProvSystem     = 10
	certStoreOpenExisting   = 0x00004000
	certStoreAddReplaceKeep = windows.CERT_STORE_ADD_REPLACE_EXISTING
)

var procCertGetCertificateContextProperty = windows.NewLazySystemDLL("crypt32.dll").NewProc("CertGetCertificateContextProperty")

// importarP12AWindows importa el PKCS#12 en el almacén "Personal" del
// usuario con la API nativa (PFXImportCertStore). Las claves quedan
// persistidas y NO exportables, como con el antiguo script. Antes se lanzaba
// powershell.exe, que muchas Administraciones bloquean y que los EDR vigilan.
func importarP12AWindows(ctx context.Context, rutaP12, password string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(rutaP12)
	if err != nil {
		return fmt.Errorf("leer P12/PFX: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxP12Bytes {
		return errors.New("el fichero P12/PFX no es válido o supera el tamaño máximo")
	}
	contenido, err := os.ReadFile(rutaP12) // #nosec G304 -- ruta elegida por el usuario para importar su propio P12.
	if err != nil {
		return fmt.Errorf("leer P12/PFX: %w", err)
	}
	defer clear(contenido)

	clave, err := windows.UTF16FromString(password)
	if err != nil {
		return errors.New("la contraseña del P12/PFX contiene caracteres no válidos")
	}
	defer clear(clave)

	if len(contenido) == 0 || len(contenido) > maxP12Bytes {
		return errors.New("el fichero P12/PFX no es válido o supera el tamaño máximo")
	}
	blob := windows.CryptDataBlob{Size: uint32(len(contenido)), Data: &contenido[0]} // #nosec G115 -- acotado por maxP12Bytes.
	temporal, err := windows.PFXImportCertStore(&blob, &clave[0], windows.CRYPT_USER_KEYSET)
	if err != nil {
		return fmt.Errorf("PFXImportCertStore: %w", err)
	}
	defer windows.CertCloseStore(temporal, 0)

	nombre, err := windows.UTF16PtrFromString("MY")
	if err != nil {
		return err
	}
	personal, err := windows.CertOpenStore(
		certStoreProvSystem,
		0,
		0,
		windows.CERT_SYSTEM_STORE_CURRENT_USER|certStoreOpenExisting,
		uintptr(unsafe.Pointer(nombre)),
	)
	if err != nil {
		return fmt.Errorf("abrir el almacén personal: %w", err)
	}
	defer windows.CertCloseStore(personal, 0)

	importados := 0
	var actual *windows.CertContext
	for {
		actual, err = windows.CertEnumCertificatesInStore(temporal, actual)
		if actual == nil {
			break
		}
		if !tieneClavePrivada(actual) {
			continue
		}
		if err := windows.CertAddCertificateContextToStore(personal, actual, certStoreAddReplaceKeep, nil); err != nil {
			windows.CertFreeCertificateContext(actual)
			return fmt.Errorf("añadir el certificado al almacén personal: %w", err)
		}
		importados++
	}
	if importados == 0 {
		return errors.New("El P12/PFX no contiene ningún certificado con clave privada.")
	}
	return nil
}

func tieneClavePrivada(contexto *windows.CertContext) bool {
	var tamano uint32
	r, _, _ := procCertGetCertificateContextProperty.Call(
		uintptr(unsafe.Pointer(contexto)),
		certKeyProvInfoPropID,
		0,
		uintptr(unsafe.Pointer(&tamano)),
	)
	return r != 0 && tamano > 0
}
