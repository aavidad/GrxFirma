// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector_test

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"grxfirma/internal/adapters/outbound/common/keyringstore"
	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// almacenMemoria simula el almacén de secretos del sistema.
type almacenMemoria struct {
	mu        sync.Mutex
	datos     map[string][]byte
	fallaTodo bool
	// fallaStore simula un almacén que se puede leer pero no guardar.
	fallaStore bool
}

func nuevoAlmacenMemoria() *almacenMemoria {
	return &almacenMemoria{datos: map[string][]byte{}}
}

func (a *almacenMemoria) Store(_ context.Context, key string, value []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fallaTodo || a.fallaStore {
		return errors.New("Secret Service no disponible")
	}
	a.datos[key] = append([]byte(nil), value...)
	return nil
}

func (a *almacenMemoria) Load(_ context.Context, key string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fallaTodo {
		return nil, errors.New("Secret Service no disponible")
	}
	value, ok := a.datos[key]
	if !ok {
		// Como el almacén real: «no está» se distingue de «no responde».
		return nil, fmt.Errorf("%w: %q", keyringstore.ErrNoEncontrado, key)
	}
	return append([]byte(nil), value...), nil
}

func (a *almacenMemoria) Delete(_ context.Context, key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.datos, key)
	return nil
}

// ningunFicheroContiene recorre los directorios y falla si algún fichero
// contiene alguno de los secretos, en bruto o en base64.
func ningunFicheroContiene(t *testing.T, dirs []string, secretos ...[]byte) {
	t.Helper()
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, secreto := range secretos {
				b64 := []byte(base64.StdEncoding.EncodeToString(secreto))
				if bytes.Contains(data, secreto) || bytes.Contains(data, b64) {
					t.Errorf("%s contiene una clave privada del perfil alto", path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func identidadAltoV1(t *testing.T, dir string) (id string, seed, x25519Key []byte, path string) {
	t.Helper()
	mlkemPriv, err := mlkem.GenerateKey768()
	if err != nil {
		t.Fatal(err)
	}
	xPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(append(mlkemPriv.EncapsulationKey().Bytes(), xPriv.PublicKey().Bytes()...))
	id = hex.EncodeToString(sum[:8])
	raw, err := json.Marshal(map[string]any{
		"version":                1,
		"id":                     id,
		"label":                  "identidad antigua",
		"mlkem768_seed_b64":      base64.StdEncoding.EncodeToString(mlkemPriv.Bytes()),
		"x25519_private_key_b64": base64.StdEncoding.EncodeToString(xPriv.Bytes()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return id, mlkemPriv.Bytes(), xPriv.Bytes(), path
}

func TestLocalStrongKeyring_ClavePrivadaNoQuedaEnDisco(t *testing.T) {
	t.Parallel()

	cfg, data := t.TempDir(), t.TempDir()
	almacen := nuevoAlmacenMemoria()
	keyring := protector.NuevoLocalStrongKeyring(cfg, data, almacen)

	recipients, err := keyring.List(context.Background())
	if err != nil || len(recipients) != 1 {
		t.Fatalf("List() = %#v, %v", recipients, err)
	}
	keys, err := keyring.DecryptionKeys(context.Background(), documentoDirigidoA(t, recipients[0].ID))
	if err != nil || len(keys) != 1 {
		t.Fatalf("DecryptionKeys() = %d claves, %v", len(keys), err)
	}
	ningunFicheroContiene(t, []string{cfg, data}, keys[0].MLKEM768Seed, keys[0].X25519PrivateKey)

	// La identidad propia vive en el directorio de datos locales, no en el
	// de configuración (que en Windows es itinerante).
	if entries, _ := os.ReadDir(filepath.Join(data, "protection", "strong")); len(entries) != 1 {
		t.Fatalf("identidades en datos locales = %d; want 1", len(entries))
	}
	if _, err := os.Stat(filepath.Join(cfg, "protection", "strong")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no debe crearse protection/strong en el directorio de configuración: %v", err)
	}
	if len(almacen.datos) != 1 {
		t.Fatalf("secretos en el almacén del sistema = %d; want 1", len(almacen.datos))
	}
}

func TestLocalStrongKeyring_MigraIdentidadEnClaro(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	excepciones.SetForTest(t, "alto_clave_fichero_leer", true)
	t.Parallel()

	ctx := context.Background()
	cfg, data := t.TempDir(), t.TempDir()
	id, seed, xKey, legacyPath := identidadAltoV1(t, filepath.Join(cfg, "protection", "strong"))
	keyring := protector.NuevoLocalStrongKeyring(cfg, data, nuevoAlmacenMemoria())

	recipients, err := keyring.List(ctx)
	if err != nil || len(recipients) != 1 || recipients[0].ID != id {
		t.Fatalf("List() = %#v, %v; want la identidad migrada %s", recipients, err, id)
	}
	if _, err := os.Lstat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("el fichero en claro sigue en disco: %v", err)
	}
	ningunFicheroContiene(t, []string{cfg, data}, seed, xKey)

	doc, err := domain.NewDocument("alto.txt", []byte("documento del perfil alto"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	engine := protector.NuevoEnvelopeProtector()
	protected, err := engine.Protect(ctx, domain.ProtectionJob{Document: doc, Profile: domain.ProtectionProfileStrong}, recipients)
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	keys, err := keyring.DecryptionKeys(ctx, protected.Document)
	if err != nil || len(keys) != 1 {
		t.Fatalf("DecryptionKeys() = %d claves, %v", len(keys), err)
	}
	if !bytes.Equal(keys[0].MLKEM768Seed, seed) || !bytes.Equal(keys[0].X25519PrivateKey, xKey) {
		t.Fatal("la identidad migrada no conserva la clave privada original")
	}
	unprotected, err := engine.Unprotect(ctx, protected.Document, keys)
	if err != nil || string(unprotected.Document.Content) != string(doc.Content) {
		t.Fatalf("Unprotect() = %v; contenido %q", err, unprotected.Document.Content)
	}
}
