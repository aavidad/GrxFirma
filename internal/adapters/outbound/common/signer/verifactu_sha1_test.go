// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

// firmarVeriFactuSHA1 firma un registro como lo hacían programas antiguos:
// mismo perfil que «Registro Veri*Factu», pero con RSA-SHA1 y resúmenes SHA-1.
// GrxFirma nunca firma así; solo sirve para comprobar la lectura.
func firmarVeriFactuSHA1(t *testing.T, data []byte, priv crypto.Signer, cert *x509.Certificate) []byte {
	t.Helper()
	k := &LocalSigningKey{ID: "sha1", Signer: priv, Certificate: cert}
	// Se construye a mano: la política criptográfica impide firmar con SHA-1.
	alg := xadesAlgorithmOptions{Label: "SHA1withRSA", Hash: crypto.SHA1, SignatureMethod: algRSASHA1, DigestMethod: algSHA1}
	opts := resolveXAdESBuildOptions(map[string]string{"expPolicy": "firmaage19"}, alg)
	opts.DocumentTransforms = []string{algEnveloped, algExcC14N}
	canon, err := canonicalizeXML(data, algExcC14N)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := digestBytes(alg.Hash, canon)
	if err != nil {
		t.Fatal(err)
	}
	ids := idsXAdESNuevos()
	info, value, keyInfo, props, err := firmarPartesXAdES(k, alg, opts, ids, "", base64.StdEncoding.EncodeToString(digest), "text/xml")
	if err != nil {
		t.Fatal(err)
	}
	out, err := insertarFirmaEnRaiz(data, ensamblarFirmaXAdES(ids, opts.Namespace, info, value, keyInfo, "", props))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), algRSASHA1) {
		t.Fatal("la firma de prueba no usa RSA-SHA1")
	}
	return out
}

func clavesVeriFactu(problemas []vfProblem) map[string]string {
	out := map[string]string{}
	for _, p := range problemas {
		out[p.Key] = p.Level
	}
	return out
}

// Lote 14: un registro Veri*Factu firmado con SHA-1 sigue el criterio común.
// La integridad se comprueba; por defecto no se da por bueno, y el usuario
// puede aceptarlo en su equipo (CLI y escritorio). Antes salía «perfil».
func TestVeriFactuSHA1MismoCriterioQueLaVerificacion(t *testing.T) {
	priv, cert := certForTest(t, "VeriFactuSHA1")
	data := vfTestXML("RegistroAlta", "12345678/G33", "", time.Now().UTC().Format(time.RFC3339))
	firmado := firmarVeriFactuSHA1(t, data, priv, cert)
	n, err := vfParse(firmado)
	if err != nil {
		t.Fatal(err)
	}
	sha1 := domain.AceptacionConfianza{Tipo: domain.ConfianzaAlgoritmoSHA1}
	otras := []domain.AceptacionConfianza{
		{Tipo: domain.ConfianzaRevocacionNoComprobable},
		{Tipo: domain.ConfianzaCADesconocida, Huella: strings.Repeat("a", 64)},
	}

	t.Run("sin registro (móvil, otros canales)", func(t *testing.T) {
		claves := clavesVeriFactu(vfCheckSignatureConContexto(context.Background(), firmado, n, time.Now()))
		if claves["verifactu.signature_sha1"] != "error" || len(claves) != 1 {
			t.Fatalf("se esperaba solo verifactu.signature_sha1 como error: %v", claves)
		}
		resultado := ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{"r.xml": firmado})
		if resultado.Valid {
			t.Fatal("un registro con SHA-1 no puede salir válido sin aceptarlo")
		}
	})

	t.Run("CLI o escritorio sin aceptar: se puede aceptar", func(t *testing.T) {
		registro := domain.NuevoRegistroConfianzaForzada(domain.ConfianzaForzada{Aceptaciones: otras})
		ctx := domain.ConRegistroConfianzaForzada(context.Background(), registro)
		claves := clavesVeriFactu(vfCheckSignatureConContexto(ctx, firmado, n, time.Now()))
		if claves["verifactu.signature_sha1_acceptable"] != "error" || len(claves) != 1 {
			t.Fatalf("se esperaba verifactu.signature_sha1_acceptable como error: %v", claves)
		}
		if len(registro.Aplicadas()) != 0 {
			t.Fatalf("aceptar otros casos no puede aplicar nada en Veri*Factu: %v", registro.Aplicadas())
		}
		obstaculos := registro.Obstaculos()
		if len(obstaculos) != 1 || obstaculos[0].Tipo != domain.ConfianzaAlgoritmoSHA1 {
			t.Fatalf("el obstáculo ofrecido debe ser solo SHA-1: %v", obstaculos)
		}
	})

	t.Run("SHA-1 aceptado: aviso visible y evidencia para la auditoría", func(t *testing.T) {
		registro := domain.NuevoRegistroConfianzaForzada(domain.ConfianzaForzada{Aceptaciones: append([]domain.AceptacionConfianza{sha1}, otras...)})
		ctx := domain.ConRegistroConfianzaForzada(context.Background(), registro)
		claves := clavesVeriFactu(vfCheckSignatureConContexto(ctx, firmado, n, time.Now()))
		if claves["verifactu.signature_sha1_accepted"] != "warning" || claves["verifactu.trust"] != "warning" || len(claves) != 2 {
			t.Fatalf("se esperaba el aviso de SHA-1 aceptado: %v", claves)
		}
		aplicadas := registro.Aplicadas()
		if len(aplicadas) != 1 || aplicadas[0].Tipo != domain.ConfianzaAlgoritmoSHA1 {
			t.Fatalf("solo SHA-1 se aplica en Veri*Factu: %v", aplicadas)
		}
		resultado := ValidarRegistrosVeriFactu(ctx, map[string][]byte{"r.xml": firmado})
		if !resultado.Valid || resultado.Warnings == 0 {
			t.Fatalf("con SHA-1 aceptado el registro sale válido con aviso: %+v", resultado)
		}
		resultado.Localize(func(k string) string { return k })
		if !strings.Contains(resultado.Report, "verifactu.signature_sha1_accepted") {
			t.Fatalf("el informe no muestra el aviso: %s", resultado.Report)
		}
	})

	t.Run("la política lo prohíbe", func(t *testing.T) {
		registro := domain.NuevoRegistroConfianzaForzada(domain.ConfianzaForzada{Prohibida: true, Aceptaciones: []domain.AceptacionConfianza{sha1}})
		ctx := domain.ConRegistroConfianzaForzada(context.Background(), registro)
		claves := clavesVeriFactu(vfCheckSignatureConContexto(ctx, firmado, n, time.Now()))
		if claves["verifactu.signature_sha1"] != "error" || len(claves) != 1 {
			t.Fatalf("con la política no se acepta ni se ofrece: %v", claves)
		}
	})

	t.Run("nunca: registro alterado con SHA-1 aceptado", func(t *testing.T) {
		alterado := bytes.Replace(firmado, []byte("123.45"), []byte("123.46"), 1)
		na, err := vfParse(alterado)
		if err != nil {
			t.Fatal(err)
		}
		registro := domain.NuevoRegistroConfianzaForzada(domain.ConfianzaForzada{Aceptaciones: []domain.AceptacionConfianza{sha1}})
		ctx := domain.ConRegistroConfianzaForzada(context.Background(), registro)
		claves := clavesVeriFactu(vfCheckSignatureConContexto(ctx, alterado, na, time.Now()))
		if claves["verifactu.signature"] != "error" || len(claves) != 1 {
			t.Fatalf("un registro alterado nunca se acepta: %v", claves)
		}
	})

	t.Run("nunca: certificado no vigente en la fecha del registro", func(t *testing.T) {
		registro := domain.NuevoRegistroConfianzaForzada(domain.ConfianzaForzada{Aceptaciones: []domain.AceptacionConfianza{sha1}})
		ctx := domain.ConRegistroConfianzaForzada(context.Background(), registro)
		claves := clavesVeriFactu(vfCheckSignatureConContexto(ctx, firmado, n, cert.NotAfter.Add(time.Hour)))
		if claves["verifactu.certificate"] != "error" || len(claves) != 1 {
			t.Fatalf("un certificado fuera de fecha nunca se acepta: %v", claves)
		}
	})

	t.Run("los registros con SHA-256 no cambian", func(t *testing.T) {
		doc, _ := domain.NewDocument("record.xml", data, "application/xml")
		job := domain.SignatureJob{Document: doc, Format: FormatVeriFactu, Action: domain.ActionSign}
		signed, err := NewVeriFactuSigner().Sign(context.Background(), job, &LocalSigningKey{ID: "key", Signer: priv, Certificate: cert})
		if err != nil {
			t.Fatal(err)
		}
		ns, _ := vfParse(signed.Data)
		registro := domain.NuevoRegistroConfianzaForzada(domain.ConfianzaForzada{Aceptaciones: []domain.AceptacionConfianza{sha1}})
		ctx := domain.ConRegistroConfianzaForzada(context.Background(), registro)
		claves := clavesVeriFactu(vfCheckSignatureConContexto(ctx, signed.Data, ns, time.Now()))
		if claves["verifactu.trust"] != "warning" || len(claves) != 1 || len(registro.Aplicadas()) != 0 {
			t.Fatalf("un registro con SHA-256 no debe tocar lo aceptado: %v %v", claves, registro.Aplicadas())
		}
	})
}
