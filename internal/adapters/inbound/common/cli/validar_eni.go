// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package cli

import (
	"fmt"
	"grxfirma/internal/adapters/outbound/common/eni"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"strings"
)

func (a *Adaptador) ejecutarValidarENI(cfg configCLI) int {
	loc := a.Localizador
	if loc == nil {
		loc = localizador.Detectar()
	}
	if strings.TrimSpace(cfg.entrada) == "" {
		fmt.Fprintln(a.Stderr, loc.T("eni.validacion.input"))
		return 1
	}
	data, err := securefile.ReadFileLimit(cfg.entrada, eni.MaxXMLBytes)
	if err != nil {
		a.escribirErrorCLI(loc.T("paridad.lote3.eni.title"), err)
		return 1
	}
	issues := eni.ValidarXML(data)
	for _, p := range issues {
		fmt.Fprintf(a.Stdout, "%s: %s\n", p.Campo, loc.T(p.Clave))
	}
	if len(issues) > 0 {
		return 1
	}
	fmt.Fprintln(a.Stdout, loc.T("eni.validacion.valid"))
	return 0
}
