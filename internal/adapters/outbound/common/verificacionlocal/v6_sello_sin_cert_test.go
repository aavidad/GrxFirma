// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal_test

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/asn1"
	"fmt"
	"testing"
	"time"

	"github.com/digitorus/timestamp"
	"grxfirma/internal/adapters/outbound/common/verificacionlocal"
	"grxfirma/internal/domain"
)

func TestV6SelloDocumentoSinCertificadoTSA(t *testing.T) {
	e := nuevoEscenario(t)
	contenido := []byte("documento")
	digest := sha256.Sum256(contenido)
	sello := timestamp.Timestamp{HashAlgorithm: crypto.SHA256, HashedMessage: digest[:], Time: time.Now().Add(-time.Minute).UTC(),
		SerialNumber: nuevaSerie(), Policy: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}, AddTSACertificate: false}
	resp, err := sello.CreateResponseWithOpts(e.pki.tsa.cert, e.pki.tsa.clave, crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := timestamp.ParseResponse(resp)
	if err != nil {
		t.Logf("ParseResponse sin cert: %v (se usa el token en bruto)", err)
	}
	var token []byte
	if parsed != nil {
		token = parsed.RawToken
	} else {
		var r struct {
			Status asn1.RawValue
			Token  asn1.RawValue
		}
		if _, err := asn1.Unmarshal(resp, &r); err != nil {
			t.Fatal(err)
		}
		token = r.Token.FullBytes
	}
	anchors := domain.CertificateChain{DERCertificates: [][]byte{e.pki.raiz.cert.Raw}}
	ev := verificacionlocal.Nuevo(verificacionlocal.Configuracion{})
	var panico any
	func() {
		defer func() { panico = recover() }()
		_, integ := ev.EvaluarSelloDocumento(context.Background(), token, contenido, anchors, time.Now())
		if integ.Estado != domain.IntegridadNoValida {
			t.Fatalf("sello sin certificado: %+v", integ)
		}
	}()
	if panico != nil {
		t.Errorf("PANICO con sello sin certificado TSA: %s", fmt.Sprint(panico))
	}
}

type tsaSinCertV6 struct{ emisor entidad }

func (s tsaSinCertV6) RequestTimestamp(_ context.Context, resumen []byte, algoritmo crypto.Hash) ([]byte, error) {
	sello := timestamp.Timestamp{HashAlgorithm: algoritmo, HashedMessage: append([]byte(nil), resumen...), Time: time.Now().Add(-time.Minute).UTC(),
		SerialNumber: nuevaSerie(), Policy: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}, AddTSACertificate: false}
	resp, err := sello.CreateResponseWithOpts(s.emisor.cert, s.emisor.clave, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	var r struct {
		Status asn1.RawValue
		Token  asn1.RawValue
	}
	if _, err := asn1.Unmarshal(resp, &r); err != nil {
		return nil, err
	}
	return r.Token.FullBytes, nil
}

func TestV6CAdESTSelloSinCertificadoEnVerify(t *testing.T) {
	e := nuevoEscenario(t)
	var firmado []byte
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("no se pudo preparar el sello sin certificado: %v", p)
			}
		}()
		firmado = e.firmarCAdES(t, tsaSinCertV6{emisor: e.pki.tsa})
	}()
	var panico any
	func() {
		defer func() { panico = recover() }()
		out := verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: e.crlDir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
		if out.Dictamen.SelloTiempo.Estado != domain.SelloNoValido {
			t.Fatalf("sello sin certificado: %+v", out.Dictamen.SelloTiempo)
		}
	}()
	if panico != nil {
		t.Errorf("PANICO en /verify con CAdES-T cuyo sello no trae el certificado TSA: %v", panico)
	}
}
