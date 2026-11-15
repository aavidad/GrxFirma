// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"encoding/json"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/security/excepciones"
	"os"
	"slices"
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

type proteccionFirmadaCLIConAvisos struct{ avisos []string }

func (m proteccionFirmadaCLIConAvisos) Execute(ctx context.Context, cmd application.ProtectAndSignCommand) (application.ProtectAndSignResult, error) {
	res, err := (&protectAndSignCLIMock{}).Execute(ctx, cmd)
	res.Warnings = m.avisos
	return res, err
}

func TestProteccionFirmadaCLI_AvisosVisiblesYJSON(t *testing.T) {
	const aviso = "excepciones.cms_rsa_v15_crear.aviso"
	for _, modoJSON := range []bool{false, true} {
		loc := localizador.Para("es")
		var out, errOut strings.Builder
		a := New(nil, nil).WithLocalizador(loc).WithProteccionFirmada(proteccionFirmadaCLIConAvisos{avisos: []string{aviso}})
		a.Stdout, a.Stderr = &out, &errOut
		a.LeerFichero = func(string) ([]byte, error) { return []byte("contenido"), nil }
		args := []string{"-operacion", "proteger-firmando", "-entrada", "acta.txt", "-destinatario", "dest-1", "-id-certificado", "cert-1", "-perfil-proteccion", "compat", "-no-guardar"}
		if modoJSON {
			args = append(args, "-json")
		}
		if rc := a.Run(context.Background(), args); rc != 0 {
			t.Fatalf("json=%v rc=%d stderr=%s", modoJSON, rc, errOut.String())
		}
		if !strings.Contains(errOut.String(), loc.T(aviso)) {
			t.Fatalf("json=%v sin aviso visible: %s", modoJSON, errOut.String())
		}
		if modoJSON {
			var campos struct {
				Advertencias []string `json:"advertencias"`
			}
			if err := json.Unmarshal([]byte(out.String()), &campos); err != nil {
				t.Fatal(err)
			}
			if len(campos.Advertencias) != 1 || campos.Advertencias[0] != aviso {
				t.Fatalf("JSON sin clave de aviso: %s", out.String())
			}
		}
	}
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

func TestV3CotejoENIC14NDevuelveAviso(t *testing.T) {
	a := New(nil, nil)
	avisos, err := a.resolverCotejoENI(commonsigner.ErrFirmaCorrespondeConC14NAntigua)
	if err != nil || !slices.Contains(avisos, excepciones.Warning("xml_c14n_antigua")) {
		t.Fatalf("aviso C14N perdido: %v %v", avisos, err)
	}
}
