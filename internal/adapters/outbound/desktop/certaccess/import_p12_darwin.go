// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin && cgo

package certaccess

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static OSStatus afv2_import_pkcs12_to_default_keychain(const UInt8* data, CFIndex dataLen, const char* password) {
	CFDataRef importedData = CFDataCreate(kCFAllocatorDefault, data, dataLen);
	if (importedData == NULL) {
		return errSecAllocate;
	}

	CFStringRef passphrase = CFStringCreateWithCString(kCFAllocatorDefault, password, kCFStringEncodingUTF8);
	if (passphrase == NULL) {
		CFRelease(importedData);
		return errSecParam;
	}

	SecItemImportExportKeyParameters params;
	memset(&params, 0, sizeof(params));
	params.version = SEC_KEY_IMPORT_EXPORT_PARAMS_VERSION;
	params.passphrase = passphrase;

	SecExternalFormat format = kSecFormatPKCS12;
	SecExternalItemType itemType = kSecItemTypeAggregate;
	SecKeychainRef keychain = NULL;
	OSStatus status = SecKeychainCopyDefault(&keychain);
	if (status != errSecSuccess) {
		CFRelease(passphrase);
		CFRelease(importedData);
		return status;
	}

	CFArrayRef outItems = NULL;
	status = SecItemImport(
		importedData,
		CFSTR("p12"),
		&format,
		&itemType,
		0,
		&params,
		keychain,
		&outItems
	);

	if (outItems != NULL) {
		CFRelease(outItems);
	}
	CFRelease(keychain);
	CFRelease(passphrase);
	CFRelease(importedData);
	return status;
}
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/common/securefile"
)

func importarP12AMac(ctx context.Context, rutaP12, password string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := securefile.ReadFileLimit(rutaP12, 16*1024*1024)
	if err != nil {
		return fmt.Errorf("leer P12 para Keychain: %w", err)
	}
	defer secmem.Zeroize(data)
	if len(data) == 0 {
		return fmt.Errorf("leer P12 para Keychain: fichero vacío")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	cPassword := C.CString(password)
	if cPassword == nil {
		return fmt.Errorf("preparar contraseña para Keychain")
	}
	defer C.free(unsafe.Pointer(cPassword))

	status := C.afv2_import_pkcs12_to_default_keychain(
		(*C.UInt8)(unsafe.Pointer(&data[0])),
		C.CFIndex(len(data)),
		cPassword,
	)
	if status != C.errSecSuccess {
		return fmt.Errorf("Security.framework SecItemImport PKCS#12: OSStatus %d", int(status))
	}
	return nil
}
