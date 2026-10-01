// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// ---------------------------------------------------------------------------
// Tests de normalizarURI
// ---------------------------------------------------------------------------

func TestNormalizarURI_DobleEncoding(t *testing.T) {
	// %2520 (doble-encoding de espacio) debe convertirse en %20.
	// %253A (doble-encoding de ':') debe convertirse en %3A.
	raw := "afirma://sign?id=1&stservlet=https%253A%252F%252Fstore.example%252FService&dat=QQ%2520%3D%3D"
	got := normalizarURI(raw)
	if strings.Contains(got, "%2520") {
		t.Errorf("normalizarURI no elimino el doble-encoding %%2520: %s", got)
	}
	if strings.Contains(got, "%253A") {
		t.Errorf("normalizarURI no elimino el doble-encoding %%253A: %s", got)
	}
	// Debe quedar el encoding simple correcto
	if !strings.Contains(got, "%3A") {
		t.Errorf("normalizarURI deberia producir %%3A: %s", got)
	}
}

func TestNormalizarURI_PlusEnValoresBase64(t *testing.T) {
	// Los '+' en valores base64 deben convertirse a %2B para que url.ParseQuery
	// no los interprete como espacios.
	raw := "afirma://sign?id=1&stservlet=https%3A%2F%2Fstore.example%2FS&dat=AB+CD==&op=sign"
	got := normalizarURI(raw)
	// El '+' en el valor 'dat' debe estar codificado como %2B
	if strings.Contains(got, "dat=AB+CD") {
		t.Errorf("normalizarURI no convirtio '+' en valor a %%2B: %s", got)
	}
	if !strings.Contains(got, "dat=AB%2BCD") {
		t.Errorf("normalizarURI deberia producir dat=AB%%2BCD: %s", got)
	}
}

func TestNormalizarURI_SinQueryNoModifica(t *testing.T) {
	raw := "afirma://sign"
	got := normalizarURI(raw)
	if got != raw {
		t.Errorf("normalizarURI modifico URI sin query: %s", got)
	}
}

func TestNormalizarURI_VacioDevuelveVacio(t *testing.T) {
	if got := normalizarURI(""); got != "" {
		t.Errorf("normalizarURI con vacio deberia devolver vacio, obtuvo: %s", got)
	}
}

// ---------------------------------------------------------------------------
// Tests de extraerVersion
// ---------------------------------------------------------------------------

func TestExtraerVersion_ParametroV(t *testing.T) {
	parms := map[string][]string{"v": {"3"}}
	if got := extraerVersion(parms); got != 3 {
		t.Errorf("esperaba version 3, obtuvo %d", got)
	}
}

func TestExtraerVersion_ParametroVer(t *testing.T) {
	parms := map[string][]string{"ver": {"2"}}
	if got := extraerVersion(parms); got != 2 {
		t.Errorf("esperaba version 2, obtuvo %d", got)
	}
}

func TestExtraerVersion_SinParametro(t *testing.T) {
	parms := map[string][]string{}
	if got := extraerVersion(parms); got != 0 {
		t.Errorf("esperaba version 0, obtuvo %d", got)
	}
}

func TestExtraerVersion_ValorInvalido(t *testing.T) {
	parms := map[string][]string{"v": {"abc"}}
	if got := extraerVersion(parms); got != 0 {
		t.Errorf("esperaba version 0 para valor invalido, obtuvo %d", got)
	}
}

// ---------------------------------------------------------------------------
// Tests de solicitudes con Version
// ---------------------------------------------------------------------------

func TestParse_VersionSeAlmacenaEnSolicitud(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://sign?id=req-v3&stservlet=https%3A%2F%2Fstore.example%2FStorageService&v=3")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.Version != 3 {
		t.Fatalf("version esperada 3, obtenida %d", solicitud.Version)
	}
}

func TestParse_VersionCeroSiNoSeEspecifica(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://sign?id=req-nv&stservlet=https%3A%2F%2Fstore.example%2FStorageService")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.Version != 0 {
		t.Fatalf("version esperada 0, obtenida %d", solicitud.Version)
	}
}

// ---------------------------------------------------------------------------
// Tests de robustez: ningun panic con input hostil
// ---------------------------------------------------------------------------

func TestParse_InputHostil_NoPanic(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	ctx := context.Background()

	casos := []string{
		"",
		"   ",
		"afirma://",
		"afirma://?",
		"afirma://sign",
		"afirma://sign?",
		"afirma://sign?id=",
		"afirma://sign?id=x",
		"afirma://sign?id=x&stservlet=",
		"afirma://sign?id=x&stservlet=nohttps",
		"afirma://sign?id=x&stservlet=%",
		"afirma://sign?id=x&stservlet=https://store.example&dat=%zz",
		"afirma://sign?id=x&stservlet=https://store.example&dat=!!!invalido!!!",
		"afirma://sign?id=x&stservlet=https://store.example&dat=" + strings.Repeat("A", 100_000),
		string([]byte{0x00, 0x01, 0x02, 0x03}),
		"afirma://" + strings.Repeat("x", 100_000),
		"afirma://sign?" + strings.Repeat("a=b&", 10_000),
		":::::::invalid:::::",
		"\x00\xff\xfe",
		"afirma://sign?id=1&stservlet=https://store.example&dat=" + base64.StdEncoding.EncodeToString([]byte{}),
	}

	for i, uri := range casos {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("caso %d provoco panic con URI %q: %v", i, truncar(uri, 80), r)
				}
			}()
			_, _ = adaptador.Parse(ctx, uri) // solo verificamos que no haga panic
		}()
	}
}

func truncar(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ---------------------------------------------------------------------------
// Corpus de 20+ URIs reales representativas del protocolo afirma://
// ---------------------------------------------------------------------------

type casoCorpus struct {
	nombre    string
	uri       string
	version   int
	operacion TipoOperacion
	accion    domain.SignatureAction
	formato   domain.SignatureFormat
	requestID string
}

func TestParse_CorpusURIsReales(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	ctx := context.Background()

	// Payload de datos embebido (texto "DOCUMENTO")
	datEmbebido := base64.StdEncoding.EncodeToString([]byte("DOCUMENTO"))
	// Payload batch JSON minimo
	batchJSON := `{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"d1","datareference":"` +
		base64.StdEncoding.EncodeToString([]byte("DOC")) + `"}]}`
	datBatch := base64.StdEncoding.EncodeToString([]byte(batchJSON))

	base := "https%3A%2F%2Fportafirmas.example%2F"

	casos := []casoCorpus{
		// 1. sign basico version 3 con rtservlet/stservlet
		{
			nombre:    "sign-v3-rtservlet-stservlet",
			uri:       "afirma://sign?v=3&fileId=req-001&retrieveServlet=" + base + "RetrieveService&storageServlet=" + base + "StorageService&signFormat=CAdES",
			version:   3,
			operacion: OperacionFirma,
			accion:    domain.ActionSign,
			formato:   domain.FormatCAdES,
			requestID: "req-001",
		},
		// 2. sign con payload embebido version 0 (sin v=)
		{
			nombre:    "sign-payload-embebido-v0",
			uri:       "afirma://sign?id=req-002&stservlet=" + base + "StorageService&dat=" + datEmbebido,
			version:   0,
			operacion: OperacionFirma,
			accion:    domain.ActionSign,
			formato:   domain.FormatCAdES,
			requestID: "req-002",
		},
		// 3. firmar (alias espanol) con XAdES
		{
			nombre:    "firmar-alias-xades",
			uri:       "afirma://firmar?id=req-003&stservlet=" + base + "StorageService&signFormat=XAdES",
			version:   0,
			operacion: OperacionFirma,
			accion:    domain.ActionSign,
			formato:   domain.FormatXAdES,
			requestID: "req-003",
		},
		// 4. cosign con PAdES version 2
		{
			nombre:    "cosign-pades-v2",
			uri:       "afirma://cosign?ver=2&fileId=req-004&rtservlet=" + base + "RetrieveService&stservlet=" + base + "StorageService&signFormat=PAdES",
			version:   2,
			operacion: OperacionFirma,
			accion:    domain.ActionCoSign,
			formato:   domain.FormatPAdES,
			requestID: "req-004",
		},
		// 5. cofirmar (alias espanol)
		{
			nombre:    "cofirmar-alias",
			uri:       "afirma://cofirmar?id=req-005&stservlet=" + base + "StorageService",
			version:   0,
			operacion: OperacionFirma,
			accion:    domain.ActionCoSign,
			formato:   domain.FormatCAdES,
			requestID: "req-005",
		},
		// 6. countersign version 1
		{
			nombre:    "countersign-v1",
			uri:       "afirma://countersign?v=1&fileId=req-006&retrieveServlet=" + base + "RetrieveService&storageServlet=" + base + "StorageService",
			version:   1,
			operacion: OperacionFirma,
			accion:    domain.ActionCounterSign,
			formato:   domain.FormatCAdES,
			requestID: "req-006",
		},
		// 7. contrafirmar (alias espanol) version 4
		{
			nombre:    "contrafirmar-v4",
			uri:       "afirma://contrafirmar?v=4&id=req-007&stservlet=" + base + "StorageService&signFormat=XAdES",
			version:   4,
			operacion: OperacionFirma,
			accion:    domain.ActionCounterSign,
			formato:   domain.FormatXAdES,
			requestID: "req-007",
		},
		// 8. batch sin payload (remoto) version 3
		{
			nombre:    "batch-remoto-v3",
			uri:       "afirma://batch?v=3&fileId=req-008&retrieveServlet=" + base + "RetrieveService&storageServlet=" + base + "StorageService",
			version:   3,
			operacion: OperacionLote,
			accion:    "",
			formato:   domain.FormatCAdES,
			requestID: "req-008",
		},
		// 9. batch con payload embebido
		{
			nombre:    "batch-payload-embebido",
			uri:       "afirma://batch?id=req-009&stservlet=" + base + "StorageService&dat=" + datBatch,
			version:   0,
			operacion: OperacionLote,
			accion:    "",
			formato:   domain.FormatCAdES,
			requestID: "req-009",
		},
		// 10. signbatch (alias) version 2
		{
			nombre:    "signbatch-alias-v2",
			uri:       "afirma://signbatch?ver=2&fileId=req-010&retrieveServlet=" + base + "RetrieveService&storageServlet=" + base + "StorageService",
			version:   2,
			operacion: OperacionLote,
			accion:    "",
			formato:   domain.FormatCAdES,
			requestID: "req-010",
		},
		// 11. selectcert
		{
			nombre:    "selectcert",
			uri:       "afirma://selectcert?id=req-011&stservlet=" + base + "StorageService",
			version:   0,
			operacion: OperacionSelectCert,
			accion:    "",
			formato:   domain.FormatCAdES,
			requestID: "req-011",
		},
		// 12. sign con op= query param (variante)
		{
			nombre:    "sign-via-op-param",
			uri:       "afirma://?op=sign&id=req-012&stservlet=" + base + "StorageService&signFormat=CAdES",
			version:   0,
			operacion: OperacionFirma,
			accion:    domain.ActionSign,
			formato:   domain.FormatCAdES,
			requestID: "req-012",
		},
		// 13. cosign via operation= param
		{
			nombre:    "cosign-via-operation-param",
			uri:       "afirma://?operation=cosign&id=req-013&stservlet=" + base + "StorageService",
			version:   0,
			operacion: OperacionFirma,
			accion:    domain.ActionCoSign,
			formato:   domain.FormatCAdES,
			requestID: "req-013",
		},
		// 14. sign con rtservlet alias (sin stservlet)
		{
			nombre:    "sign-solo-rtservlet",
			uri:       "afirma://sign?id=req-014&rtservlet=" + base + "RetrieveService",
			version:   0,
			operacion: OperacionFirma,
			accion:    domain.ActionSign,
			formato:   domain.FormatCAdES,
			requestID: "req-014",
		},
		// 15. sign con retrieveservlet alias minusculas
		{
			nombre:    "sign-retrieveservlet-minusculas",
			uri:       "afirma://sign?id=req-015&retrieveservlet=" + base + "RetrieveService",
			version:   0,
			operacion: OperacionFirma,
			accion:    domain.ActionSign,
			formato:   domain.FormatCAdES,
			requestID: "req-015",
		},
		// 16. Portafirmas real: sign version 3 con clave de sesion
		{
			nombre:    "portafirmas-sign-v3-con-key",
			uri:       "afirma://sign?v=3&fileId=pf-req-016&retrieveServlet=" + base + "RetrieveService&storageServlet=" + base + "StorageService&key=clave%2Bsesion&signFormat=CAdES",
			version:   3,
			operacion: OperacionFirma,
			accion:    domain.ActionSign,
			formato:   domain.FormatCAdES,
			requestID: "pf-req-016",
		},
		// 17. Portafirmas real: batch version 3 con clave de sesion
		{
			nombre:    "portafirmas-batch-v3-con-key",
			uri:       "afirma://batch?v=3&fileId=pf-req-017&retrieveServlet=" + base + "RetrieveService&storageServlet=" + base + "StorageService&key=clave%2Bsesion",
			version:   3,
			operacion: OperacionLote,
			accion:    "",
			formato:   domain.FormatCAdES,
			requestID: "pf-req-017",
		},
		// 18. URI con XAdES version 1
		{
			nombre:    "sign-xades-v1",
			uri:       "afirma://sign?v=1&id=req-018&stservlet=" + base + "StorageService&signFormat=XAdES",
			version:   1,
			operacion: OperacionFirma,
			accion:    domain.ActionSign,
			formato:   domain.FormatXAdES,
			requestID: "req-018",
		},
		// 19. URI con PAdES version 0
		{
			nombre:    "sign-pades-v0",
			uri:       "afirma://sign?v=0&id=req-019&stservlet=" + base + "StorageService&signFormat=PAdES",
			version:   0,
			operacion: OperacionFirma,
			accion:    domain.ActionSign,
			formato:   domain.FormatPAdES,
			requestID: "req-019",
		},
		// 20. sign con fileId (camelCase) version 4
		{
			nombre:    "sign-fileid-camelcase-v4",
			uri:       "afirma://sign?v=4&fileId=req-020&stservlet=" + base + "StorageService&signFormat=CAdES",
			version:   4,
			operacion: OperacionFirma,
			accion:    domain.ActionSign,
			formato:   domain.FormatCAdES,
			requestID: "req-020",
		},
		// 21. selectcert version 2 con stservlet
		{
			nombre:    "selectcert-v2",
			uri:       "afirma://selectcert?ver=2&id=req-021&stservlet=" + base + "StorageService",
			version:   2,
			operacion: OperacionSelectCert,
			accion:    "",
			formato:   domain.FormatCAdES,
			requestID: "req-021",
		},
		// 22. countersign con PAdES y payload embebido
		{
			nombre:    "countersign-pades-payload",
			uri:       "afirma://countersign?id=req-022&stservlet=" + base + "StorageService&signFormat=PAdES&dat=" + datEmbebido,
			version:   0,
			operacion: OperacionFirma,
			accion:    domain.ActionCounterSign,
			formato:   domain.FormatPAdES,
			requestID: "req-022",
		},
		// 23. batch con XAdES version 1
		{
			nombre:    "batch-xades-v1",
			uri:       "afirma://batch?v=1&fileId=req-023&retrieveServlet=" + base + "RetrieveService&storageServlet=" + base + "StorageService&signFormat=XAdES",
			version:   1,
			operacion: OperacionLote,
			accion:    "",
			formato:   domain.FormatXAdES,
			requestID: "req-023",
		},
	}

	for _, tc := range casos {
		t.Run(tc.nombre, func(t *testing.T) {
			solicitud, err := adaptador.Parse(ctx, tc.uri)
			if err != nil {
				t.Fatalf("no se esperaba error: %v", err)
			}
			if solicitud.Version != tc.version {
				t.Errorf("version: esperada %d, obtenida %d", tc.version, solicitud.Version)
			}
			if solicitud.Operacion != tc.operacion {
				t.Errorf("operacion: esperada %s, obtenida %s", tc.operacion, solicitud.Operacion)
			}
			if tc.accion != "" && solicitud.AccionFirma != tc.accion {
				t.Errorf("accion: esperada %s, obtenida %s", tc.accion, solicitud.AccionFirma)
			}
			if solicitud.Formato != tc.formato {
				t.Errorf("formato: esperado %s, obtenido %s", tc.formato, solicitud.Formato)
			}
			if solicitud.Sesion.RequestID != tc.requestID {
				t.Errorf("requestID: esperado %s, obtenido %s", tc.requestID, solicitud.Sesion.RequestID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests de encoding especial Guadaltel/Portafirmas
// ---------------------------------------------------------------------------

func TestParse_NormalizacionDobleEncoding(t *testing.T) {
	// Simula URI de Portafirmas con doble-encoding en las URLs de servicio.
	// La URL del servlet https://portafirmas.example/StorageService
	// codificada una vez: https%3A%2F%2Fportafirmas.example%2FStorageService
	// codificada dos veces (Guadaltel): https%253A%252F%252Fportafirmas.example%252FStorageService
	// normalizarURI debe eliminar la capa extra: %253A → %3A, %252F → %2F
	// para que url.ParseQuery lo decodifique a la URL correcta.
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	doubleEncoded := "https%253A%252F%252Fportafirmas.example%252FStorageService"
	uri := "afirma://sign?id=req-doble&stservlet=" + doubleEncoded + "&v=3"

	solicitud, err := adaptador.Parse(context.Background(), uri)
	if err != nil {
		t.Fatalf("no se esperaba error con doble-encoding: %v", err)
	}
	if solicitud.Version != 3 {
		t.Errorf("version esperada 3, obtenida %d", solicitud.Version)
	}
	if solicitud.Sesion.UploadEndpoint != "https://portafirmas.example/StorageService" {
		t.Errorf("endpoint inesperado tras normalizar doble-encoding: %q", solicitud.Sesion.UploadEndpoint)
	}
}

func TestParse_NormalizacionPlusEnBase64(t *testing.T) {
	// Base64 de "AutoFirma+" que contiene '+' como caracter valido de base64
	// Cuando llega sin encodear en query param debe ser tratado correctamente.
	contenido := []byte{0x00, 0xFB, 0xEF, 0xBE} // produce base64 con '+' y '/'
	b64 := base64.StdEncoding.EncodeToString(contenido)
	if !strings.ContainsAny(b64, "+/") {
		// Si por casualidad no hay '+' o '/', construir uno que sí tenga
		b64 = "AP++/w==" // base64 con '+' y '/'
		_ = contenido
	}

	// La URI tiene el '+' sin encodear en el valor de dat
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	uri := "afirma://sign?id=req-b64plus&stservlet=https%3A%2F%2Fstore.example%2FStorageService&dat=" + b64

	solicitud, err := adaptador.Parse(context.Background(), uri)
	if err != nil {
		t.Fatalf("no se esperaba error con base64 con '+': %v", err)
	}
	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand para payload embebido con '+'")
	}
}

func TestParse_PayloadEmbebidoBase64URLSafe(t *testing.T) {
	contenido := []byte("%PDF-test-urlsafe%")
	b64 := base64.RawURLEncoding.EncodeToString(contenido)

	adaptador := New(trustStub{estado: domain.TrustAllowed})
	uri := "afirma://sign?id=req-b64url&stservlet=https%3A%2F%2Fstore.example%2FStorageService&dat=" + b64

	solicitud, err := adaptador.Parse(context.Background(), uri)
	if err != nil {
		t.Fatalf("no se esperaba error con base64 url-safe: %v", err)
	}
	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand para payload embebido url-safe")
	}
	if got := string(solicitud.SignCommand.Document.Content); got != string(contenido) {
		t.Fatalf("payload decodificado inesperado: %q", got)
	}
}

// TestParse_RechazaURIDemasiadoGrande (T055) verifica el rechazo de payloads
// por encima del límite de tamaño antes de parsear la URI.
func TestParse_RechazaURIDemasiadoGrande(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	rawURI := "afirma://sign?dat=" + strings.Repeat("A", MaxURISize+1)
	_, err := adaptador.Parse(context.Background(), rawURI)
	if err == nil {
		t.Fatal("se esperaba error por URI demasiado grande")
	}
	if !strings.Contains(err.Error(), "tamaño máximo") {
		t.Fatalf("error inesperado: %v", err)
	}
}
