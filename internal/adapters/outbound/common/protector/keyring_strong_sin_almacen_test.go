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
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"testing"

	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// Lote 19: retrocompatibilidad de F-02. Sin almacén de secretos del sistema
// la identidad del perfil alto sigue guardada en el fichero v1 (0600) de
// siempre y se avisa; con almacén todo sigue como en F-02.

// destinatarioDeV1 deriva la parte pública de una identidad v1, como haría
// quien la exportó con la 0.0.125.
func destinatarioDeV1(t *testing.T, id string, seed, xKey []byte) domain.ProtectionRecipient {
	t.Helper()
	mlkemPriv, err := mlkem.NewDecapsulationKey768(seed)
	if err != nil {
		t.Fatal(err)
	}
	xPriv, err := ecdh.X25519().NewPrivateKey(xKey)
	if err != nil {
		t.Fatal(err)
	}
	return domain.ProtectionRecipient{
		ID:                id,
		Label:             "identidad antigua",
		MLKEM768PublicKey: mlkemPriv.EncapsulationKey().Bytes(),
		X25519PublicKey:   xPriv.PublicKey().Bytes(),
	}
}

func protegerAlto(t *testing.T, recipients ...domain.ProtectionRecipient) (domain.Document, domain.Document) {
	t.Helper()
	doc, err := domain.NewDocument("alto.txt", []byte("documento del perfil alto"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	protected, err := protector.NuevoEnvelopeProtector().Protect(context.Background(),
		domain.ProtectionJob{Document: doc, Profile: domain.ProtectionProfileStrong}, recipients)
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	return doc, protected.Document
}

func descifrarCon(t *testing.T, keyring *protector.LocalStrongKeyring, ctx context.Context, protected, original domain.Document) {
	t.Helper()
	keys, err := keyring.DecryptionKeys(ctx, protected)
	if err != nil || len(keys) != 1 {
		t.Fatalf("DecryptionKeys() = %d claves, %v; want 1", len(keys), err)
	}
	unprotected, err := protector.NuevoEnvelopeProtector().Unprotect(ctx, protected, keys)
	if err != nil || !bytes.Equal(unprotected.Document.Content, original.Content) {
		t.Fatalf("Unprotect() = %v; contenido %q", err, unprotected.Document.Content)
	}
}

func tieneAviso(avisos *domain.AvisosOperacion) bool {
	return slices.Contains(avisos.Claves(), protector.AvisoProteccionAltoSinAlmacen)
}

func TestLocalStrongKeyring_SinAlmacenUsaLaIdentidadV1(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_crear")
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	excepciones.SetForTest(t, "alto_clave_fichero_crear", true)
	excepciones.SetForTest(t, "alto_clave_fichero_leer", true)

	for nombre, almacen := range map[string]func() *almacenMemoria{
		"almacen_caido": func() *almacenMemoria { a := nuevoAlmacenMemoria(); a.fallaTodo = true; return a },
		"sin_almacen":   func() *almacenMemoria { return nil },
	} {
		t.Run(nombre, func(t *testing.T) {

			cfg, data := t.TempDir(), t.TempDir()
			id, seed, xKey, legacyPath := identidadAltoV1(t, filepath.Join(cfg, "protection", "strong"))
			var keyring *protector.LocalStrongKeyring
			if a := almacen(); a != nil {
				keyring = protector.NuevoLocalStrongKeyring(cfg, data, a)
			} else {
				keyring = protector.NuevoLocalStrongKeyring(cfg, data, nil)
			}
			original, protected := protegerAlto(t, destinatarioDeV1(t, id, seed, xKey))

			ctx, avisos := domain.ConAvisosOperacion(context.Background())
			keys, err := keyring.DecryptionKeys(ctx, protected)
			if err != nil || len(keys) != 1 {
				t.Fatalf("DecryptionKeys() = %d claves, %v; want la clave de la identidad v1", len(keys), err)
			}
			if !bytes.Equal(keys[0].MLKEM768Seed, seed) || !bytes.Equal(keys[0].X25519PrivateKey, xKey) {
				t.Fatal("la clave devuelta no es la del fichero v1")
			}
			descifrarCon(t, keyring, ctx, protected, original)
			if !tieneAviso(avisos) {
				t.Fatalf("avisos = %v; want %s", avisos.Claves(), protector.AvisoProteccionAltoSinAlmacen)
			}
			if _, err := os.Stat(legacyPath); err != nil {
				t.Fatalf("el fichero v1 debe seguir en disco: %v", err)
			}

			// La identidad sale en la lista y se puede proteger y exportar.
			recipients, err := keyring.List(context.Background())
			if err != nil || len(recipients) != 1 || recipients[0].ID != id {
				t.Fatalf("List() = %#v, %v; want la identidad v1 %s", recipients, err, id)
			}
			ctxProteger, avisosProteger := domain.ConAvisosOperacion(context.Background())
			if resolved, err := keyring.Resolve(ctxProteger, []string{id}); err != nil || len(resolved) != 1 {
				t.Fatalf("Resolve() = %#v, %v", resolved, err)
			}
			if !tieneAviso(avisosProteger) {
				t.Fatal("proteger con la identidad en fichero no avisa")
			}
			ctxExportar, avisosExportar := domain.ConAvisosOperacion(context.Background())
			if _, exported, err := keyring.Export(ctxExportar, id); err != nil || len(exported) == 0 {
				t.Fatalf("Export() error = %v", err)
			} else {
				ningunSecretoEn(t, exported, seed, xKey)
			}
			if !tieneAviso(avisosExportar) {
				t.Fatal("exportar la identidad en fichero no avisa")
			}
			if entries, _ := os.ReadDir(filepath.Join(data, "protection", "strong")); len(entries) != 0 {
				t.Fatalf("sin almacén no debe escribirse otra identidad: %d ficheros", len(entries))
			}
		})
	}
}

func ningunSecretoEn(t *testing.T, data []byte, secretos ...[]byte) {
	t.Helper()
	for _, secreto := range secretos {
		if bytes.Contains(data, secreto) || bytes.Contains(data, []byte(base64.StdEncoding.EncodeToString(secreto))) {
			t.Fatal("la exportación contiene una clave privada")
		}
	}
}

// capturarSlog cambia el registro global. Quien lo usa no llama a t.Parallel:
// así corre antes que las pruebas en paralelo del paquete y no se mezcla.
func capturarSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	registro := &bytes.Buffer{}
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(registro, nil)))
	t.Cleanup(func() { slog.SetDefault(anterior) })
	return registro
}

func TestLocalStrongKeyring_SinAlmacenCreaV1YLaMigraAlVolver(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_crear")
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	excepciones.SetForTest(t, "alto_clave_fichero_crear", true)
	excepciones.SetForTest(t, "alto_clave_fichero_leer", true)
	registro := capturarSlog(t)

	cfg, data := t.TempDir(), t.TempDir()
	almacen := nuevoAlmacenMemoria()
	almacen.fallaTodo = true
	keyring := protector.NuevoLocalStrongKeyring(cfg, data, almacen)

	ctx, avisos := domain.ConAvisosOperacion(context.Background())
	recipients, err := keyring.List(ctx)
	if err != nil || len(recipients) != 1 {
		t.Fatalf("List() = %d identidades, %v; want una identidad nueva en fichero", len(recipients), err)
	}
	if !tieneAviso(avisos) {
		t.Fatal("crear la identidad en fichero no avisa")
	}
	id := recipients[0].ID
	legacyDir := filepath.Join(cfg, "protection", "strong")
	legacyPath := filepath.Join(legacyDir, id+".json")
	info, err := os.Stat(legacyPath)
	if err != nil {
		t.Fatalf("no se escribió la identidad v1: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("permisos de la identidad v1 = %o; want 600", perm)
		}
		if dirInfo, err := os.Stat(legacyDir); err != nil || dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("permisos del directorio = %v, %v; want 700", dirInfo, err)
		}
	}
	raw, err := os.ReadFile(legacyPath)
	if err != nil || !strings.Contains(string(raw), `"version": 1`) || !strings.Contains(string(raw), "mlkem768_seed_b64") {
		t.Fatalf("la identidad no está en el formato v1 de siempre: %s, %v", raw, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(data, "protection", "strong")); len(entries) != 0 {
		t.Fatalf("sin almacén no debe escribirse la parte pública v2: %d ficheros", len(entries))
	}

	// Una segunda llamada no crea otra identidad.
	if again, err := keyring.List(context.Background()); err != nil || len(again) != 1 || again[0].ID != id {
		t.Fatalf("List() repetido = %#v, %v", again, err)
	}

	original, protected := protegerAlto(t, recipients[0])
	descifrarCon(t, keyring, context.Background(), protected, original)

	// El registro avisa sin escribir la clave.
	if !strings.Contains(registro.String(), "protector_strong_key_unprotected_file") {
		t.Fatalf("falta el aviso en el registro:\n%s", registro.String())
	}
	keys, _ := keyring.DecryptionKeys(context.Background(), protected)
	for _, key := range keys {
		for _, secreto := range [][]byte{key.MLKEM768Seed, key.X25519PrivateKey} {
			if strings.Contains(registro.String(), base64.StdEncoding.EncodeToString(secreto)) {
				t.Fatal("el registro contiene la clave privada")
			}
		}
	}

	// Vuelve el almacén: la identidad se migra y el fichero v1 desaparece.
	almacen.mu.Lock()
	almacen.fallaTodo = false
	almacen.mu.Unlock()
	ctxMigrada, avisosMigrada := domain.ConAvisosOperacion(context.Background())
	migrated, err := keyring.List(ctxMigrada)
	if err != nil || len(migrated) != 1 || migrated[0].ID != id {
		t.Fatalf("List() tras volver el almacén = %#v, %v", migrated, err)
	}
	if _, err := os.Lstat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("el fichero v1 sigue en disco tras migrar: %v", err)
	}
	descifrarCon(t, keyring, ctxMigrada, protected, original)
	if tieneAviso(avisosMigrada) {
		t.Fatal("con la clave ya en el almacén no debe avisarse")
	}
	if len(almacen.datos) != 1 {
		t.Fatalf("secretos en el almacén = %d; want 1", len(almacen.datos))
	}
}

func TestLocalStrongKeyring_FalloAlGuardarNoPierdeLaV1(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_crear")
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	excepciones.SetForTest(t, "alto_clave_fichero_crear", true)
	excepciones.SetForTest(t, "alto_clave_fichero_leer", true)

	cfg, data := t.TempDir(), t.TempDir()
	id, seed, xKey, legacyPath := identidadAltoV1(t, filepath.Join(cfg, "protection", "strong"))
	almacen := nuevoAlmacenMemoria()
	almacen.fallaStore = true
	keyring := protector.NuevoLocalStrongKeyring(cfg, data, almacen)
	original, protected := protegerAlto(t, destinatarioDeV1(t, id, seed, xKey))

	ctx, avisos := domain.ConAvisosOperacion(context.Background())
	descifrarCon(t, keyring, ctx, protected, original)
	if !tieneAviso(avisos) {
		t.Fatal("falta el aviso de clave en fichero")
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("se perdió la identidad v1: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(data, "protection", "strong")); len(entries) != 0 {
		t.Fatalf("no debe quedar una parte pública v2 sin su clave en el almacén: %d ficheros", len(entries))
	}
}

func TestLocalStrongKeyring_MixtoV2YV1SinMigrar(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_crear")
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	excepciones.SetForTest(t, "alto_clave_fichero_crear", true)
	excepciones.SetForTest(t, "alto_clave_fichero_leer", true)

	cfg, data := t.TempDir(), t.TempDir()
	almacen := nuevoAlmacenMemoria()
	keyring := protector.NuevoLocalStrongKeyring(cfg, data, almacen)
	v2, err := keyring.List(context.Background())
	if err != nil || len(v2) != 1 {
		t.Fatalf("List() inicial = %#v, %v", v2, err)
	}

	// Aparece una v1 y el almacén falla al guardar: no se migra esta vez.
	id, seed, xKey, legacyPath := identidadAltoV1(t, filepath.Join(cfg, "protection", "strong"))
	almacen.mu.Lock()
	almacen.fallaStore = true
	almacen.mu.Unlock()

	recipients, err := keyring.List(context.Background())
	if err != nil || len(recipients) != 2 {
		t.Fatalf("List() mixto = %d identidades, %v; want la v2 y la v1", len(recipients), err)
	}
	originalV2, protectedV2 := protegerAlto(t, v2[0])
	descifrarCon(t, keyring, context.Background(), protectedV2, originalV2)
	originalV1, protectedV1 := protegerAlto(t, destinatarioDeV1(t, id, seed, xKey))
	descifrarCon(t, keyring, context.Background(), protectedV1, originalV1)
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("se perdió la identidad v1: %v", err)
	}
}

func TestLocalStrongKeyring_V2HuerfanaNoCaeAFichero(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_crear")
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	excepciones.SetForTest(t, "alto_clave_fichero_crear", true)
	excepciones.SetForTest(t, "alto_clave_fichero_leer", true)

	cfg, data := t.TempDir(), t.TempDir()
	almacen := nuevoAlmacenMemoria()
	keyring := protector.NuevoLocalStrongKeyring(cfg, data, almacen)
	recipients, err := keyring.List(context.Background())
	if err != nil || len(recipients) != 1 {
		t.Fatalf("List() = %#v, %v", recipients, err)
	}
	// El almacén funciona pero ya no tiene la entrada.
	almacen.mu.Lock()
	clear(almacen.datos)
	almacen.mu.Unlock()
	excepciones.SetForTest(t, "alto_clave_fichero_leer", false)

	ctx, avisos := domain.ConAvisosOperacion(context.Background())
	keys, err := keyring.DecryptionKeys(ctx, documentoDirigidoA(t, recipients[0].ID))
	if err != nil || len(keys) != 0 {
		t.Fatalf("DecryptionKeys() = %d claves, %v; want ninguna", len(keys), err)
	}
	if tieneAviso(avisos) {
		t.Fatal("una v2 huérfana no debe avisar de clave en fichero")
	}
	if _, err := os.Stat(filepath.Join(cfg, "protection", "strong")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("una v2 huérfana no debe crear una identidad en fichero: %v", err)
	}
}

func TestLocalCombinedKeyring_NoOcultaElAltoConV1Usable(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_crear")
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	excepciones.SetForTest(t, "alto_clave_fichero_crear", true)
	excepciones.SetForTest(t, "alto_clave_fichero_leer", true)

	ctx := context.Background()
	cfg, data := t.TempDir(), t.TempDir()
	id, seed, xKey, _ := identidadAltoV1(t, filepath.Join(cfg, "protection", "strong"))
	almacen := nuevoAlmacenMemoria()
	almacen.fallaTodo = true
	combined := &protector.LocalCombinedKeyring{Strong: protector.NuevoLocalStrongKeyring(cfg, data, almacen)}

	recipients, err := combined.List(ctx)
	if err != nil || len(recipients) != 1 || recipients[0].ID != id {
		t.Fatalf("LocalCombinedKeyring.List() = %#v, %v; want la identidad v1", recipients, err)
	}
	if resolved, err := combined.Resolve(ctx, []string{id}); err != nil || len(resolved) != 1 {
		t.Fatalf("LocalCombinedKeyring.Resolve() = %#v, %v", resolved, err)
	}
	_, protected := protegerAlto(t, destinatarioDeV1(t, id, seed, xKey))
	keys, err := combined.DecryptionKeys(ctx, protected)
	if err != nil || len(keys) != 1 || keys[0].RecipientID != id {
		t.Fatalf("LocalCombinedKeyring.DecryptionKeys() = %d claves, %v", len(keys), err)
	}
}

func TestLocalStrongKeyring_BloqueaClaveFicheroSinAjuste(t *testing.T) {
	excepciones.SetForTest(t, "alto_clave_fichero_leer", false)
	excepciones.SetForTest(t, "alto_clave_fichero_crear", false)
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_crear")
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	cfg, data := t.TempDir(), t.TempDir()
	legacyDir := filepath.Join(cfg, "protection", "strong")
	id, seed, xKey, _ := identidadAltoV1(t, legacyDir)
	keyring := protector.NuevoLocalStrongKeyring(cfg, data, nil)
	protectedOriginal, protected := protegerAlto(t, destinatarioDeV1(t, id, seed, xKey))
	_ = protectedOriginal
	if _, err := keyring.DecryptionKeys(context.Background(), protected); err == nil || !strings.Contains(err.Error(), "alto_clave_fichero_leer") {
		t.Fatalf("lectura sin bloqueo: %v", err)
	}
	// Una directiva ilegible y una prohibición explícita prevalecen sobre el ajuste.
	for _, tc := range []struct {
		name   string
		policy func(*testing.T)
	}{
		{"prohibido", func(t *testing.T) { machinepolicy.SetForTest(t, "permitir_alto_clave_fichero_leer", false) }},
		{"ilegible", func(t *testing.T) {
			machinepolicy.ErrorForTest(t, "permitir_alto_clave_fichero_leer", errors.New("ilegible"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			excepciones.SetForTest(t, "alto_clave_fichero_leer", true)
			tc.policy(t)
			if _, err := keyring.DecryptionKeys(context.Background(), protected); err == nil || !strings.Contains(err.Error(), "alto_clave_fichero_leer") {
				t.Fatalf("lectura sin bloqueo: %v", err)
			}
		})
	}
	noLegacy := protector.NuevoLocalStrongKeyring(t.TempDir(), t.TempDir(), nil)
	if _, err := noLegacy.List(context.Background()); err == nil || !strings.Contains(err.Error(), "alto_clave_fichero_crear") {
		t.Fatalf("creación sin bloqueo: %v", err)
	}
}

func TestLocalStrongKeyring_ConfiguracionUsuarioActivaSoloLoElegido(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_crear")
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	cfg, data := t.TempDir(), t.TempDir()
	for _, id := range []string{"alto_clave_fichero_crear", "alto_clave_fichero_leer"} {
		if _, err := excepciones.ActivateLocal(excepciones.ForLocalCLI(context.Background()), cfg, excepciones.ActivationRequest{
			ID: id, Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion,
		}); err != nil {
			t.Fatal(err)
		}
	}
	keyring := protector.NuevoLocalStrongKeyring(cfg, data, nil)
	ctx, warnings := domain.ConAvisosOperacion(context.Background())
	recipients, err := keyring.List(ctx)
	if err != nil || len(recipients) != 1 {
		t.Fatalf("identidad sin almacén: %v, %v", recipients, err)
	}
	if !slices.Contains(warnings.Claves(), excepciones.Warning("alto_clave_fichero_crear")) {
		t.Fatalf("falta aviso de creación: %v", warnings.Claves())
	}
	if !slices.Contains(warnings.Claves(), excepciones.Warning("alto_clave_fichero_leer")) {
		t.Fatalf("falta aviso de lectura: %v", warnings.Claves())
	}
}

func TestLocalStrongKeyring_V2NoDependeDeLeerV1Bloqueada(t *testing.T) {
	cfg, data := t.TempDir(), t.TempDir()
	store := nuevoAlmacenMemoria()
	keyring := protector.NuevoLocalStrongKeyring(cfg, data, store)
	v2, err := keyring.List(context.Background())
	if err != nil || len(v2) != 1 {
		t.Fatalf("crear identidad v2: %v, %v", v2, err)
	}
	_, _, _, _ = identidadAltoV1(t, filepath.Join(cfg, "protection", "strong"))
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	excepciones.SetForTest(t, "alto_clave_fichero_leer", false)
	ctx, warnings := domain.ConAvisosOperacion(context.Background())
	keys, err := keyring.DecryptionKeys(ctx, documentoDirigidoA(t, v2[0].ID))
	if err != nil || len(keys) != 1 || keys[0].RecipientID != v2[0].ID {
		t.Fatalf("la v1 bloqueada impide usar v2: claves=%d err=%v", len(keys), err)
	}
	if slices.Contains(warnings.Claves(), excepciones.Warning("alto_clave_fichero_leer")) {
		t.Fatalf("la operación v2 no leyó clave v1: %v", warnings.Claves())
	}
}
