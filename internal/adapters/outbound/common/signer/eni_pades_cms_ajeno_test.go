// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"errors"
	"testing"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
)

// KD-05 (a): el cotejo previo al ENI con PAdES usa el mismo verificador que
// E2-01 arregla. Un PDF cuyo /Contents es un CAdES implícito válido de la
// víctima sobre otros datos no puede envolverse como ENI «PAdES» suyo.
// Depende de E2-01 (814a342): solo pasa con ese arreglo fusionado.
func TestComprobarIntegridadPAdESRechazaCMSImplicitoAjeno(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	firmado, err := desktopsigner.NuevoMotorFirmaGo(nil).Sign(context.Background(), domain.SignatureJob{
		Document: documentoPDFRealPrueba(t), Format: domain.FormatPAdES, Action: domain.ActionSign,
	}, desktopsigner.NuevaClaveLocal(priv, cert))
	if err != nil {
		t.Fatalf("firmar PDF: %v", err)
	}
	if err := commonsigner.ComprobarIntegridadPAdES(context.Background(), firmado.Data); err != nil {
		t.Fatalf("el PDF firmado intacto debe cotejar: %v", err)
	}
	ajeno := firmaCAdESImplicitaPrueba(t, priv, cert, []byte("Autorizo el pago de 10 EUR"))
	atacado := sustituirContentsPDF(t, firmado.Data, ajeno)
	if err := commonsigner.ComprobarIntegridadPAdES(context.Background(), atacado); !errors.Is(err, commonsigner.ErrFirmaNoCorrespondeOriginal) {
		t.Fatalf("un CMS implícito ajeno al PDF no puede cotejar para el ENI: %v", err)
	}
}
