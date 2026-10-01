// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"grxfirma/internal/domain"
)

const maximoCuerpoIdentidad = 1024 * 1024

type dictamenIdentidadDTO struct {
	Status    string `json:"status,omitempty"`
	Source    string `json:"source,omitempty"`
	CheckedAt string `json:"checkedAt,omitempty"`
}

type respuestaVerificacionIdentidadDTO struct {
	Outcome                string               `json:"outcome"`
	EvidenceRef            string               `json:"evidenceRef"`
	Contract               string               `json:"contract"`
	ChallengeID            string               `json:"challengeId"`
	Audience               string               `json:"audience"`
	RegisteredClient       string               `json:"registeredClient"`
	Purpose                string               `json:"purpose"`
	Operation              string               `json:"operation"`
	TenantContextHash      string               `json:"tenantContextHash"`
	SessionBinding         string               `json:"sessionBinding"`
	Origin                 string               `json:"origin"`
	ConsentID              string               `json:"consentId"`
	ConsentVersion         string               `json:"consentVersion"`
	PolicyID               string               `json:"policyId"`
	PolicyVersion          string               `json:"policyVersion"`
	IssuedAt               string               `json:"issuedAt"`
	ExpiresAt              string               `json:"expiresAt"`
	AssuranceLevel         string               `json:"assuranceLevel,omitempty"`
	AuthenticationMethod   string               `json:"authenticationMethod,omitempty"`
	Identity               string               `json:"identity,omitempty"`
	CertificateFingerprint string               `json:"certificateFingerprint,omitempty"`
	Integrity              dictamenIdentidadDTO `json:"integrity"`
	Chain                  dictamenIdentidadDTO `json:"chain"`
	Validity               dictamenIdentidadDTO `json:"validity"`
	KeyUsage               dictamenIdentidadDTO `json:"keyUsage"`
	CertificatePolicy      dictamenIdentidadDTO `json:"certificatePolicy"`
	Revocation             dictamenIdentidadDTO `json:"revocation"`
}

type errorIdentidadDTO struct {
	ErrorCode string `json:"errorCode"`
}

func (a *Adaptador) authorizeIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.authenticationConfigured() {
			writeJSON(w, http.StatusServiceUnavailable, errorIdentidadDTO{ErrorCode: "identity.configuration_unavailable"})
			return
		}
		a.authorize(next).ServeHTTP(w, r)
	})
}

func (a *Adaptador) handleIdentityChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorIdentidadDTO{ErrorCode: "identity.method_not_allowed"})
		return
	}
	var dto solicitudRetoIdentidadJSON
	if !decodificarIdentidad(w, r, &dto) {
		return
	}
	solicitud, err := dto.dominio()
	if err != nil || strings.TrimSpace(r.Header.Get("Origin")) != solicitud.Origen {
		writeJSON(w, http.StatusBadRequest, errorIdentidadDTO{ErrorCode: "identity.invalid_request"})
		return
	}
	reto, err := a.PrepararIdentidad.Preparar(r.Context(), solicitud)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorIdentidadDTO{ErrorCode: "identity.challenge_rejected"})
		return
	}
	writeJSON(w, http.StatusCreated, respuestaRetoIdentidadJSON{
		Contract: reto.Solicitud.Contrato, ChallengeID: reto.Solicitud.RetoID,
		IssuedAt: reto.Solicitud.EmitidoEn.Format(time.RFC3339Nano), ExpiresAt: reto.Solicitud.ExpiraEn.Format(time.RFC3339Nano),
		PolicyVersion:       reto.Solicitud.VersionPolitica,
		CanonicalPayloadB64: base64.StdEncoding.EncodeToString(reto.ContenidoCanonico),
	})
}

func (a *Adaptador) handleIdentityVerification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorIdentidadDTO{ErrorCode: "identity.method_not_allowed"})
		return
	}
	var dto solicitudVerificacionIdentidadJSON
	if !decodificarIdentidad(w, r, &dto) || dto.Contract != domain.VersionContratoIdentidadReforzada {
		writeJSON(w, http.StatusBadRequest, errorIdentidadDTO{ErrorCode: "identity.invalid_proof"})
		return
	}
	prueba, err := dto.dominio()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorIdentidadDTO{ErrorCode: "identity.invalid_proof"})
		return
	}
	resultado, err := a.ConfirmarIdentidad.Confirmar(r.Context(), prueba)
	if err != nil && resultado.Resultado == "" {
		writeJSON(w, http.StatusBadRequest, errorIdentidadDTO{ErrorCode: "identity.verification_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, nuevaRespuestaVerificacion(resultado))
}

type solicitudRetoIdentidadJSON struct {
	Contract          string `json:"contract"`
	ChallengeID       string `json:"challengeId"`
	Audience          string `json:"audience"`
	RegisteredClient  string `json:"registeredClient"`
	Purpose           string `json:"purpose"`
	Operation         string `json:"operation"`
	TenantContextHash string `json:"tenantContextHash"`
	SessionBinding    string `json:"sessionBinding"`
	Origin            string `json:"origin"`
	ConsentID         string `json:"consentId"`
	ConsentVersion    string `json:"consentVersion"`
	PolicyID          string `json:"policyId"`
	PolicyVersion     string `json:"policyVersion"`
	Nonce             string `json:"nonce"`
	IssuedAt          string `json:"issuedAt"`
	ExpiresAt         string `json:"expiresAt"`
}

type respuestaRetoIdentidadJSON struct {
	Contract            string `json:"contract"`
	ChallengeID         string `json:"challengeId"`
	IssuedAt            string `json:"issuedAt"`
	ExpiresAt           string `json:"expiresAt"`
	PolicyVersion       string `json:"policyVersion"`
	CanonicalPayloadB64 string `json:"canonicalPayloadB64"`
}

type solicitudVerificacionIdentidadJSON struct {
	Contract           string   `json:"contract"`
	ChallengeID        string   `json:"challengeId"`
	SignatureB64       string   `json:"signatureB64"`
	CertificateB64     string   `json:"certificateB64"`
	ChainB64           []string `json:"chainB64"`
	Format             string   `json:"format"`
	SignatureAlgorithm string   `json:"signatureAlgorithm"`
	DigestAlgorithm    string   `json:"digestAlgorithm"`
}

func (d solicitudRetoIdentidadJSON) dominio() (domain.SolicitudRetoIdentidad, error) {
	nonce, err := base64.RawURLEncoding.Strict().DecodeString(d.Nonce)
	if err != nil {
		return domain.SolicitudRetoIdentidad{}, err
	}
	emitido, err := instanteUTC(d.IssuedAt)
	if err != nil {
		return domain.SolicitudRetoIdentidad{}, err
	}
	expira, err := instanteUTC(d.ExpiresAt)
	if err != nil {
		return domain.SolicitudRetoIdentidad{}, err
	}
	return domain.SolicitudRetoIdentidad{Contrato: d.Contract, RetoID: d.ChallengeID,
		Audiencia: d.Audience, ClienteRegistrado: d.RegisteredClient, Finalidad: d.Purpose,
		Operacion: d.Operation, HuellaContextoTenant: d.TenantContextHash,
		VinculoSesion: d.SessionBinding, Origen: d.Origin, ConsentimientoID: d.ConsentID,
		VersionConsentimiento: d.ConsentVersion, PoliticaID: d.PolicyID,
		VersionPolitica: d.PolicyVersion, Nonce: nonce, EmitidoEn: emitido, ExpiraEn: expira}, nil
}

func (d solicitudVerificacionIdentidadJSON) dominio() (domain.PruebaIdentidad, error) {
	firma, err := base64.StdEncoding.Strict().DecodeString(d.SignatureB64)
	if err != nil {
		return domain.PruebaIdentidad{}, err
	}
	certificado, err := base64.StdEncoding.Strict().DecodeString(d.CertificateB64)
	if err != nil {
		return domain.PruebaIdentidad{}, err
	}
	cadena := make([][]byte, len(d.ChainB64))
	for i, codificado := range d.ChainB64 {
		cadena[i], err = base64.StdEncoding.Strict().DecodeString(codificado)
		if err != nil {
			return domain.PruebaIdentidad{}, err
		}
	}
	prueba := domain.PruebaIdentidad{RetoID: d.ChallengeID, Formato: d.Format,
		AlgoritmoFirma: d.SignatureAlgorithm, AlgoritmoHuella: d.DigestAlgorithm,
		Firma: firma, Certificado: certificado, Cadena: cadena}
	return prueba, prueba.Validar()
}

func instanteUTC(valor string) (time.Time, error) {
	instante, err := time.Parse(time.RFC3339Nano, valor)
	if err != nil || !strings.HasSuffix(valor, "Z") {
		return time.Time{}, domain.ErrSolicitudIdentidadInvalida
	}
	return instante.UTC(), nil
}

func decodificarIdentidad(w http.ResponseWriter, r *http.Request, destino any) bool {
	contenido, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximoCuerpoIdentidad))
	if err != nil || len(contenido) == 0 || jsonDuplicado(contenido) {
		writeJSON(w, http.StatusBadRequest, errorIdentidadDTO{ErrorCode: "identity.invalid_json"})
		return false
	}
	decodificador := json.NewDecoder(bytes.NewReader(contenido))
	decodificador.DisallowUnknownFields()
	if err := decodificador.Decode(destino); err != nil || decodificador.Decode(&struct{}{}) != io.EOF {
		writeJSON(w, http.StatusBadRequest, errorIdentidadDTO{ErrorCode: "identity.invalid_json"})
		return false
	}
	return true
}

func jsonDuplicado(contenido []byte) bool {
	decodificador := json.NewDecoder(bytes.NewReader(contenido))
	inicio, err := decodificador.Token()
	if err != nil || inicio != json.Delim('{') {
		return true
	}
	vistas := map[string]struct{}{}
	for decodificador.More() {
		token, err := decodificador.Token()
		clave, correcta := token.(string)
		if err != nil || !correcta {
			return true
		}
		if _, existe := vistas[clave]; existe {
			return true
		}
		vistas[clave] = struct{}{}
		var valor json.RawMessage
		if err := decodificador.Decode(&valor); err != nil {
			return true
		}
	}
	fin, err := decodificador.Token()
	if err != nil || fin != json.Delim('}') {
		return true
	}
	_, err = decodificador.Token()
	return err != io.EOF
}

func nuevaRespuestaVerificacion(r domain.ResultadoVerificacionIdentidad) respuestaVerificacionIdentidadDTO {
	return respuestaVerificacionIdentidadDTO{Outcome: string(r.Resultado), EvidenceRef: r.EvidenciaRef,
		Contract: r.Solicitud.Contrato, ChallengeID: r.RetoID, Audience: r.Solicitud.Audiencia,
		RegisteredClient: r.Solicitud.ClienteRegistrado, Purpose: r.Solicitud.Finalidad,
		Operation: r.Solicitud.Operacion, TenantContextHash: r.Solicitud.HuellaContextoTenant,
		SessionBinding: r.Solicitud.VinculoSesion, Origin: r.Solicitud.Origen,
		ConsentID: r.Solicitud.ConsentimientoID, ConsentVersion: r.Solicitud.VersionConsentimiento,
		PolicyID: r.Solicitud.PoliticaID, PolicyVersion: r.Solicitud.VersionPolitica,
		IssuedAt: r.Solicitud.EmitidoEn.Format(time.RFC3339Nano), ExpiresAt: r.Solicitud.ExpiraEn.Format(time.RFC3339Nano),
		AssuranceLevel: r.NivelAseguramiento, AuthenticationMethod: r.MetodoAutenticacion,
		Identity: r.IdentidadAcreditada, CertificateFingerprint: r.HuellaCertificado,
		Integrity: nuevoDictamen(r.Integridad), Chain: nuevoDictamen(r.Cadena), Validity: nuevoDictamen(r.Vigencia),
		KeyUsage: nuevoDictamen(r.EKU), CertificatePolicy: nuevoDictamen(r.PoliticaCertificado), Revocation: nuevoDictamen(r.Revocacion)}
}

func nuevoDictamen(d domain.DictamenComprobacion) dictamenIdentidadDTO {
	instante := ""
	if !d.ComprobadoEn.IsZero() {
		instante = d.ComprobadoEn.UTC().Format(time.RFC3339Nano)
	}
	return dictamenIdentidadDTO{Status: d.Estado, Source: d.Fuente, CheckedAt: instante}
}
