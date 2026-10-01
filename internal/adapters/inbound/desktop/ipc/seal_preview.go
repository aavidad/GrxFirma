// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
)

// paramsVistaPreviaSello usa los mismos campos que la firma para que la
// vista previa sea exactamente el sello que se incrustará.
type paramsVistaPreviaSello struct {
	CertificateID string            `json:"certificateId,omitempty"`
	VisibleSeal   map[string]any    `json:"visibleSeal"`
	QRContent     string            `json:"qrContent,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	Location      string            `json:"location,omitempty"`
	ContactInfo   string            `json:"contactInfo,omitempty"`
	ExtraOptions  map[string]string `json:"extraOptions,omitempty"`
}

type resultadoVistaPreviaSello struct {
	Image string `json:"image"`
}

// handleVistaPreviaSello genera el PNG del sello visible con el diseño real,
// el nombre del certificado elegido y las opciones de la pantalla de firma.
func (m *Manejador) handleVistaPreviaSello(ctx context.Context, raw json.RawMessage) respuesta {
	const accion = "seal_preview"
	var p paramsVistaPreviaSello
	if err := json.Unmarshal(raw, &p); err != nil || len(p.VisibleSeal) == 0 {
		return respuesta{OK: false, Action: accion, Error: m.t("error.formato_invalido")}
	}
	opciones, err := construirOpcionesFirmaIPCBase(false, false, p.VisibleSeal, p.QRContent, p.Reason, p.Location, p.ContactInfo, p.ExtraOptions, "pades")
	if err != nil {
		return respuesta{OK: false, Action: accion, Error: m.localizarErrorOpacidadLogoSello(err)}
	}
	firmante, emisor := m.identidadParaSello(ctx, p.CertificateID)
	img, err := desktopsigner.PrevisualizarSello(opciones, firmante, emisor, time.Now())
	if err != nil {
		return respuesta{OK: false, Action: accion, Error: err.Error()}
	}
	return respuesta{OK: true, Action: accion, Data: resultadoVistaPreviaSello{Image: base64.StdEncoding.EncodeToString(img)}}
}

// identidadParaSello devuelve el nombre y el emisor del certificado; sin
// certificado elegido se muestra un texto de ejemplo.
func (m *Manejador) identidadParaSello(ctx context.Context, id string) (string, string) {
	const ejemplo = "Nombre del firmante"
	id = strings.TrimSpace(id)
	if id == "" || m.Catalogo == nil {
		return ejemplo, ""
	}
	certs, err := m.Catalogo.List(ctx)
	if err != nil {
		return ejemplo, ""
	}
	for _, c := range certs {
		if c.ID != id {
			continue
		}
		if cert, err := x509.ParseCertificate(c.DER); err == nil {
			nombre := strings.TrimSpace(cert.Subject.CommonName)
			if nombre == "" {
				nombre = cert.Subject.String()
			}
			emisor := strings.TrimSpace(strings.Join(cert.Issuer.Organization, " "))
			if emisor == "" {
				emisor = cert.Issuer.CommonName
			}
			return nombre, emisor
		}
		if s := strings.TrimSpace(c.Subject); s != "" {
			return s, ""
		}
	}
	return ejemplo, ""
}
