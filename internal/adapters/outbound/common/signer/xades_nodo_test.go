// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/eni"
	"grxfirma/internal/domain"
)

// Expediente ENI con el índice firmado por FirmarNodoXAdES: la firma,
// insertada dentro de enids:firmas, debe verificarse sobre el expediente.
func TestFirmarNodoXAdES_IndiceDeExpedienteENI(t *testing.T) {
	priv, cert := certForTest(t, "Expediente ENI")
	clave := &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert}
	ahora := time.Now().UTC().Truncate(time.Second)
	var docs []eni.DocumentoExpediente
	for _, contenido := range []string{"%PDF-1.7 uno", "%PDF-1.7 dos"} {
		x, err := eni.Generar(eni.Documento{
			Contenido: []byte(contenido), NombreFormato: "PDF",
			Metadatos: eni.Metadatos{Organos: []string{"L01180877"}, EstadoElaboracion: "EE01", TipoDocumental: "TD99"},
			Firmas:    []eni.Firma{{Tipo: eni.FirmaPAdES}},
		}, ahora)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, eni.DocumentoExpediente{XML: x})
	}
	firmar := func(nodo []byte, id string) ([]byte, error) { return FirmarNodoXAdES(nodo, id, clave, nil) }
	exp, err := eni.GenerarExpediente(eni.MetadatosExpediente{
		Organos: []string{"L01180877"}, Clasificacion: "L01180877_PRO_SUBVENCIONES", Estado: "E02", Interesados: []string{"12345678Z"},
	}, docs, firmar, CanonicalizarExclusivo, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("GRXFIRMA_ENI_SALIDA"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "expediente.xml"), exp, 0o600)
		for i, d := range docs {
			_ = os.WriteFile(filepath.Join(dir, "expediente-doc"+string(rune('1'+i))+".xml"), d.XML, 0o600)
		}
	}
	verificar := func(data []byte) domain.VerificationResult {
		doc, _ := domain.NewDocument("expediente.xml", data, "application/xml")
		res, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
		if err != nil {
			return domain.NewVerificationFailure("XAdES", err.Error(), nil)
		}
		return res
	}
	if res := verificar(exp); res.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("la firma del índice debe verificarse: %+v", res.Integrity)
	}
	// Cambiar la huella de un documento en el índice invalida la firma.
	manipulado := bytes.Replace(exp, []byte("<eniconexpind:OrdenDocumentoExpediente>2<"), []byte("<eniconexpind:OrdenDocumentoExpediente>3<"), 1)
	if res := verificar(manipulado); res.Integrity.Status == domain.VerificationStatusValid {
		t.Fatal("un índice manipulado no puede verificarse como íntegro")
	}
}
