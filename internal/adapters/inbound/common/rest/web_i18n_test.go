// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"golang.org/x/net/html"
)

const webI18nSentinel = "I18N_SENTINEL"

func sentinelWebLocalizer(string, ...any) string {
	return webI18nSentinel
}

func TestSignerPageMessagesCubreTodasLasReferenciasLiteralesDelTemplate(t *testing.T) {
	t.Parallel()

	messages := signerPageMessages(func(key string, _ ...any) string {
		return key
	})
	references := regexp.MustCompile(`\bt\(\s*"([^"]+)"\s*\)`).
		FindAllStringSubmatch(signerPageTemplateRaw, -1)
	missing := make([]string, 0)
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		key := reference[1]
		if _, checked := seen[key]; checked {
			continue
		}
		seen[key] = struct{}{}
		if _, exists := messages[key]; !exists {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("signerPageMessages no cubre referencias t() del template: %q", missing)
	}
}

func TestWebConsoleCubreTodasLasReferenciasLiteralesDelScript(t *testing.T) {
	t.Parallel()

	references := regexp.MustCompile(`\bt\(\s*"([^"]+)"\s*\)`).
		FindAllStringSubmatch(consoleIndexHTML, -1)
	translated := translateConsoleIndexPage(consoleIndexHTML, "zz", func(key string, _ ...any) string {
		return webI18nSentinel + "[" + key + "]"
	})
	missing := make([]string, 0)
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		key := reference[1]
		if _, checked := seen[key]; checked {
			continue
		}
		seen[key] = struct{}{}
		localizationKey := key
		switch key {
		case "__GRXFIRMA_BOOLEAN_YES__":
			localizationKey = "Sí"
		case "__GRXFIRMA_BOOLEAN_NO__":
			localizationKey = "No"
		case "__GRXFIRMA_ERROR_LABEL__":
			localizationKey = "Error"
		}
		expectedCall := `t` + `("` + webI18nSentinel + `[` + localizationKey + `]")`
		if !strings.Contains(translated, expectedCall) {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("translateConsoleIndexPage no cubre referencias t() del script: %q", missing)
	}
}

func TestWebConsoleCubreLiteralesDirectosEnSalidasDinamicas(t *testing.T) {
	t.Parallel()

	patterns := []struct {
		name       string
		expression *regexp.Regexp
	}{
		{name: "etiqueta strong", expression: regexp.MustCompile(`<strong>([^<]+)</strong>`)},
		{name: "resultBanner", expression: regexp.MustCompile(`resultBanner\(\s*[^,\n]+,\s*"([^"]+)"`)},
		{name: "setGlobalStatus", expression: regexp.MustCompile(`setGlobalStatus\(\s*[^,\n]+,\s*"([^"]+)"`)},
		{name: "textContent", expression: regexp.MustCompile(`\.textContent\s*=\s*"([^"]+)"`)},
		{name: "escapeHtml", expression: regexp.MustCompile(`escapeHtml\(\s*"([^"]+)"\s*\)`)},
		{name: "Error", expression: regexp.MustCompile(`new Error\(\s*"([^"]+)"`)},
		{name: "return", expression: regexp.MustCompile(`\breturn\s+"([^"]+)"`)},
	}
	seen := make(map[string]struct{})
	for _, pattern := range patterns {
		for _, match := range pattern.expression.FindAllStringSubmatch(consoleIndexHTML, -1) {
			key := match[1]
			visible := strings.TrimSpace(key)
			if visible == "" || strings.Contains(visible, "escapeHtml(") ||
				isWebI18nTechnicalLiteral(visible) || !containsWebI18nWord(visible) {
				continue
			}
			if pattern.name == "return" &&
				!strings.ContainsAny(visible, " \tÁÉÍÓÚÑáéíóúñ") {
				continue
			}
			identity := pattern.name + "\x00" + key
			if _, checked := seen[identity]; checked {
				continue
			}
			seen[identity] = struct{}{}
			translated := translateConsoleIndexPage(key, "zz", func(candidate string, _ ...any) string {
				return webI18nSentinel + "[" + candidate + "]"
			})
			expected := webI18nSentinel + "[" + key + "]"
			if translated != expected {
				t.Errorf("%s visible sin reemplazo exacto: %q", pattern.name, key)
			}
		}
	}
}

func TestWebLocalI18nNoDejaTextoVisibleFueraDelLocalizador(t *testing.T) {
	t.Parallel()

	pages := map[string]string{
		"consola":   translateConsoleIndexPage(consoleIndexHTML, "en", sentinelWebLocalizer),
		"firmador":  consoleSignerPage("zz", sentinelWebLocalizer),
		"validador": consoleValidatorPage("zz", sentinelWebLocalizer),
	}
	for name, page := range pages {
		name, page := name, page
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertNoUnlocalizedWebText(t, page)
		})
	}
}

func TestWebLocalI18nCubreControlesHistoricamenteHardcodeados(t *testing.T) {
	t.Parallel()

	keys := []string{
		"Recordar certificado seleccionado",
		"CN=..., NIF=..., etc.",
		"FNMT, ACCV, ...",
		"o carpeta",
		"/ruta/al/documento.pdf",
		"/ruta/de/salida/opcional",
		"/ruta/de/salida.hashreport",
		"/ruta/al/documento.bin",
		"/ruta/al/documento.afp, .enveloped o .authenveloped.p7m",
		"/ruta/de/salida/opcional.afp, .enveloped o .authenveloped.p7m",
		"cms AuthEnvelopedData (.authenveloped.p7m)",
		"AuthEnvelopedData usa AES-256-GCM y RSA-OAEP-SHA256/MGF1-SHA256; requiere un destinatario compatible con certificado X.509 RSA válido.",
		"QR del sello",
		"Opcional. Si indicas un texto o URL, se incrusta un QR dentro del sello visible y puede combinarse con imagen personalizada.",
		"Selecciona un certificado para exportar su resumen visible como JSON.",
		"Copiar JSON del certificado",
		"Copiar huella",
		"certificado",
		"Crear huella",
		"Comprobar huella",
		"Recursivo",
		"Estado actual",
	}
	seen := make(map[string]bool, len(keys))
	translateConsoleIndexPage(consoleIndexHTML, "en", func(key string, _ ...any) string {
		seen[key] = true
		return webI18nSentinel
	})
	for _, key := range keys {
		if !seen[key] {
			t.Errorf("la consola web no pasa por el localizador %q", key)
		}
	}

	messages := signerPageMessages(func(key string, _ ...any) string {
		seen[key] = true
		return webI18nSentinel
	})
	for _, messageKey := range []string{
		"rememberSelectedCertificate",
		"certificateIssuerFilterPlaceholder",
		"localPathPlaceholder",
		"outputPathPlaceholder",
	} {
		if messages[messageKey] != webI18nSentinel {
			t.Errorf("el firmador web no localiza el mensaje %q", messageKey)
		}
	}

	validatorMessages := validatorPageMessages(func(key string, _ ...any) string {
		seen[key] = true
		return webI18nSentinel
	})
	for _, messageKey := range []string{
		"title",
		"subtitle",
		"certificateHelp",
		"fileTooLarge",
		"selectRequired",
	} {
		if validatorMessages[messageKey] != webI18nSentinel {
			t.Errorf("el validador web no localiza el mensaje %q", messageKey)
		}
	}
}

func TestWebLocalI18nRenderizaControlesYPlaceholdersEnIngles(t *testing.T) {
	t.Parallel()

	adapter := New(nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/?lang=en", nil)
	response := httptest.NewRecorder()
	adapter.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /?lang=en = %d", response.Code)
	}
	body := response.Body.String()
	for _, expected := range []string{
		"Remember selected certificate",
		"CN=..., TIN=..., etc.",
		"or folder",
		"/path/to/document.pdf",
		"/optional/output/path",
		"Seal QR",
		"Select a certificate to export its visible summary as JSON.",
		"Copy certificate JSON",
		"Copy fingerprint",
		"Create hash",
		"Check hash",
		"Recursive",
		"Current status",
		"/output/path.hashreport",
		"/path/to/document.bin",
		"/path/to/document.afp, .enveloped or .authenveloped.p7m",
		"/optional/output/path.afp, .enveloped or .authenveloped.p7m",
		"CMS AuthEnvelopedData (.authenveloped.p7m)",
		"AuthEnvelopedData uses AES-256-GCM and RSA-OAEP-SHA256/MGF1-SHA256; it requires a compatible recipient with a valid RSA X.509 certificate.",
		"The current selection will not be remembered between sessions.",
		"Enable this preference and select a certificate to reuse it in the next session.",
		"The saved preference is no longer available in the current catalog.",
		"Certificate restored from the previous session: ",
		"It will be reused in the next session: ",
		"TLS status",
		"Detected certificates",
		"Private keys",
		"Local certificate",
		"No proxy",
		"System proxy",
		"Configured proxy",
		"Authenticated proxy",
		"Copy diagnostics",
		"Certificate JSON copied to clipboard.",
		"Could not copy the certificate JSON.",
		"Certificate fingerprint copied to clipboard.",
		"Could not copy the certificate fingerprint.",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("la consola inglesa no contiene %q", expected)
		}
	}
}

func TestWebConsoleDefineHelperParaMensajesDinamicosLocalizados(t *testing.T) {
	t.Parallel()

	if !strings.Contains(consoleIndexHTML, "function t(value)") ||
		!strings.Contains(consoleIndexHTML, "return value;") {
		t.Fatal("la consola debe definir t(value) para sus mensajes dinámicos localizados en servidor")
	}
}

func TestWebConsoleFlujoHashNoDejaLiteralesVisiblesSinLocalizar(t *testing.T) {
	t.Parallel()

	visibleLiteral := regexp.MustCompile(
		`"(?:Calculando|Indica|Selecciona|Descargar|Manifiesto|Huella|Comprobando|Directorio|Error al|Algoritmo|Formato|Entradas|Recursivo|Salida|Coinciden|No coinciden|Hash sin|Fichero sin|Informe|Esperada|Estado actual)[^"]*"`,
	)
	rawStrongLabel := regexp.MustCompile(`<strong>[[:alpha:]ÁÉÍÓÚÑáéíóúñ][^<]*</strong>`)
	localizedCall := regexp.MustCompile(`\bt\(\s*"([^"]+)"\s*\)`)
	for _, functionName := range []string{"createHash", "checkHash"} {
		body := consoleJavaScriptFunctionBody(t, functionName)
		withoutLocalizedCalls := localizedCall.ReplaceAllString(body, "t()")
		if literal := visibleLiteral.FindString(withoutLocalizedCalls); literal != "" {
			t.Errorf("%s conserva el literal visible sin t(): %s", functionName, literal)
		}
		if label := rawStrongLabel.FindString(body); label != "" {
			t.Errorf("%s conserva una etiqueta strong sin t(): %s", functionName, label)
		}

		translated := translateConsoleIndexPage(body, "zz", sentinelWebLocalizer)
		for _, reference := range localizedCall.FindAllStringSubmatch(translated, -1) {
			if reference[1] != webI18nSentinel {
				t.Errorf("%s usa t(%q) sin registrarlo en translateConsoleIndexPage", functionName, reference[1])
			}
		}
	}
}

func TestWebConsoleRenderizaFlujoHashEnIngles(t *testing.T) {
	t.Parallel()

	adapter := New(nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/?lang=en", nil)
	response := httptest.NewRecorder()
	adapter.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /?lang=en = %d", response.Code)
	}
	body := response.Body.String()
	for _, expected := range []string{
		"Calculating hash...",
		"Download manifest",
		"Hash manifest created.",
		"Directory hash manifest generated.",
		"Download hash",
		"Hash generated successfully.",
		"Checking integrity...",
		"Directory is intact.",
		"Directory differs from the manifest.",
		"Matching",
		"Not matching",
		"Manifest entry without file",
		"File without manifest entry",
		"Expected",
		"Error checking hash: ",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("la consola inglesa no contiene %q", expected)
		}
	}
}

func TestWebConsoleBooleanosLocalizadosNoCorrompenJavaScript(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		lang string
		yes  string
		no   string
	}{
		{lang: "de", yes: "Ja", no: "Nein"},
		{lang: "eu", yes: "Bai", no: "Ez"},
		{lang: "fr", yes: "Oui", no: "Non"},
		{lang: "gl", yes: "Si", no: "Non"},
		{lang: "pt", yes: "Sim", no: "Não"},
		{lang: "zh", yes: "是", no: "否"},
	} {
		testCase := testCase
		t.Run(testCase.lang, func(t *testing.T) {
			t.Parallel()

			adapter := New(nil, nil, nil)
			request := httptest.NewRequest(http.MethodGet, "/?lang="+testCase.lang, nil)
			response := httptest.NewRecorder()
			adapter.Routes().ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("GET /?lang=%s = %d", testCase.lang, response.Code)
			}
			body := response.Body.String()
			booleanExpression := `? ` + `t` + `("` + testCase.yes + `") : ` +
				`t` + `("` + testCase.no + `")`
			if count := strings.Count(body, booleanExpression); count != 4 {
				t.Errorf("la consola %s contiene %d de 4 booleanos localizados %q", testCase.lang, count, booleanExpression)
			}
			if !strings.Contains(body, "cert.NotAfter") {
				t.Errorf("la consola %s ha corrompido el identificador cert.NotAfter", testCase.lang)
			}
			if strings.Contains(body, "__GRXFIRMA_BOOLEAN_") {
				t.Errorf("la consola %s conserva un marcador booleano interno", testCase.lang)
			}
		})
	}
}

func consoleJavaScriptFunctionBody(t *testing.T, functionName string) string {
	t.Helper()
	marker := "async function " + functionName + "() {"
	start := strings.Index(consoleIndexHTML, marker)
	if start < 0 {
		t.Fatalf("no se encontró %s", marker)
	}
	bodyStart := start + len(marker)
	depth := 1
	for i := bodyStart; i < len(consoleIndexHTML); i++ {
		switch consoleIndexHTML[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return consoleIndexHTML[bodyStart:i]
			}
		}
	}
	t.Fatalf("la función JavaScript %s no tiene cierre", functionName)
	return ""
}

func assertNoUnlocalizedWebText(t *testing.T, page string) {
	t.Helper()
	document, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("html.Parse(): %v", err)
	}
	var inspect func(*html.Node, bool)
	inspect = func(node *html.Node, ignored bool) {
		if node.Type == html.ElementNode && (node.Data == "script" || node.Data == "style") {
			ignored = true
		}
		if !ignored && node.Type == html.TextNode {
			assertLocalizedWebFragment(t, "texto", node.Data)
		}
		if !ignored && node.Type == html.ElementNode {
			for _, attribute := range node.Attr {
				switch attribute.Key {
				case "placeholder", "title", "aria-label", "aria-description", "alt":
					assertLocalizedWebFragment(t, attribute.Key, attribute.Val)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			inspect(child, ignored)
		}
	}
	inspect(document, false)
}

func assertLocalizedWebFragment(t *testing.T, source, raw string) {
	t.Helper()
	value := strings.TrimSpace(raw)
	if value == "" || strings.Contains(value, webI18nSentinel) ||
		isWebI18nTechnicalLiteral(value) || !containsWebI18nWord(value) {
		return
	}
	t.Errorf("%s visible fuera del localizador: %q", source, value)
}

func containsWebI18nWord(value string) bool {
	consecutive := 0
	for _, r := range value {
		if unicode.IsLetter(r) {
			consecutive++
			if consecutive >= 3 {
				return true
			}
			continue
		}
		consecutive = 0
	}
	return false
}

func isWebI18nTechnicalLiteral(value string) bool {
	switch value {
	case "GrxFirma", "OpenAPI", "AUTO", "Auto", "PAdES", "CAdES", "XAdES", "/signer",
		"XMLdSig", "ODF", "OOXML", "ODF + XAdES", "OOXML + XMLDSig",
		"FacturaE", "ASiC-XAdES", "SHA-1",
		"SHA-256", "SHA-384", "SHA-512", "CMS", "JSON", "ES", "QR":
		return true
	default:
		return strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}")
	}
}
