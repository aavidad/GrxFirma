// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type proteccionCLIConAvisos struct{ avisos []string }

func (m proteccionCLIConAvisos) Execute(ctx context.Context, cmd application.ProtectCommand) (application.ProtectResult, error) {
	res, err := (&protectCLIMock{}).Execute(ctx, cmd)
	res.Warnings = m.avisos
	return res, err
}

type exportacionCLIConAvisos struct{ avisos []string }

func (m exportacionCLIConAvisos) Execute(context.Context, application.ExportProtectionRecipientCommand) (application.ExportProtectionRecipientResult, error) {
	return application.ExportProtectionRecipientResult{Recipient: domain.ProtectionRecipient{ID: "propio"}, Data: []byte("{}"), Warnings: m.avisos}, nil
}

func TestProteccionCLI_AvisosVisiblesYJSONOpcional(t *testing.T) {
	for _, operacion := range []string{"proteger", "exportar-destinatario-proteccion"} {
		for _, modoJSON := range []bool{false, true} {
			for _, conAvisos := range []bool{false, true} {
				var avisos []string
				if conAvisos {
					avisos = []string{protector.AvisoProteccionAltoSinAlmacen}
				}
				loc := localizador.Para("en")
				var out, errOut strings.Builder
				a := New(nil, nil).WithLocalizador(loc).WithProteccion(proteccionCLIConAvisos{avisos}, nil, nil).WithIntercambioProteccion(exportacionCLIConAvisos{avisos}, nil)
				a.Stdout, a.Stderr = &out, &errOut
				a.LeerFichero = func(string) ([]byte, error) { return []byte("contenido"), nil }
				a.Escribir = func(string, []byte, os.FileMode) error { return nil }
				args := []string{"-operacion", operacion, "-entrada", "acta.txt", "-destinatario", "propio", "-salida", "destinatario.json"}
				if operacion == "proteger" {
					args = append(args, "-no-guardar")
				}
				if modoJSON {
					args = append(args, "-json")
				}
				if rc := a.Run(context.Background(), args); rc != 0 {
					t.Fatalf("%s: rc=%d stderr=%s", operacion, rc, errOut.String())
				}
				if strings.Contains(errOut.String(), loc.T(protector.AvisoProteccionAltoSinAlmacen)) != conAvisos {
					t.Fatalf("avisos CLI (%s): %s", operacion, errOut.String())
				}
				if modoJSON {
					var campos map[string]json.RawMessage
					if err := json.Unmarshal([]byte(out.String()), &campos); err != nil {
						t.Fatal(err)
					}
					if _, present := campos["advertencias"]; present != conAvisos {
						t.Fatalf("advertencias presente=%v: %s", present, out.String())
					}
				}
			}
		}
	}
}
