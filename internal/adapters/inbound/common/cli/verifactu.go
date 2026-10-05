// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/adapters/outbound/common/signer"
)

func (a *Adaptador) ejecutarVeriFactu(ctx context.Context, cfg configCLI) int {
	loc := a.localizadorENI()
	if strings.TrimSpace(cfg.entrada) == "" {
		fmt.Fprintln(a.Stderr, loc.T("verifactu.input"))
		return 1
	}
	if cfg.operacion == "validar-verifactu" {
		result, e := signer.ValidarRutaVeriFactu(ctx, cfg.entrada)
		if e != nil {
			fmt.Fprintln(a.Stderr, signer.TraducirErrorVeriFactu(e, func(k string) string { return loc.T(k) }))
			return 1
		}
		result.Localize(func(k string) string { return loc.T(k) })
		if cfg.json {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Fprintln(a.Stdout, string(data))
		} else {
			fmt.Fprint(a.Stdout, result.Report)
		}
		if cfg.salida != "" {
			input, _ := filepath.Abs(cfg.entrada)
			output, _ := filepath.Abs(cfg.salida)
			same := input == output
			if a, err := os.Stat(input); err == nil {
				if b, err := os.Stat(output); err == nil {
					same = same || os.SameFile(a, b)
				}
			}
			for _, record := range result.Records {
				source, _ := filepath.Abs(record.File)
				if source == output {
					same = true
					break
				}
				if a, err := os.Stat(source); err == nil {
					if b, err := os.Stat(output); err == nil && os.SameFile(a, b) {
						same = true
						break
					}
				}
			}
			if same {
				fmt.Fprintln(a.Stderr, loc.T("verifactu.output"))
				return 1
			}
			if e = securefile.WriteFileAtomic(cfg.salida, []byte(result.Report), 0600); e != nil {
				fmt.Fprintln(a.Stderr, signer.TraducirErrorVeriFactu(e, func(k string) string { return loc.T(k) }))
				return 1
			}
		}
		if !result.Valid {
			return 1
		}
		return 0
	}
	if cfg.operacion == "leer-qr-verifactu" {
		qr, e := signer.LeerQRVeriFactu(cfg.entrada)
		if e != nil {
			fmt.Fprintln(a.Stderr, signer.TraducirErrorVeriFactu(e, func(k string) string { return loc.T(k) }))
			return 1
		}
		data, _ := json.MarshalIndent(qr, "", "  ")
		fmt.Fprintln(a.Stdout, string(data))
		return 0
	}
	data, e := signer.ConsultarQRVeriFactu(ctx, cfg.entrada)
	if e != nil {
		fmt.Fprintln(a.Stderr, signer.TraducirErrorVeriFactu(e, func(k string) string { return loc.T(k) }))
		return 1
	}
	fmt.Fprintln(a.Stdout, string(data))
	return 0
}
