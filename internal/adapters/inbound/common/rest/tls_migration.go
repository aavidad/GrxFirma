// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
)

const legacyRESTPrefix = "rest-localhost"

// retireLegacyRESTCertificate retira únicamente el material reconocible de la
// antigua CA REST. Los inventarios son la única prueba de confianza instalada
// por GrxFirma; sin ellos no se atribuye ninguna entrada del sistema.
func retireLegacyRESTCertificate(ctx context.Context, dir string) error {
	if dir == "" {
		return nil
	}
	if err := prepareTLSDirectory(dir); err != nil {
		return err
	}
	unlock, err := acquireTLSLock(filepath.Join(dir, "."+legacyRESTPrefix+"-ca.lock"))
	if err != nil {
		return fmt.Errorf("rest: bloquear migración TLS: %w", err)
	}
	defer unlock()

	rootFile := filepath.Join(dir, legacyRESTPrefix+"-root.crt.pem")
	root, ok := cargarCertificadoLocal(rootFile)
	if !ok || !localtlstrust.IsManagedLocalCA(root) || !localCANameConstraintsValid(root) {
		return nil
	}
	// La retirada gestionada valida las huellas y los almacenes inventariados.
	currentInventory := ".grxfirma-trust.json"
	otherInventory := ".grxfirma-nss-trust.json"
	if runtime.GOOS == "linux" {
		currentInventory, otherInventory = otherInventory, currentInventory
	}
	if _, err := os.Lstat(rootFile + currentInventory); err == nil {
		if err := localtlstrust.RemoveManagedTrusted(ctx, rootFile); err != nil {
			return fmt.Errorf("rest: retirar confianza REST anterior: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Lstat(rootFile + otherInventory); err == nil {
		// Un inventario de otra plataforma conserva la prueba de propiedad para
		// retirarlo allí. El REST ya no utiliza este certificado.
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	rootKeyFile := filepath.Join(dir, legacyRESTPrefix+"-root.key.pem")
	if securePrivateKeyFile(rootKeyFile) {
		keyDER, err := localtlstrust.LoadManagedCAKey(rootKeyFile)
		if err == nil {
			key, parseErr := x509.ParsePKCS1PrivateKey(keyDER)
			secmem.Zeroize(keyDER)
			if parseErr == nil {
				if public, yes := root.PublicKey.(*rsa.PublicKey); yes &&
					key.PublicKey.E == public.E && key.PublicKey.N.Cmp(public.N) == 0 {
					if err := os.Remove(rootKeyFile); err != nil {
						return err
					}
				}
			}
		}
	}
	leafFile := filepath.Join(dir, legacyRESTPrefix+".crt.pem")
	keyFile := filepath.Join(dir, legacyRESTPrefix+".key.pem")
	if regularTLSFile(leafFile) && securePrivateKeyFile(keyFile) {
		if pair, _, err := loadX509KeyPairSecure(leafFile, keyFile); err == nil && len(pair.Certificate) > 0 {
			if leaf, err := x509.ParseCertificate(pair.Certificate[0]); err == nil && legacyRESTLeafOwned(leaf, root) {
				if err := os.Remove(keyFile); err != nil {
					return err
				}
				if err := os.Remove(leafFile); err != nil {
					return err
				}
			}
		}
	}
	if err := os.Remove(rootFile); err != nil {
		return err
	}
	for _, suffix := range []string{currentInventory, ".trustcache.json"} {
		path := rootFile + suffix
		if regularTLSFile(path) {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func legacyRESTLeafOwned(leaf, root *x509.Certificate) bool {
	if leaf == nil || root == nil {
		return false
	}
	if leaf.CheckSignatureFrom(root) == nil {
		return true
	}
	// La antigua orden CLI generaba una hoja autofirmada con este perfil; las
	// versiones anteriores usaban otra organización y se siguen reconociendo.
	return bytes.Equal(leaf.RawIssuer, leaf.RawSubject) &&
		leaf.Subject.CommonName == "localhost" &&
		len(leaf.Subject.Organization) == 1 &&
		(leaf.Subject.Organization[0] == localtlstrust.ManagedLocalCAOrganization ||
			leaf.Subject.Organization[0] == "Diputacion de Granada") &&
		len(leaf.DNSNames) == 1 && leaf.DNSNames[0] == "localhost" &&
		len(leaf.IPAddresses) == 1 && leaf.IPAddresses[0].String() == "127.0.0.1" &&
		leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature) == nil
}

// ClearManagedLocalhostTLS retira la confianza inventariada y los ficheros de
// la identidad compartida, sin vaciar otros ficheros del directorio TLS.
func ClearManagedLocalhostTLS(ctx context.Context, dir string) (int, error) {
	paths := tlsRESTManagedArtifactPaths(dir)
	if err := validateTLSRESTCleanupTargets(dir, paths); err != nil {
		return 0, err
	}
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	if err := retireLegacyRESTCertificate(ctx, dir); err != nil {
		return 0, err
	}
	unlock, err := acquireTLSLock(filepath.Join(dir, "."+ManagedLocalhostPrefix+"-ca.lock"))
	if err != nil {
		return 0, fmt.Errorf("rest: bloquear limpieza TLS: %w", err)
	}
	defer unlock()
	if err := validateTLSRESTCleanupTargets(dir, paths); err != nil {
		return 0, err
	}
	rootFile := filepath.Join(dir, ManagedLocalhostPrefix+"-root.crt.pem")
	if root, ok := cargarCertificadoLocal(rootFile); ok {
		if !localtlstrust.IsManagedLocalCA(root) || !localCANameConstraintsValid(root) {
			return 0, errors.New("rest: CA TLS ajena")
		}
	} else if _, err := os.Lstat(rootFile); err == nil {
		return 0, errors.New("rest: CA TLS no verificable")
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	if err := localtlstrust.RemoveManagedTrusted(ctx, rootFile); err != nil {
		return 0, err
	}
	return removeTLSRESTManagedArtifacts(paths)
}
