// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/asiccontainer"
	"grxfirma/internal/domain"
)

func TestASiCXAdES_SignAndVerify(t *testing.T) {
	priv, cert := certForTest(t, "ASiC-Test")
	engine := NewASiCXAdESSigner()
	cases := []struct {
		name                string
		documentName        string
		expectedPayloadName string
		mimeType            string
		payload             []byte
		expectXMLTransform  bool
	}{
		{
			name:                "xml_con_bom_declaracion_saltos_y_namespaces",
			documentName:        "payload.xml",
			expectedPayloadName: "payload.xml",
			mimeType:            "application/xml",
			payload: []byte("\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
				"<raiz xmlns=\"urn:grxfirma:qa\" xmlns:a=\"urn:grxfirma:atributos\" a:estado=\"ok\">\n" +
				"  <dato>hola</dato>\n" +
				"</raiz>\n"),
			expectXMLTransform: true,
		},
		{
			name:                "binario",
			documentName:        "payload.bin",
			expectedPayloadName: "payload.bin",
			mimeType:            "application/octet-stream",
			payload:             []byte{0x00, 0x01, 0x02, 0xff, '<', 'x', 0x00, '\n'},
			expectXMLTransform:  false,
		},
		{
			name:                "metadatos_xml_con_contenido_malformado",
			documentName:        "payload.xml",
			expectedPayloadName: "payload.xml",
			mimeType:            "application/xml",
			payload:             []byte("<?xml version=\"1.0\"?><raiz><sin-cierre>"),
			expectXMLTransform:  false,
		},
		{
			name:                "ruta_windows",
			documentName:        `C:\documentos\payload-windows.xml`,
			expectedPayloadName: "payload-windows.xml",
			mimeType:            "application/xml",
			payload:             []byte(`<?xml version="1.0"?><raiz>windows</raiz>`),
			expectXMLTransform:  true,
		},
		{
			name:                "nombre_reservado",
			documentName:        "mimetype",
			expectedPayloadName: "dataobject.bin",
			mimeType:            "application/octet-stream",
			payload:             []byte{0x00, 0xff, 0x01},
			expectXMLTransform:  false,
		},
		{
			name:                "nombre_con_caracteres_xml",
			documentName:        `factura & "anexo".xml`,
			expectedPayloadName: `factura & "anexo".xml`,
			mimeType:            "application/xml",
			payload:             []byte(`<?xml version="1.0"?><raiz>anexo</raiz>`),
			expectXMLTransform:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := domain.SignatureJob{
				Document: domain.Document{Name: tc.documentName, MIMEType: tc.mimeType, Content: tc.payload},
				Format:   formatASiCXAdES,
				Action:   domain.ActionSign,
			}
			result, err := engine.Sign(context.Background(), job, &LocalSigningKey{Signer: priv, Certificate: cert})
			if err != nil {
				t.Fatalf("Sign(ASiC-XAdES) error = %v", err)
			}
			containerReader, err := zip.NewReader(
				bytes.NewReader(result.Data),
				int64(len(result.Data)),
			)
			if err != nil {
				t.Fatalf("zip.NewReader(ASiC-XAdES) error = %v", err)
			}
			if len(containerReader.File) == 0 ||
				containerReader.File[0].Name != asiccontainer.EntryMIMEType ||
				containerReader.File[0].Method != zip.Store {
				t.Fatal("ASiC-XAdES no almacena mimetype como primera entrada sin compresión")
			}
			signatureXML, err := asiccontainer.ExtractXAdESSignature(result.Data)
			if err != nil {
				t.Fatalf("ExtractXAdESSignature() error = %v", err)
			}
			payload, payloadName, err := asiccontainer.ExtractData(result.Data)
			if err != nil {
				t.Fatalf("ExtractData() error = %v", err)
			}
			if payloadName != tc.expectedPayloadName || !bytes.Equal(payload, tc.payload) {
				t.Fatalf("payload ASiC inesperado: nombre=%q datos=%x", payloadName, payload)
			}
			expectedReference := []byte(`URI="` + escapeXMLAttr(tc.expectedPayloadName) + `"`)
			if !bytes.Contains(signatureXML, expectedReference) {
				t.Fatalf("la firma no referencia el payload esperado: %s", expectedReference)
			}
			signedInfoXML, _, _, err := extractXMLSignatureCore(signatureXML)
			if err != nil {
				t.Fatal(err)
			}
			var signedInfo signedInfoForVerify
			if err := xml.Unmarshal(signedInfoXML, &signedInfo); err != nil {
				t.Fatal(err)
			}
			if len(signedInfo.References) != 3 || signedInfo.References[0].URI != tc.expectedPayloadName {
				t.Fatal("referencia del payload ASiC ausente o inesperada")
			}
			documentTransforms := signedInfo.References[0].Transforms
			hasDocumentTransform := len(documentTransforms) == 1 && documentTransforms[0].Algorithm == algExcC14N
			if hasDocumentTransform != tc.expectXMLTransform {
				t.Fatalf("transformación XML anunciada = %v, esperaba %v", hasDocumentTransform, tc.expectXMLTransform)
			}
			if !tc.expectXMLTransform && len(documentTransforms) != 0 {
				t.Fatal("la referencia al payload binario no debe contener ds:Transforms")
			}

			verify, signers, err := NewASiCXAdESVerifier().Verify(context.Background(), domain.Document{Name: "payload.asics", MIMEType: "application/vnd.etsi.asic-s+zip", Content: result.Data}, domain.CertificateChain{})
			if err != nil {
				t.Fatalf("Verify(ASiC-XAdES) error = %v", err)
			}
			if !verify.Valid || len(signers) != 1 {
				t.Fatalf("resultado inesperado: %+v signers=%d", verify, len(signers))
			}

			tamperedPayload := append([]byte(nil), tc.payload...)
			tamperedPayload[len(tamperedPayload)-1] ^= 0x01
			tampered, err := asiccontainer.CreateXAdESContainer(signatureXML, tamperedPayload, tc.documentName)
			if err != nil {
				t.Fatalf("CreateXAdESContainer(manipulado) error = %v", err)
			}
			if _, _, err := NewASiCXAdESVerifier().Verify(context.Background(), domain.Document{Name: "manipulado.asics", MIMEType: "application/vnd.etsi.asic-s+zip", Content: tampered}, domain.CertificateChain{}); err == nil {
				t.Fatal("Verify(ASiC-XAdES manipulado) debería rechazar el digest")
			}
		})
	}
}

// La muestra se localiza con GRXFIRMA_V19_SOURCE; antes habia aqui una ruta
// absoluta al equipo de desarrollo.
func TestASiCXAdESVerifier_AcceptsOfficialV19Sample(t *testing.T) {
	raiz := os.Getenv("GRXFIRMA_V19_SOURCE")
	if strings.TrimSpace(raiz) == "" {
		t.Skip("defina GRXFIRMA_V19_SOURCE con la raiz de clienteafirma-1.9-oficial")
	}
	data, err := os.ReadFile(filepath.Join(raiz, "afirma-crypto-xades", "src", "test", "resources", "ASIC-XAdES-2017789087353489524.zip"))
	if err != nil {
		t.Skipf("muestra oficial ASiC-XAdES no disponible: %v", err)
	}
	verify, signers, err := NewASiCXAdESVerifier().Verify(context.Background(), domain.Document{Name: "muestra.asics", MIMEType: "application/vnd.etsi.asic-s+zip", Content: data}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify(muestra oficial ASiC-XAdES) error = %v", err)
	}
	// El certificado de la muestra expiró en 2016. Su vigencia no debe
	// ocultar que la firma matemática y sus referencias sí son válidas.
	if verify.Integrity.Status != domain.VerificationStatusValid || len(signers) == 0 {
		t.Fatalf("resultado inesperado: %+v signers=%d", verify, len(signers))
	}
}
