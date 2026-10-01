// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localizador_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestI18nCLITieneContratoBaseEnTodosLosIdiomas(t *testing.T) {
	claves := []string{
		"cli.error.no_operation",
		"cli.error.p12_secret_input",
		"cli.error.prefix",
		"cli.error.protection_secret_input",
		"cli.error.secret_env_cleanup",
		"cli.error.unsupported_operation",
		"cli.help.examples",
		"cli.help.summary",
		"cli.help.title",
		"security.secret.error",
		"security.secret.argv_value",
		"security.secret.argv_source_value",
		"security.secret.sensitive_option",
		"security.secret.stdin_unavailable",
		"security.secret.terminal_read",
		"security.secret.stdin_read",
		"security.secret.too_large",
		"security.secret.nul",
		"security.secret.environment_cleanup",
		"security.secret.hint.rest",
		"security.secret.hint.protection",
		"security.secret.hint.gui_p12",
		"security.secret.hint.import_p12",
		"security.secret.hint.p12",
		"security.secret.prompt.p12",
		"security.secret.prompt.protection",
		"rest.error.private_credential",
		"rest.help.private_delivery",
		"rest.output.bearer_file",
		"rest.output.curl_usage",
	}
	for _, idioma := range append([]string{"es"}, idiomasI18n...) {
		catalogo := cargarLocale(t, idioma)
		for _, clave := range claves {
			if strings.TrimSpace(catalogo[clave]) == "" {
				t.Errorf("%s.json: falta contenido para %q", idioma, clave)
			}
		}
	}
}

func TestCLIManualCubreOperacionesYBanderasPublicadas(t *testing.T) {
	raiz := raizRepositorio(t)
	manual := leerSuperficie(t, filepath.Join(raiz, "packaging", "linux", "man", "grxfirma.1"))
	ayuda := leerSuperficie(t, filepath.Join(raiz, "internal", "adapters", "inbound", "common", "cli", "adapter.go"))

	operaciones := []string{
		"firmar",
		"cofirmar",
		"contrafirmar",
		"verificar",
		"proteger",
		"proteger\\-firmando",
		"desproteger",
		"crear\\-hash",
		"comprobar\\-hash",
		"listar\\-destinatarios\\-proteccion",
		"exportar\\-destinatario\\-proteccion",
		"importar\\-destinatario\\-proteccion",
		"informe\\-diagnostico",
	}
	for _, operacion := range operaciones {
		needle := "\\-operacion " + operacion
		if !strings.Contains(manual, needle) {
			t.Errorf("grxfirma.1 no documenta %q", needle)
		}
	}

	for _, bandera := range []string{
		"\\-contenedor\\-proteccion",
		"\\-clave\\-proteccion\\-stdin",
		"\\-algoritmo\\-hash",
		"\\-formato\\-hash",
		"\\-fichero\\-hash",
		"\\-sello\\-qr",
		"\\-motivo\\-firma",
		"\\-ubicacion\\-firma",
		"\\-contacto\\-firma",
	} {
		if !strings.Contains(manual, bandera) {
			t.Errorf("grxfirma.1 no documenta la bandera %q", bandera)
		}
	}

	for _, operacion := range []string{
		"-operacion proteger-firmando",
		"-operacion crear-hash",
		"-operacion comprobar-hash",
	} {
		if !strings.Contains(ayuda, operacion) {
			t.Errorf("la ayuda CLI no documenta %q", operacion)
		}
	}
}

func TestManualesDocumentanPerfilesYRestriccionesDeFormatos(t *testing.T) {
	raiz := raizRepositorio(t)
	manual := leerSuperficie(t, filepath.Join(raiz, "packaging", "linux", "man", "grxfirma.1"))
	formatos := leerSuperficie(t, filepath.Join(raiz, "packaging", "linux", "man", "grxfirma-formatos.7"))

	const listaFormatos = "auto|pades|cades|xades|xmldsig|odf|ooxml|facturae|asic\\-xades"
	if !strings.Contains(manual, listaFormatos) {
		t.Errorf("grxfirma.1 no enumera todos los formatos: falta %q", listaFormatos)
	}
	for _, esperado := range []string{
		"\\-permitir\\-pdf\\-invalido",
		"Opción legacy",
		"se rechaza",
	} {
		if !strings.Contains(manual, esperado) {
			t.Errorf("grxfirma.1 no aclara el rechazo del PDF inválido: falta %q", esperado)
		}
	}

	for _, esperado := range []string{
		".SH XMLDSIG",
		".SH ODF",
		".SH OOXML",
		".SH FACTURAE",
		".SH ASIC\\-XADES",
		"solo soporta firma",
		"firma y cofirma",
		"monodato",
		"grxfirma \\-modo\\-cli \\-operacion firmar",
	} {
		if !strings.Contains(formatos, esperado) {
			t.Errorf("grxfirma-formatos.7 no documenta el contrato %q", esperado)
		}
	}
}

func TestManpagesLinuxSonEstructuralesYSeInstalanYDesinstalan(t *testing.T) {
	raiz := raizRepositorio(t)
	rutas, err := filepath.Glob(filepath.Join(raiz, "packaging", "linux", "man", "*"))
	if err != nil {
		t.Fatalf("Glob(man): %v", err)
	}
	if len(rutas) < 20 {
		t.Fatalf("se esperaban al menos 20 manpages publicadas, se encontraron %d", len(rutas))
	}

	makefileRaw, err := os.ReadFile(filepath.Join(raiz, "Makefile"))
	if err != nil {
		t.Fatalf("ReadFile(Makefile): %v", err)
	}
	makefile := string(makefileRaw)
	objetivos := map[string]string{}
	for _, objetivo := range []string{"install", "uninstall", "install-user", "uninstall-user"} {
		objetivos[objetivo] = cuerpoObjetivoMakefile(t, makefile, objetivo)
	}

	for _, ruta := range rutas {
		nombre := filepath.Base(ruta)
		contenido := leerSuperficie(t, ruta)
		// Git puede materializar los ficheros de texto con CRLF en Windows.
		// La estructura roff no depende del separador de linea del checkout.
		contenido = strings.ReplaceAll(contenido, "\r\n", "\n")
		if !strings.HasPrefix(contenido, ".TH ") {
			t.Errorf("%s: falta cabecera .TH", nombre)
		}
		if !strings.Contains(contenido, "\n.SH NOMBRE\n") {
			t.Errorf("%s: falta sección NOMBRE", nombre)
		}
		for objetivo, cuerpo := range objetivos {
			if !strings.Contains(cuerpo, nombre) {
				t.Errorf("Makefile %s: no incluye %s", objetivo, nombre)
			}
		}
	}
}

func cuerpoObjetivoMakefile(t *testing.T, makefile, objetivo string) string {
	t.Helper()
	lineas := strings.Split(makefile, "\n")
	dentro := false
	var cuerpo strings.Builder
	for _, linea := range lineas {
		if !dentro {
			if strings.HasPrefix(linea, objetivo+":") {
				dentro = true
			}
			continue
		}
		if linea != "" && linea[0] != '\t' && linea[0] != '#' {
			break
		}
		cuerpo.WriteString(linea)
		cuerpo.WriteByte('\n')
	}
	if !dentro {
		t.Fatalf("Makefile: no existe el objetivo %s", objetivo)
	}
	return cuerpo.String()
}
