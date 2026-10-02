// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/pdffixture"
	"grxfirma/internal/testsupport/pdfsigtest"
)

func TestTraducirSelloVisibleJava(t *testing.T) {
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(0xABC),
		Subject: pkix.Name{CommonName: "PRUEBAS EIDAS", Names: []pkix.AttributeTypeAndValue{
			{Type: []int{2, 5, 4, 3}, Value: "PRUEBAS EIDAS"}, {Type: []int{2, 5, 4, 5}, Value: "IDCES-99999999R"},
		}},
		Issuer: pkix.Name{CommonName: "AC FNMT Usuarios"},
	}
	fecha := time.Date(2026, 9, 26, 10, 5, 0, 0, time.UTC)
	opts, err := traducirSelloVisibleJava(map[string]string{
		"signaturePositionOnPageLowerLeftX": "50", "signaturePositionOnPageLowerLeftY": "60",
		"signaturePositionOnPageUpperRightX": "250", "signaturePositionOnPageUpperRightY": "140",
		"signaturePage": "-1", "signReason": "Aprobación",
		"layer2Text": "Firmado por $$SUBJECTCN$$ ($$SUBJECTFIELD=SERIALNUMBER$$)\nel $$SIGNDATE=dd/MM/yyyy$$ - $$REASON$$ - $$CERTSERIAL$$",
	}, pdffixture.Minimal(), cert, fecha)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"visibleSeal": "true", "visibleSealRectX": "50", "visibleSealRectY": "60",
		"visibleSealRectW": "200", "visibleSealRectH": "80", "page": "1",
		"visibleSealText": "Firmado por PRUEBAS EIDAS (IDCES-99999999R)\nel 26/09/2026 - Aprobación - ABC",
	} {
		if opts[k] != v {
			t.Fatalf("%s = %q, want %q", k, opts[k], v)
		}
	}
	sinJava := map[string]string{"visibleSeal": "false"}
	if got, _ := traducirSelloVisibleJava(sinJava, nil, cert, fecha); got["visibleSeal"] != "false" {
		t.Fatal("sin parámetros de Java no debe cambiar nada")
	}
	if _, err := traducirSelloVisibleJava(map[string]string{
		"signaturePositionOnPageLowerLeftX": "300", "signaturePositionOnPageLowerLeftY": "60",
		"signaturePositionOnPageUpperRightX": "250", "signaturePositionOnPageUpperRightY": "140",
	}, nil, cert, fecha); err == nil {
		t.Fatal("un área invertida debe rechazarse")
	}
}

func TestMotorFirmaGo_PAdESVisibleConParametrosJava(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Firmante Visible"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	res, err := NuevoMotorFirmaGo(nil).Sign(t.Context(), domain.SignatureJob{
		Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign,
		Options: map[string]string{
			"signaturePositionOnPageLowerLeftX": "50", "signaturePositionOnPageLowerLeftY": "60",
			"signaturePositionOnPageUpperRightX": "250", "signaturePositionOnPageUpperRightY": "140",
			"signaturePage": "-1", "layer2Text": "Firmado por $$SUBJECTCN$$",
		},
	}, NuevaClaveLocal(priv, cert))
	if err != nil {
		t.Fatalf("PAdES visible: %v", err)
	}
	if !strings.Contains(string(res.Data), "/Rect [50") && !strings.Contains(string(res.Data), "/Rect[50") {
		t.Fatal("la anotación de firma no está en la posición pedida por la web")
	}
	if _, err := exec.LookPath("pdfsig"); err == nil {
		t.Run("pdfsig", func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "v.pdf")
			_ = os.WriteFile(f, res.Data, 0o600)
			out, _ := pdfsigtest.Command(t, f).CombinedOutput()
			if !strings.Contains(string(out), "Signature is Valid") {
				t.Fatalf("pdfsig no valida la firma visible:\n%s", out)
			}
		})
	}
}

func TestEstiloTextoDesdeOpciones(t *testing.T) {
	e := estiloTextoDesdeOpciones(map[string]string{"visibleSealText": "Hola", "layer2FontSize": "14", "layer2FontColor": "red"})
	if e.texto != "Hola" || e.tamPuntos != 14 || e.color == nil || e.color.R != 255 || e.color.G != 0 {
		t.Fatalf("estilo = %+v", e)
	}
	if c, ok := colorJava("#102030"); !ok || c.R != 0x10 || c.B != 0x30 {
		t.Fatalf("color hex = %+v %v", c, ok)
	}
	if _, ok := colorJava("verde-raro"); ok {
		t.Fatal("un color desconocido no debe aceptarse")
	}
	img, err := generarImagenSelloPAdES(testInfoPAdES(), 200, 80, true, "", "", nil, e)
	if err != nil || len(img) == 0 {
		t.Fatalf("sello con estilo: %v", err)
	}
}

func TestPosicionSelloElegida_CalculaSobreLaPagina(t *testing.T) {
	pdfData := pdffixture.Minimal()
	casos := map[string][4]float64{
		"inferior-derecha":   {595 - 36 - 200, 36, 595 - 36, 36 + 70},
		"superior-izquierda": {36, 842 - 36 - 70, 36 + 200, 842 - 36},
	}
	for pos, esperado := range casos {
		out, err := posicionSelloElegida(map[string]string{domain.OpcionPaginaSello: "-1"}, pos, pdfData)
		if err != nil {
			t.Fatal(err)
		}
		for i, clave := range javaSelloClaves {
			v, _ := strconv.ParseFloat(out[clave], 64)
			if math.Abs(v-esperado[i]) > 0.01 {
				t.Errorf("%s %s = %v, se esperaba %v", pos, clave, v, esperado[i])
			}
		}
		if out["signaturePage"] != "1" {
			t.Errorf("la última página de un PDF de una página es la 1: %q", out["signaturePage"])
		}
	}
	if _, err := posicionSelloElegida(nil, "en-medio", pdfData); err == nil {
		t.Error("una posición no ofrecida debe rechazarse")
	}
	if _, err := posicionSelloElegida(map[string]string{domain.OpcionPaginaSello: "3"}, "inferior-centro", pdfData); err == nil {
		t.Error("una página fuera del documento debe rechazarse")
	}
}
