// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Pruebas de los validadores de preferencias de usuario y de la ida y vuelta
// entre el mapa legacy y el documento tipado. Este paquete concentra la
// traduccion de la configuracion persistida, asi que un fallo silencioso aqui
// se traduce en preferencias perdidas o en valores fuera de rango aceptados
// como buenos.
package ports

import (
	"errors"
	"reflect"
	"testing"
)

func TestExtraerEnteroPositivo(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		entrada  string
		esperado int
		valido   bool
	}{
		{"vacia", "", 0, false},
		{"uno", "1", 1, true},
		{"varios digitos", "42", 42, true},
		{"cero no es positivo", "0", 0, false},
		{"ceros a la izquierda", "007", 7, true},
		{"signo mas rechazado", "+5", 0, false},
		{"signo menos rechazado", "-5", 0, false},
		{"espacio rechazado", " 5", 0, false},
		{"texto rechazado", "5a", 0, false},
		{"maximo int64", "9223372036854775807", 9223372036854775807, true},
		// Regresion: la acumulacion manual daba la vuelta al entero y estos
		// dos casos se colaban como validos (1 y 7766279631452241919).
		{"desbordamiento que daba 1", "18446744073709551617", 0, false},
		{"desbordamiento arbitrario", "99999999999999999999", 0, false},
		{"justo por encima de int64", "9223372036854775808", 0, false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			valor, ok := extraerEnteroPositivo(caso.entrada)
			if ok != caso.valido {
				t.Fatalf("extraerEnteroPositivo(%q) ok = %v, se esperaba %v", caso.entrada, ok, caso.valido)
			}
			if valor != caso.esperado {
				t.Fatalf("extraerEnteroPositivo(%q) = %d, se esperaba %d", caso.entrada, valor, caso.esperado)
			}
		})
	}
}

func TestExtraerPaginasSello(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		entrada  any
		esperado string
		valido   bool
	}{
		{"ausente", nil, "", false},
		{"todas en ingles", "all", "all", true},
		{"todas en castellano", "todas", "all", true},
		{"asterisco", "*", "all", true},
		{"pagina suelta", "3", "3", true},
		{"lista", "1,3,5", "1,3,5", true},
		{"rango", "2-7", "2-7", true},
		{"mezcla con espacios", " 1, 3-4 ,9 ", "1,3-4,9", true},
		{"rango invertido", "7-2", "", false},
		{"pagina cero", "0", "", false},
		{"coma sobrante", "1,,3", "", false},
		{"no numerico", "abc", "", false},
		{"no es cadena", 3, "", false},
		// Regresion: dependia de extraerEnteroPositivo, asi que un rango con
		// un extremo desbordado pasaba la comprobacion inicio > fin y ademas
		// se devolvia sin normalizar para que otro lo reparsease.
		{"extremo desbordado", "1-18446744073709551617", "", false},
		{"pagina desbordada", "18446744073709551617", "", false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			datos := map[string]any{}
			if caso.entrada != nil {
				datos["pages"] = caso.entrada
			}
			valor, ok := extraerPaginasSello(datos, "pages")
			if ok != caso.valido {
				t.Fatalf("extraerPaginasSello(%v) ok = %v, se esperaba %v", caso.entrada, ok, caso.valido)
			}
			if valor != caso.esperado {
				t.Fatalf("extraerPaginasSello(%v) = %q, se esperaba %q", caso.entrada, valor, caso.esperado)
			}
		})
	}
}

func TestExtraerURLHTTPNoVacia(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre  string
		entrada any
		valido  bool
	}{
		{"https", "https://tsa.dipgra.es/tsa", true},
		{"http", "http://127.0.0.1:8080/tsa", true},
		{"sin esquema", "tsa.dipgra.es", false},
		{"esquema no http", "ftp://tsa.dipgra.es", false},
		{"file rechazado", "file:///etc/passwd", false},
		{"sin host", "https://", false},
		{"vacia", "   ", false},
		{"no es cadena", 42, false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			_, ok := extraerURLHTTPNoVacia(map[string]any{"url": caso.entrada}, "url")
			if ok != caso.valido {
				t.Fatalf("extraerURLHTTPNoVacia(%v) ok = %v, se esperaba %v", caso.entrada, ok, caso.valido)
			}
		})
	}
}

func TestExtraerURIAbsolutaNoVacia(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre  string
		entrada any
		valido  bool
	}{
		{"https", "https://politica.dipgra.es/v1", true},
		{"urn", "urn:oid:2.16.724.1.3.1.1.2.1.9", true},
		{"oid opaco", "oid:2.16.724", true},
		{"relativa", "/politica/v1", false},
		{"esquema no permitido", "ftp://politica.dipgra.es", false},
		{"vacia", "", false},
		{"no es cadena", true, false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			_, ok := extraerURIAbsolutaNoVacia(map[string]any{"uri": caso.entrada}, "uri")
			if ok != caso.valido {
				t.Fatalf("extraerURIAbsolutaNoVacia(%v) ok = %v, se esperaba %v", caso.entrada, ok, caso.valido)
			}
		})
	}
}

func TestExtraerNivelCertificacionPDF(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		entrada  any
		esperado int
		valido   bool
	}{
		{"entero 0", 0, 0, true},
		{"entero 3", 3, 3, true},
		{"entero fuera de rango", 4, 0, false},
		{"negativo", -1, 0, false},
		{"cadena 2", "2", 2, true},
		{"cadena con espacios", " 1 ", 1, true},
		{"cadena fuera de rango", "9", 0, false},
		{"float entero", float64(3), 3, true},
		{"float con decimales", 2.5, 0, false},
		{"ausente", nil, 0, false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			datos := map[string]any{}
			if caso.entrada != nil {
				datos["nivel"] = caso.entrada
			}
			valor, ok := extraerNivelCertificacionPDF(datos, "nivel")
			if ok != caso.valido {
				t.Fatalf("extraerNivelCertificacionPDF(%v) ok = %v, se esperaba %v", caso.entrada, ok, caso.valido)
			}
			if valor != caso.esperado {
				t.Fatalf("extraerNivelCertificacionPDF(%v) = %d, se esperaba %d", caso.entrada, valor, caso.esperado)
			}
		})
	}
}

func TestExtraerFloat01(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre  string
		entrada any
		valido  bool
	}{
		{"cero", float64(0), true},
		{"uno", float64(1), true},
		{"medio", 0.5, true},
		{"entero 1", 1, true},
		{"negativo", -0.1, false},
		{"mayor que uno", 1.0001, false},
		{"cadena", "0.5", false},
		{"ausente", nil, false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			datos := map[string]any{}
			if caso.entrada != nil {
				datos["x"] = caso.entrada
			}
			_, ok := extraerFloat01(datos, "x")
			if ok != caso.valido {
				t.Fatalf("extraerFloat01(%v) ok = %v, se esperaba %v", caso.entrada, ok, caso.valido)
			}
		})
	}
}

func TestExtraerRotacionSello(t *testing.T) {
	t.Parallel()

	for _, valido := range []int{0, 1, 45, 90, 180, 270, 359} {
		if _, ok := extraerRotacionSello(map[string]any{"r": valido}, "r"); !ok {
			t.Fatalf("rotacion %d debio aceptarse", valido)
		}
	}
	for _, invalido := range []int{-90, -1, 360, 720} {
		if _, ok := extraerRotacionSello(map[string]any{"r": invalido}, "r"); ok {
			t.Fatalf("rotacion %d debio rechazarse", invalido)
		}
	}
}

func TestExtraerListaStringsNormalizada(t *testing.T) {
	t.Parallel()

	t.Run("recorta duplica y descarta vacias", func(t *testing.T) {
		t.Parallel()
		datos := map[string]any{"lista": []string{" dipgra.es ", "dipgra.es", "", "  ", "sede.dipgra.es"}}
		valores, ok := extraerListaStringsNormalizada(datos, "lista")
		if !ok {
			t.Fatal("se esperaba lista valida")
		}
		esperado := []string{"dipgra.es", "sede.dipgra.es"}
		if !reflect.DeepEqual(valores, esperado) {
			t.Fatalf("lista = %#v, se esperaba %#v", valores, esperado)
		}
	})

	t.Run("acepta []any de cadenas", func(t *testing.T) {
		t.Parallel()
		datos := map[string]any{"lista": []any{"a", "b"}}
		valores, ok := extraerListaStringsNormalizada(datos, "lista")
		if !ok || !reflect.DeepEqual(valores, []string{"a", "b"}) {
			t.Fatalf("lista = %#v ok = %v", valores, ok)
		}
	})

	t.Run("rechaza []any con elemento no cadena", func(t *testing.T) {
		t.Parallel()
		datos := map[string]any{"lista": []any{"a", 3}}
		if _, ok := extraerListaStringsNormalizada(datos, "lista"); ok {
			t.Fatal("se esperaba rechazo de elemento no cadena")
		}
	})

	t.Run("lista que queda vacia se rechaza", func(t *testing.T) {
		t.Parallel()
		datos := map[string]any{"lista": []string{"", "   "}}
		if _, ok := extraerListaStringsNormalizada(datos, "lista"); ok {
			t.Fatal("una lista solo de vacias no es una lista valida")
		}
	})
}

func TestExtraerListaTiposCertificado(t *testing.T) {
	t.Parallel()

	datos := map[string]any{"tipos": []string{"Fisica", " SELLO "}}
	valores, ok := extraerListaTiposCertificado(datos, "tipos")
	if !ok {
		t.Fatal("se esperaban tipos validos")
	}
	if !reflect.DeepEqual(valores, []string{"fisica", "sello"}) {
		t.Fatalf("tipos = %#v", valores)
	}

	if _, ok := extraerListaTiposCertificado(map[string]any{"tipos": []string{"fisica", "inventado"}}, "tipos"); ok {
		t.Fatal("un tipo desconocido debe invalidar la lista completa")
	}
}

func TestExtraerEnumeradosNormalizanYRechazan(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre    string
		fn        func(map[string]any, string) (string, bool)
		validos   map[string]string
		invalidos []string
	}{
		{
			nombre:    "closeBehavior",
			fn:        extraerCloseBehavior,
			validos:   map[string]string{"exit": "exit", " RESIDENT ": "resident"},
			invalidos: []string{"quit", "", "  "},
		},
		{
			nombre:    "accionFirma",
			fn:        extraerAccionFirma,
			validos:   map[string]string{"Sign": "sign", "COUNTERSIGN": "countersign"},
			invalidos: []string{"verify", ""},
		},
		{
			nombre:    "modoSobrescritura",
			fn:        extraerModoSobrescritura,
			validos:   map[string]string{"rename": "rename", " Force ": "force"},
			invalidos: []string{"skip", ""},
		},
		{
			nombre:    "modoMultifirma",
			fn:        extraerModoMultifirma,
			validos:   map[string]string{"CoSign": "cosign", "countersign": "countersign"},
			invalidos: []string{"sign", ""},
		},
		{
			nombre:    "proxyType",
			fn:        extraerProxyType,
			validos:   map[string]string{"None": "none", " manual": "manual"},
			invalidos: []string{"socks5", ""},
		},
		{
			nombre:    "algoritmoHash",
			fn:        extraerAlgoritmoHash,
			validos:   map[string]string{"sha-256": "SHA-256", " SHA-512 ": "SHA-512"},
			invalidos: []string{"MD5", "SHA256", ""},
		},
		{
			nombre:    "formatoHashFichero",
			fn:        extraerFormatoHashFichero,
			validos:   map[string]string{"HEX": "hex", "base64": "base64"},
			invalidos: []string{"pem", ""},
		},
		{
			nombre:    "formatoHashDirectorio",
			fn:        extraerFormatoHashDirectorio,
			validos:   map[string]string{"XML": "xml", "csv": "csv"},
			invalidos: []string{"json", ""},
		},
		{
			nombre:    "perfilFirma",
			fn:        extraerPerfilFirma,
			validos:   map[string]string{"B": "baseline", "baseline": "baseline", "LTA": "lta"},
			invalidos: []string{"ltv", ""},
		},
		{
			nombre:    "formatoXAdES",
			fn:        extraerFormatoXAdES,
			validos:   map[string]string{"XAdES Detached": "detached", "enveloping": "enveloping"},
			invalidos: []string{"internal", ""},
		},
		{
			nombre:    "subfiltroPAdES",
			fn:        extraerSubfiltroPAdES,
			validos:   map[string]string{"ETSI.CAdES.detached": "etsi", "adobe": "adobe"},
			invalidos: []string{"otro", ""},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			for entrada, esperado := range caso.validos {
				valor, ok := caso.fn(map[string]any{"k": entrada}, "k")
				if !ok {
					t.Fatalf("%s(%q) debio aceptarse", caso.nombre, entrada)
				}
				if valor != esperado {
					t.Fatalf("%s(%q) = %q, se esperaba %q", caso.nombre, entrada, valor, esperado)
				}
			}
			for _, entrada := range caso.invalidos {
				if _, ok := caso.fn(map[string]any{"k": entrada}, "k"); ok {
					t.Fatalf("%s(%q) debio rechazarse", caso.nombre, entrada)
				}
			}
		})
	}
}

func TestExtraerInt(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		entrada  any
		esperado int
		valido   bool
	}{
		{"int", 7, 7, true},
		{"int32", int32(7), 7, true},
		{"int64", int64(7), 7, true},
		{"float entero", float64(7), 7, true},
		{"float con decimales", 7.5, 0, false},
		{"cadena", "7", 0, false},
		{"ausente", nil, 0, false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			datos := map[string]any{}
			if caso.entrada != nil {
				datos["n"] = caso.entrada
			}
			valor, ok := extraerInt(datos, "n")
			if ok != caso.valido || valor != caso.esperado {
				t.Fatalf("extraerInt(%v) = (%d, %v), se esperaba (%d, %v)", caso.entrada, valor, ok, caso.esperado, caso.valido)
			}
		})
	}
}

func TestValidarDuracionCompatibilidadWeb(t *testing.T) {
	t.Parallel()

	for _, minutos := range []any{5, 30, 240, float64(60)} {
		if err := ValidarDuracionCompatibilidadWeb(map[string]any{
			"webCompatibilityDurationMinutes": minutos,
		}); err != nil {
			t.Fatalf("duracion valida %v rechazada: %v", minutos, err)
		}
	}
	for _, minutos := range []any{0, 4, 241, 30.5, "30", true} {
		if err := ValidarDuracionCompatibilidadWeb(map[string]any{
			"webCompatibilityDurationMinutes": minutos,
		}); !errors.Is(err, ErrWebCompatibilityDuration) {
			t.Fatalf("duracion invalida %v: error=%v", minutos, err)
		}
	}
	if err := ValidarDuracionCompatibilidadWeb(nil); err != nil {
		t.Fatalf("mapa nil: %v", err)
	}
	if err := ValidarDuracionCompatibilidadWeb(map[string]any{}); err != nil {
		t.Fatalf("duracion ausente: %v", err)
	}
}

// mapaCanonico devuelve preferencias ya normalizadas, de forma que la ida y
// vuelta deba ser la identidad exacta.
func mapaCanonico() map[string]any {
	return map[string]any{
		"idioma":                          "es",
		"themeIndex":                      2,
		"expertMode":                      true,
		"autoClose":                       false,
		"confirmToSign":                   true,
		"omitAskOnClose":                  false,
		"closeBehavior":                   "exit",
		"hideDnieStartScreen":             true,
		"webCompatibilityDurationMinutes": 30,
		"secureConnections":               true,
		"secureDomainsList":               []string{"dipgra.es", "sede.dipgra.es"},
		"signAction":                      "sign",
		"signFormat":                      "pades",
		"signProfile":                     "baseline",
		"signOverwrite":                   "rename",
		"signStrictCompat":                true,
		"signAllowInvalidPDF":             false,
		"signReason":                      "Conformidad",
		"signLocation":                    "Granada",
		"signContactInfo":                 "contacto@example.invalid",
	}
}

func TestDocumentoConfiguracionUsuario_IdaYVueltaEsIdentidad(t *testing.T) {
	t.Parallel()

	entrada := mapaCanonico()
	salida := DocumentoConfiguracionUsuarioDesdeMapa(entrada).Mapa()

	if !reflect.DeepEqual(entrada, salida) {
		for clave, esperado := range entrada {
			obtenido, presente := salida[clave]
			if !presente {
				t.Errorf("la ida y vuelta perdio la clave %q", clave)
				continue
			}
			if !reflect.DeepEqual(esperado, obtenido) {
				t.Errorf("clave %q: entrada %#v, salida %#v", clave, esperado, obtenido)
			}
		}
		for clave := range salida {
			if _, presente := entrada[clave]; !presente {
				t.Errorf("la ida y vuelta invento la clave %q", clave)
			}
		}
	}
}

func TestDocumentoConfiguracionUsuario_NoMutaElMapaDeEntrada(t *testing.T) {
	t.Parallel()

	entrada := mapaCanonico()
	copia := mapaCanonico()

	doc := DocumentoConfiguracionUsuarioDesdeMapa(entrada)
	_ = doc.Mapa()

	if !reflect.DeepEqual(entrada, copia) {
		t.Fatalf("DesdeMapa muto el mapa recibido:\n entrada = %#v\n original = %#v", entrada, copia)
	}
}

func TestDocumentoConfiguracionUsuario_TipaLosCamposConocidos(t *testing.T) {
	t.Parallel()

	doc := DocumentoConfiguracionUsuarioDesdeMapa(mapaCanonico())

	if doc.General.Idioma == nil || *doc.General.Idioma != "es" {
		t.Errorf("General.Idioma = %v", doc.General.Idioma)
	}
	if doc.General.ThemeIndex == nil || *doc.General.ThemeIndex != 2 {
		t.Errorf("General.ThemeIndex = %v", doc.General.ThemeIndex)
	}
	if doc.General.CloseBehavior == nil || *doc.General.CloseBehavior != "exit" {
		t.Errorf("General.CloseBehavior = %v", doc.General.CloseBehavior)
	}
	if !reflect.DeepEqual(doc.General.SecureDomainsList, []string{"dipgra.es", "sede.dipgra.es"}) {
		t.Errorf("General.SecureDomainsList = %#v", doc.General.SecureDomainsList)
	}
	if doc.Firma.Action == nil || *doc.Firma.Action != "sign" {
		t.Errorf("Firma.Action = %v", doc.Firma.Action)
	}
	if doc.Firma.Profile == nil || *doc.Firma.Profile != "baseline" {
		t.Errorf("Firma.Profile = %v", doc.Firma.Profile)
	}
	if doc.FirmaMeta.Location == nil || *doc.FirmaMeta.Location != "Granada" {
		t.Errorf("FirmaMeta.Location = %v", doc.FirmaMeta.Location)
	}
	if len(doc.Extras) != 0 {
		t.Errorf("todas las claves canonicas debieron tiparse, quedan Extras = %#v", doc.Extras)
	}
}

func TestDocumentoConfiguracionUsuario_ConservaClavesDesconocidas(t *testing.T) {
	t.Parallel()

	entrada := map[string]any{
		"idioma":            "es",
		"claveHeredadaRara": "valor",
		"otraClaveNumerica": 17,
		"bloqueAnidadoJSON": map[string]any{"a": 1},
	}
	doc := DocumentoConfiguracionUsuarioDesdeMapa(entrada)

	if _, presente := doc.Extras["idioma"]; presente {
		t.Error("idioma se tipa y no debe quedar en Extras")
	}
	for _, clave := range []string{"claveHeredadaRara", "otraClaveNumerica", "bloqueAnidadoJSON"} {
		if _, presente := doc.Extras[clave]; !presente {
			t.Errorf("Extras perdio la clave desconocida %q", clave)
		}
	}

	salida := doc.Mapa()
	if !reflect.DeepEqual(entrada, salida) {
		t.Fatalf("la ida y vuelta con claves desconocidas no es identidad:\n entrada = %#v\n salida = %#v", entrada, salida)
	}
}

// Un valor invalido no se tipa, pero tampoco se descarta: se queda en Extras y
// sobrevive a la ida y vuelta. Es la politica de compatibilidad heredada, y
// conviene que este fijada por una prueba para que nadie la cambie sin querer.
func TestDocumentoConfiguracionUsuario_ValorInvalidoSobreviveEnExtras(t *testing.T) {
	t.Parallel()

	entrada := map[string]any{
		"closeBehavior": "modo-inventado",
		"themeIndex":    "no-es-un-entero",
	}
	doc := DocumentoConfiguracionUsuarioDesdeMapa(entrada)

	if doc.General.CloseBehavior != nil {
		t.Errorf("closeBehavior invalido no debe tiparse, quedo %v", *doc.General.CloseBehavior)
	}
	if doc.General.ThemeIndex != nil {
		t.Errorf("themeIndex invalido no debe tiparse, quedo %v", *doc.General.ThemeIndex)
	}
	if !reflect.DeepEqual(doc.Mapa(), entrada) {
		t.Fatalf("los valores invalidos deben conservarse tal cual: %#v", doc.Mapa())
	}
}

func TestDocumentoConfiguracionUsuario_NoConservaCredencialSeguridadUIEnClaro(t *testing.T) {
	t.Parallel()

	doc := DocumentoConfiguracionUsuarioDesdeMapa(map[string]any{
		"idioma":                 "es",
		"securityAccessPassword": "secreto-local",
	})

	if _, presente := doc.Extras["securityAccessPassword"]; presente {
		t.Fatalf("securityAccessPassword no debe entrar en Extras: %#v", doc.Extras)
	}
	if _, presente := doc.Mapa()["securityAccessPassword"]; presente {
		t.Fatalf("securityAccessPassword no debe sobrevivir al roundtrip: %#v", doc.Mapa())
	}

	// Defensa en profundidad para documentos construidos por código o migradores
	// que rellenen Extras sin pasar por DocumentoConfiguracionUsuarioDesdeMapa.
	doc.Extras["securityAccessPassword"] = "otro-secreto"
	if _, presente := doc.Mapa()["securityAccessPassword"]; presente {
		t.Fatalf("Mapa no debe serializar la credencial UI desde Extras: %#v", doc.Mapa())
	}
}

func TestDocumentoConfiguracionUsuario_DescartaCaducidadWebHeredadaInvalida(t *testing.T) {
	t.Parallel()

	doc := DocumentoConfiguracionUsuarioDesdeMapa(map[string]any{
		"webCompatibilityDurationMinutes": 1440,
		"claveHeredadaRara":               "valor",
	})
	if doc.Desktop.WebCompatibilityDurationMinutes != nil {
		t.Fatalf("la duracion insegura no debe tiparse: %#v", doc.Desktop.WebCompatibilityDurationMinutes)
	}
	salida := doc.Mapa()
	if _, presente := salida["webCompatibilityDurationMinutes"]; presente {
		t.Fatalf("la duracion insegura no debe reaparecer como Extra: %#v", salida)
	}
	if salida["claveHeredadaRara"] != "valor" {
		t.Fatalf("se perdio una clave desconocida no relacionada: %#v", salida)
	}
}

func TestDocumentoConfiguracionUsuario_NoPersisteEstadoWebActivo(t *testing.T) {
	t.Parallel()

	doc := DocumentoConfiguracionUsuarioDesdeMapa(map[string]any{
		"webCompatibilityDurationMinutes": 30,
		"webCompatibilityActive":          true,
		"webCompatibilityExpiresAt":       "2099-01-01T00:00:00Z",
	})
	salida := doc.Mapa()
	if salida["webCompatibilityDurationMinutes"] != 30 {
		t.Fatalf("se perdio la duracion permitida: %#v", salida)
	}
	for _, clave := range []string{"webCompatibilityActive", "webCompatibilityExpiresAt"} {
		if _, presente := salida[clave]; presente {
			t.Fatalf("el estado efimero %q no debe persistirse: %#v", clave, salida)
		}
	}
}

func TestDocumentoConfiguracionUsuario_MapaNilNoRompe(t *testing.T) {
	t.Parallel()

	doc := DocumentoConfiguracionUsuarioDesdeMapa(nil)
	if doc.Extras == nil {
		t.Fatal("Extras debe quedar inicializado aunque la entrada sea nil")
	}
	if salida := doc.Mapa(); len(salida) != 0 {
		t.Fatalf("un documento vacio debe producir un mapa vacio, no %#v", salida)
	}
}
