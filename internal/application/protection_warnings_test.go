// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/keyringstore"
	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type almacenProteccionPrueba map[string][]byte

func (a almacenProteccionPrueba) Store(_ context.Context, k string, v []byte) error {
	a[k] = slices.Clone(v)
	return nil
}
func (a almacenProteccionPrueba) Load(_ context.Context, k string) ([]byte, error) {
	if v, ok := a[k]; ok {
		return slices.Clone(v), nil
	}
	return nil, keyringstore.ErrNoEncontrado
}
func (a almacenProteccionPrueba) Delete(_ context.Context, k string) error { delete(a, k); return nil }

// Usa el keyring y el motor reales: el aviso no bloquea la operación y deja
// de aparecer cuando la migración al almacén vuelve a estar disponible.
func TestProteccionYExportacion_AvisoSinAlmacenYMigracion(t *testing.T) {
	for _, conAlmacen := range []bool{false, true} {
		nombre := "sin_almacen"
		if conAlmacen {
			nombre = "con_almacen"
		}
		t.Run(nombre, func(t *testing.T) {
			ctx := context.Background()
			config, data := t.TempDir(), t.TempDir()
			var almacen ports.SecureStorage
			if conAlmacen {
				almacen = almacenProteccionPrueba{}
			}
			k := protector.NuevoLocalStrongKeyring(config, data, almacen)
			recipients, err := k.List(ctx)
			if err != nil || len(recipients) != 1 {
				t.Fatalf("List = %v, %v", recipients, err)
			}
			id := recipients[0].ID
			doc, err := domain.NewDocument("acta.txt", []byte("contenido"), "text/plain")
			if err != nil {
				t.Fatal(err)
			}
			uc := application.NuevoProtectDocumentUseCase(k, protector.NuevoEnvelopeProtector(), nil, nil, nil)
			exportar := application.NuevoExportProtectionRecipientUseCase(k)
			ejecutar := func(quiereAviso bool) {
				t.Helper()
				// No hereda avisos del contexto padre, pero propaga los de esta operación.
				padre, avisosPadre := domain.ConAvisosOperacion(ctx)
				domain.AnotarAviso(padre, "aviso ajeno")
				res, err := uc.Execute(padre, application.ProtectCommand{Document: doc, Profile: domain.ProtectionProfileStrong, RecipientIDs: []string{id}})
				if err != nil || len(res.Protected.Document.Content) == 0 {
					t.Fatalf("Protect = %+v, %v", res, err)
				}
				exp, err := exportar.Execute(padre, application.ExportProtectionRecipientCommand{RecipientID: id})
				if err != nil || len(exp.Data) == 0 {
					t.Fatalf("Export = %+v, %v", exp, err)
				}
				var want []string
				if quiereAviso {
					want = []string{protector.AvisoProteccionAltoSinAlmacen}
				}
				if !slices.Equal(res.Warnings, want) || !slices.Equal(exp.Warnings, want) {
					t.Fatalf("avisos proteger=%q exportar=%q; want %q", res.Warnings, exp.Warnings, want)
				}
				if quiereAviso && !slices.Contains(avisosPadre.Claves(), protector.AvisoProteccionAltoSinAlmacen) {
					t.Fatal("se perdió el aviso en el contexto padre")
				}
			}
			ejecutar(!conAlmacen)
			legacy := filepath.Join(config, "protection", "strong", id+".json")
			if !conAlmacen {
				info, err := os.Stat(legacy)
				if err != nil {
					t.Fatal(err)
				}
				if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
					t.Fatalf("permisos = %o", info.Mode().Perm())
				}
				k.Secrets = almacenProteccionPrueba{}
				ejecutar(false)
				if _, err := os.Stat(legacy); !os.IsNotExist(err) {
					t.Fatalf("el fichero antiguo no se borró al migrar: %v", err)
				}
			}
			raw, err := os.ReadFile(filepath.Join(data, "protection", "strong", id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"version": 2`) || strings.Contains(string(raw), "mlkem768_seed_b64") || strings.Contains(string(raw), "x25519_private_key_b64") {
				t.Fatalf("la identidad pública contiene datos inesperados: %s", raw)
			}
		})
	}
}

func TestProteccionNormal_NoLlevaAvisoDelPerfilAlto(t *testing.T) {
	for _, conAlmacen := range []bool{false, true} {
		nombre := "sin_almacen"
		if conAlmacen {
			nombre = "con_almacen"
		}
		t.Run(nombre, func(t *testing.T) {
			ctx := context.Background()
			var almacen ports.SecureStorage
			if conAlmacen {
				almacen = almacenProteccionPrueba{}
			}
			k := protector.NuevoLocalStrongKeyring(t.TempDir(), t.TempDir(), almacen)
			// El listado inicial es el que ofrece ambos perfiles en la aplicación.
			if _, err := k.List(ctx); err != nil {
				t.Fatal(err)
			}
			priv, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal(err)
			}
			book := protector.NuevoLocalPublicRecipientBook(t.TempDir())
			recipient, err := book.Import(ctx, mustCompatRecipientCertDER(t, priv))
			if err != nil {
				t.Fatal(err)
			}
			catalogo := &protector.LocalCombinedKeyring{Strong: k, Public: book}
			doc, err := domain.NewDocument("normal.txt", []byte("contenido"), "text/plain")
			if err != nil {
				t.Fatal(err)
			}
			uc := application.NuevoProtectDocumentUseCase(catalogo, protector.NuevoEnvelopeProtector(), nil, nil, nil)
			res, err := uc.Execute(ctx, application.ProtectCommand{Document: doc, Profile: domain.ProtectionProfileCompat, RecipientIDs: []string{recipient.ID}})
			if err != nil || len(res.Warnings) != 0 {
				t.Fatalf("protección normal: avisos=%q err=%v", res.Warnings, err)
			}
		})
	}
}
