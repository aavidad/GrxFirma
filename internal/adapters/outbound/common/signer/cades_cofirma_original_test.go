// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// cofirmarConOriginal cofirma o contrafirma firma con un certificado nuevo,
// pasando original al motor, y devuelve el resultado y los avisos anotados.
func cofirmarConOriginal(t *testing.T, firma, original []byte, accion domain.SignatureAction) ([]byte, []string, error) {
	t.Helper()
	priv, cert := generarCertRSAPruebaConCN(t, "Cofirmante con original")
	clave := &commonsigner.LocalSigningKey{ID: "cofirmante", Signer: priv, Certificate: cert}
	doc, err := domain.NewDocument("firma.csig", firma, "application/pkcs7-signature")
	if err != nil {
		t.Fatal(err)
	}
	ctx, avisos := domain.ConAvisosOperacion(context.Background())
	res, err := commonsigner.NewCAdESBESDetached().Sign(ctx, domain.SignatureJob{
		Document:        doc,
		Format:          domain.FormatCAdES,
		Action:          accion,
		OriginalContent: original,
	}, clave)
	return res.Data, avisos.Claves(), err
}

// Lote 17b (3c y KD-03): una CAdES explícita cuyos firmantes solo usan SHA-1
// no se podía cofirmar porque el motor no tenía de dónde sacar el resumen
// SHA-256. Con el original, el motor lo coteja, cofirma y avisa del SHA-1;
// con firmantes mixtos, igual. Un original que no corresponde no se firma.
func TestCAdES_CofirmaExplicitaSHA1ConOriginal(t *testing.T) {
	machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
	excepciones.SetForTest(t, "sha1_conservar", true)
	machinepolicy.UnsetForTest(t, "permitir_cofirma_sin_original")
	excepciones.SetForTest(t, "cofirma_sin_original", true)
	original := []byte("acta firmada en 2012 con AutoFirma")
	soloSHA1 := cadesSHA1Prueba(t, original, true)

	if _, _, err := cofirmarConOriginal(t, soloSHA1, nil, domain.ActionCoSign); err == nil {
		t.Fatal("sin original no hay de dónde sacar el resumen SHA-256: debe fallar como antes")
	}

	mixta, avisos, err := cofirmarConOriginal(t, soloSHA1, original, domain.ActionCoSign)
	if err != nil {
		t.Fatalf("con el original la cofirma SHA-1 debe hacerse: %v", err)
	}
	if want := []string{commonsigner.AvisoFirmaSHA1Corresponde}; !slices.Equal(avisos, want) {
		t.Fatalf("avisos = %q, want %q", avisos, want)
	}
	// Todos los firmantes, también el nuevo (SHA-256), cubren el original.
	if err := commonsigner.CotejarCAdESExplicita(mixta, original); !errors.Is(err, commonsigner.ErrFirmaCorrespondeConSHA1) {
		t.Fatalf("la cofirma debe cubrir el original con todos sus firmantes: %v", err)
	}

	t.Run("mixta_con_original", func(t *testing.T) {
		otra, avisos, err := cofirmarConOriginal(t, mixta, original, domain.ActionCoSign)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{commonsigner.AvisoFirmaSHA1Corresponde}; !slices.Equal(avisos, want) {
			t.Fatalf("avisos = %q, want %q", avisos, want)
		}
		if err := commonsigner.CotejarCAdESExplicita(otra, original); !errors.Is(err, commonsigner.ErrFirmaCorrespondeConSHA1) {
			t.Fatalf("la cofirma mixta debe seguir cubriendo el original: %v", err)
		}
	})
	t.Run("mixta_sin_original", func(t *testing.T) {
		// Retrocompatibilidad: con un firmante SHA-256 se cofirma sin el
		// original, con el aviso de que no se ha visto el documento.
		_, avisos, err := cofirmarConOriginal(t, mixta, nil, domain.ActionCoSign)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{excepciones.Warning("cofirma_sin_original")}; !slices.Equal(avisos, want) {
			t.Fatalf("avisos = %q, want %q", avisos, want)
		}
	})
	t.Run("contrafirma_con_original", func(t *testing.T) {
		_, avisos, err := cofirmarConOriginal(t, soloSHA1, original, domain.ActionCounterSign)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{commonsigner.AvisoFirmaSHA1Corresponde}; !slices.Equal(avisos, want) {
			t.Fatalf("avisos = %q, want %q", avisos, want)
		}
	})
	for nombre, firma := range map[string][]byte{"solo_sha1": soloSHA1, "mixta": mixta} {
		t.Run("original_ajeno_"+nombre, func(t *testing.T) {
			for _, accion := range []domain.SignatureAction{domain.ActionCoSign, domain.ActionCounterSign} {
				res, _, err := cofirmarConOriginal(t, firma, []byte("otro documento"), accion)
				if !errors.Is(err, commonsigner.ErrFirmaNoCorrespondeOriginal) || len(res) != 0 {
					t.Fatalf("%s con un original ajeno no debe firmar: err=%v", accion, err)
				}
			}
		})
	}
}

// Con SHA-256 y el original, la cofirma no lleva aviso: se ha visto el
// documento y no hay algoritmo antiguo.
func TestCAdES_CofirmaExplicitaConOriginalSinAviso(t *testing.T) {
	original := []byte("documento que quien cofirma ha visto")
	firma, _, err := cofirmarConOriginal(t, original, nil, domain.ActionSign)
	if err != nil {
		t.Fatal(err)
	}
	cofirma, avisos, err := cofirmarConOriginal(t, firma, original, domain.ActionCoSign)
	if err != nil {
		t.Fatal(err)
	}
	if len(avisos) != 0 {
		t.Fatalf("avisos = %q, want ninguno", avisos)
	}
	if err := commonsigner.CotejarCAdESExplicita(cofirma, original); err != nil {
		t.Fatalf("la cofirma debe cubrir el original: %v", err)
	}
}

// La política de máquina con SHA-1 prohibido cierra también la cofirma con
// original de una firma SHA-1.
func TestCAdES_CofirmaExplicitaSHA1LoCierraLaPolitica(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	original := []byte("acta firmada en 2012 con AutoFirma")
	res, _, err := cofirmarConOriginal(t, cadesSHA1Prueba(t, original, true), original, domain.ActionCoSign)
	if !errors.Is(err, commonsigner.ErrCotejoSHA1Prohibido) || len(res) != 0 {
		t.Fatalf("con SHA-1 prohibido no se cofirma: err=%v", err)
	}
}
