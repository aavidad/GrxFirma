// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"grxfirma/internal/domain"
)

// aspectoDictamenResponse serializa un aspecto del dictamen. Las claves y
// los valores son castellanos y cerrados: forman el contrato
// autofirmav2.dictamen-verificacion.v1.
type aspectoDictamenResponse struct {
	Estado string `json:"estado"`
	Motivo string `json:"motivo,omitempty"`
	Fuente string `json:"fuente,omitempty"`
	Fecha  string `json:"fecha,omitempty"`
}

type firmanteDictamenResponse struct {
	CertificadoHuellaSHA256 string                  `json:"certificadoHuellaSHA256"`
	Serie                   string                  `json:"serie,omitempty"`
	Asunto                  string                  `json:"asunto,omitempty"`
	Emisor                  string                  `json:"emisor,omitempty"`
	Cadena                  aspectoDictamenResponse `json:"cadena"`
	Certificado             aspectoDictamenResponse `json:"certificado"`
	Revocacion              aspectoDictamenResponse `json:"revocacion"`
	SelloTiempo             aspectoDictamenResponse `json:"selloTiempo"`
}

type extensionesDictamenResponse struct {
	RevocacionRemota  string `json:"revocacionRemota"`
	SelloTiempoRemoto string `json:"selloTiempoRemoto"`
}

type dictamenResponse struct {
	Contrato                string                      `json:"contrato"`
	Estado                  string                      `json:"estado"`
	Motivo                  string                      `json:"motivo"`
	Formato                 string                      `json:"formato,omitempty"`
	ComprobadoEn            string                      `json:"comprobadoEn"`
	Integridad              aspectoDictamenResponse     `json:"integridad"`
	Cadena                  aspectoDictamenResponse     `json:"cadena"`
	Certificado             aspectoDictamenResponse     `json:"certificado"`
	Revocacion              aspectoDictamenResponse     `json:"revocacion"`
	SelloTiempo             aspectoDictamenResponse     `json:"selloTiempo"`
	VinculoOriginal         aspectoDictamenResponse     `json:"vinculoOriginal"`
	HuellaFirmadoSHA256     string                      `json:"huellaFirmadoSHA256"`
	HuellaOriginalSHA256    string                      `json:"huellaOriginalSHA256,omitempty"`
	CertificadoHuellaSHA256 string                      `json:"certificadoHuellaSHA256,omitempty"`
	Firmantes               []firmanteDictamenResponse  `json:"firmantes"`
	Extensiones             extensionesDictamenResponse `json:"extensiones"`
}

func buildDictamenResponse(d *domain.DictamenVerificacion) *dictamenResponse {
	if d == nil {
		return nil
	}
	out := &dictamenResponse{
		Contrato:                d.Contrato,
		Estado:                  string(d.Estado),
		Motivo:                  string(d.Motivo),
		Formato:                 d.Formato,
		ComprobadoEn:            fechaDictamen(d.ComprobadoEn),
		Integridad:              aspectoDictamen(d.Integridad),
		Cadena:                  aspectoDictamen(d.Cadena),
		Certificado:             aspectoDictamen(d.Certificado),
		Revocacion:              aspectoDictamen(d.Revocacion),
		SelloTiempo:             aspectoDictamen(d.SelloTiempo),
		VinculoOriginal:         aspectoDictamen(d.VinculoOriginal),
		HuellaFirmadoSHA256:     d.HuellaFirmadoSHA256,
		HuellaOriginalSHA256:    d.HuellaOriginalSHA256,
		CertificadoHuellaSHA256: d.CertificadoHuellaSHA256,
		Firmantes:               make([]firmanteDictamenResponse, 0, len(d.Firmantes)),
		Extensiones: extensionesDictamenResponse{
			RevocacionRemota:  d.Extensiones.RevocacionRemota,
			SelloTiempoRemoto: d.Extensiones.SelloTiempoRemoto,
		},
	}
	for _, f := range d.Firmantes {
		out.Firmantes = append(out.Firmantes, firmanteDictamenResponse{
			CertificadoHuellaSHA256: f.CertificadoHuellaSHA256,
			Serie:                   f.Serie,
			Asunto:                  f.Asunto,
			Emisor:                  f.Emisor,
			Cadena:                  aspectoDictamen(f.Cadena),
			Certificado:             aspectoDictamen(f.Certificado),
			Revocacion:              aspectoDictamen(f.Revocacion),
			SelloTiempo:             aspectoDictamen(f.SelloTiempo),
		})
	}
	return out
}

func aspectoDictamen(a domain.AspectoDictamen) aspectoDictamenResponse {
	return aspectoDictamenResponse{Estado: a.Estado, Motivo: a.Motivo, Fuente: a.Fuente, Fecha: fechaDictamen(a.Fecha)}
}

func fechaDictamen(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

type healthSoloVerificacionResponse struct {
	OK      bool   `json:"ok"`
	Service string `json:"service"`
	Modo    string `json:"modo"`
}

// RoutesSoloVerificacion publica salud y los contratos v1 y v2 de verificación.
// No hay firma, importación de certificados, protección,
// ajustes, gestión de servicio, páginas web ni rutas de fichero: cualquier
// otra ruta responde 404. El arranque del modo exige TLS 1.3 y Bearer.
func (a *Adaptador) RoutesSoloVerificacion() http.Handler {
	health := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
			return
		}
		writeJSON(w, http.StatusOK, healthSoloVerificacionResponse{
			OK:      true,
			Service: "GrxFirma validador",
			Modo:    "solo_verificacion",
		})
	})
	verify := http.HandlerFunc(a.handleVerifySoloBase64)
	// Enrutado literal: ServeMux puede normalizar rutas y devolver 301.
	routes := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != r.URL.Path {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/health" && r.URL.Path != "/verify" && r.URL.Path != "/v2/verify" {
			http.NotFound(w, r)
			return
		}
		token := strings.TrimSpace(a.BearerToken)
		if token == "" {
			writeError(w, http.StatusServiceUnavailable, "autorizacion no configurada")
			return
		}
		if !bearerSoloVerificacionValido(r, token) {
			writeError(w, http.StatusUnauthorized, "autorizacion requerida")
			return
		}
		switch r.URL.Path {
		case "/health":
			health.ServeHTTP(w, r)
		case "/verify":
			verify.ServeHTTP(w, r)
		case "/v2/verify":
			a.handleVerifyV2(w, r)
		}
	})
	limit := a.MaxBodyBytes
	if limit <= 0 {
		limit = 100 * 1024 * 1024
	}
	return limitBodyMiddleware(restCorrelationMiddleware(routes), limit)
}

func bearerSoloVerificacionValido(r *http.Request, token string) bool {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || len(r.Header.Values("X-API-Token")) != 0 {
		return false
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(token)) == 1
}

// handleVerifySoloBase64 rechaza siempre las rutas de fichero, aunque el
// adaptador se hubiera configurado para admitirlas en otro modo.
func (a *Adaptador) handleVerifySoloBase64(w http.ResponseWriter, r *http.Request) {
	if a.AllowFileSystemPaths {
		writeError(w, http.StatusForbidden, "operaciones con rutas de fichero deshabilitadas; use content_base64")
		return
	}
	a.handleVerify(w, r)
}
