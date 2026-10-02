// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

type VerificationAspectStatus string

const (
	VerificationStatusUnknown VerificationAspectStatus = "unknown"
	VerificationStatusValid   VerificationAspectStatus = "valid"
	VerificationStatusInvalid VerificationAspectStatus = "invalid"
	VerificationStatusWarning VerificationAspectStatus = "warning"
)

type VerificationAspect struct {
	Status  VerificationAspectStatus `json:"status"`
	Reason  string                   `json:"reason,omitempty"`
	Details []string                 `json:"details,omitempty"`
}

type VerificationSignerSummary struct {
	ID          string `json:"id,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Issuer      string `json:"issuer,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type VerificationEvidence struct {
	Type    string `json:"type"`
	Summary string `json:"summary,omitempty"`
}

// VerificationResult es el resultado de verificar una firma electronica.
// Mantiene los campos legacy (Valid/Reason/Details) y añade una capa más rica
// para superficies avanzadas de producto sin romper compatibilidad.
type VerificationResult struct {
	Valid   bool
	Reason  string
	Details []string

	Format   string
	Coverage string

	Integrity   VerificationAspect
	Certificate VerificationAspect
	Trust       VerificationAspect

	SignerSummaries []VerificationSignerSummary
	Warnings        []string
	Errors          []string
	Evidence        []VerificationEvidence

	// Material conserva certificados, sellos y coberturas halladas para la
	// evaluación autónoma posterior. Nunca se serializa.
	Material MaterialVerificacion `json:"-"`
}

func NewVerificationSuccess(format, reason string, details []string) VerificationResult {
	out := VerificationResult{
		Valid:    true,
		Reason:   reason,
		Details:  append([]string(nil), details...),
		Format:   format,
		Coverage: "full",
		Integrity: VerificationAspect{
			Status:  VerificationStatusValid,
			Reason:  reason,
			Details: append([]string(nil), details...),
		},
		Certificate: VerificationAspect{Status: VerificationStatusUnknown},
		Trust:       VerificationAspect{Status: VerificationStatusUnknown},
	}
	return out.Normalize()
}

func NewVerificationFailure(format, reason string, details []string) VerificationResult {
	out := VerificationResult{
		Valid:    false,
		Reason:   reason,
		Details:  append([]string(nil), details...),
		Format:   format,
		Coverage: "partial",
		Integrity: VerificationAspect{
			Status:  VerificationStatusInvalid,
			Reason:  reason,
			Details: append([]string(nil), details...),
		},
		Certificate: VerificationAspect{Status: VerificationStatusUnknown},
		Trust:       VerificationAspect{Status: VerificationStatusUnknown},
		Errors:      []string{reason},
	}
	return out.Normalize()
}

func (v VerificationResult) AddDetail(detail string) VerificationResult {
	v.Details = append(v.Details, detail)
	if v.Integrity.Status == "" {
		v.Integrity.Status = VerificationStatusUnknown
	}
	v.Integrity.Details = append(v.Integrity.Details, detail)
	return v
}

func (v VerificationResult) WithSignerSummaries(signers []CertificateRef) VerificationResult {
	if len(signers) == 0 {
		return v
	}
	v.SignerSummaries = make([]VerificationSignerSummary, 0, len(signers))
	for _, signer := range signers {
		v.SignerSummaries = append(v.SignerSummaries, VerificationSignerSummary{
			ID:          signer.ID,
			Subject:     signer.Subject,
			Issuer:      signer.Issuer,
			Fingerprint: signer.Fingerprint,
		})
	}
	return v
}

func (v VerificationResult) Normalize() VerificationResult {
	if v.Valid {
		if v.Coverage == "" {
			v.Coverage = "full"
		}
	} else if v.Coverage == "" {
		v.Coverage = "partial"
	}
	if v.Integrity.Status == "" {
		if v.Valid {
			v.Integrity.Status = VerificationStatusValid
		} else {
			v.Integrity.Status = VerificationStatusInvalid
		}
	}
	if v.Integrity.Reason == "" {
		v.Integrity.Reason = v.Reason
	}
	if len(v.Integrity.Details) == 0 && len(v.Details) > 0 {
		v.Integrity.Details = append([]string(nil), v.Details...)
	}
	if v.Certificate.Status == "" {
		v.Certificate.Status = VerificationStatusUnknown
	}
	if v.Trust.Status == "" {
		v.Trust.Status = VerificationStatusUnknown
	}
	if !v.Valid && v.Reason != "" && len(v.Errors) == 0 {
		v.Errors = append(v.Errors, v.Reason)
	}
	return v
}
