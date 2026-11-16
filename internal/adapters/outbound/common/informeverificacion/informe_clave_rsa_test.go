// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package informeverificacion

import (
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

const veredictoRojo = `<div class="veredicto error">`

// resultadoClaveRSACorta reproduce lo que deja el motor (signer/cades_verify.go)
// para un firmante con clave RSA de 1024 bits sin la excepción aceptada:
// integridad válida y certificado inválido con el detalle rsa_bits.
func resultadoClaveRSACorta() domain.VerificationResult {
	const motivo = "clave RSA del firmante de menos de 2048 bits"
	res := domain.NewVerificationSuccess("CAdES", "firma CAdES valida", nil)
	res.Valid = false
	res.Reason = motivo
	res.Warnings = []string{motivo}
	res.Certificate = domain.VerificationAspect{
		Status:  domain.VerificationStatusInvalid,
		Reason:  motivo,
		Details: []string{"subject=Firmante sintético", "rsa_bits=1024"},
	}
	res.Trust = domain.VerificationAspect{Status: domain.VerificationStatusValid}
	return res
}

// Orden de Alberto (16/11/2026): la clave RSA corta sin excepción sale en
// rojo, como firma no válida, aunque el documento esté íntegro.
func TestHTML_ClaveRSACortaEsFirmaNoValida(t *testing.T) {
	for idioma, esperado := range map[string]string{"es": "FIRMA NO VÁLIDA", "en": "INVALID SIGNATURE"} {
		html := informe(t, resultadoClaveRSACorta(), idioma)
		if !strings.Contains(html, veredictoRojo+esperado+"</div>") {
			t.Errorf("%s: la clave RSA corta no sale como firma no válida en rojo", idioma)
		}
		if strings.Contains(html, "VALIDEZ DEL CERTIFICADO NO ACREDITADA") {
			t.Errorf("%s: la clave RSA corta no puede salir como «íntegra sin acreditar»", idioma)
		}
	}
}

// Con SHA-1 además de la clave corta manda la clave: sigue en rojo.
func TestHTML_ClaveRSACortaYSHA1SigueEnRojo(t *testing.T) {
	res := resultadoClaveRSACorta()
	res.Evidence = append(res.Evidence, domain.VerificationEvidence{Type: domain.EvidenciaAlgoritmoAntiguo, Summary: "algoritmo_antiguo=sha1"})
	if html := informe(t, res, "es"); !strings.Contains(html, veredictoRojo+"FIRMA NO VÁLIDA</div>") {
		t.Fatal("la clave RSA corta con SHA-1 debe salir como firma no válida")
	}
}

// Con la excepción clave_rsa_corta aceptada, el informe no la pinta en rojo.
func TestHTML_ClaveRSACortaConExcepcionNoEsRoja(t *testing.T) {
	res := resultadoClaveRSACorta()
	res.Valid = true
	res.Reason = ""
	res.Certificate.Status = domain.VerificationStatusValid
	res.Certificate.Reason = ""
	res = res.MarcarConfianzaForzada(domain.ObstaculoConfianza{Tipo: domain.ConfianzaClaveRSACorta, Huella: strings.Repeat("b", 64)})
	html := informe(t, res, "es")
	if strings.Contains(html, veredictoRojo) {
		t.Fatal("con la excepción aceptada la clave corta no debe salir en rojo")
	}
	if !strings.Contains(html, "FIRMA VÁLIDA CON ADVERTENCIAS") {
		t.Fatal("con la excepción aceptada debe salir como válida con advertencias")
	}
}

// El certificado caducado y el emisor desconocido siguen en ámbar aunque el
// certificado figure como inválido: solo la clave corta pasa a rojo.
func TestHTML_CaducadoYEmisorDesconocidoSiguenEnAmbar(t *testing.T) {
	caducado := domain.NewVerificationSuccess("CAdES", "firma CAdES valida", nil)
	caducado.Valid = false
	caducado.Certificate = domain.VerificationAspect{
		Status:  domain.VerificationStatusInvalid,
		Reason:  "certificado caducado: Firmante sintético",
		Details: []string{"not_after=2020-01-01T00:00:00Z", "rsa_bits=4096"},
	}
	desconocido := domain.NewVerificationSuccess("CAdES", "firma CAdES valida", nil)
	desconocido.Valid = false
	desconocido.Trust = domain.VerificationAspect{Status: domain.VerificationStatusInvalid, Reason: "emisor no reconocido"}
	for nombre, res := range map[string]domain.VerificationResult{"caducado": caducado, "emisor desconocido": desconocido} {
		html := informe(t, res, "es")
		if strings.Contains(html, veredictoRojo) {
			t.Errorf("%s: no debe salir en rojo", nombre)
		}
		if !strings.Contains(html, `<div class="veredicto aviso">FIRMA ÍNTEGRA · VALIDEZ DEL CERTIFICADO NO ACREDITADA</div>`) {
			t.Errorf("%s: debe seguir como «íntegra, validez no acreditada» en ámbar", nombre)
		}
	}
}
