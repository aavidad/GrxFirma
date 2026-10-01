// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Cobertura del nucleo de dominio que no tocaba domain_test.go: perfiles de
// proteccion, modelo de resultado de verificacion y manifiestos de huellas.
// Son invariantes baratas de probar y caras de romper sin darse cuenta.
package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestProtectionProfile_Validate(t *testing.T) {
	t.Parallel()

	for _, perfil := range []ProtectionProfile{ProtectionProfileStrong, ProtectionProfileCompat} {
		if err := perfil.Validate(); err != nil {
			t.Errorf("perfil %q debio aceptarse: %v", perfil, err)
		}
	}
	for _, perfil := range []ProtectionProfile{"", "aes", "alto", ProtectionProfile(strings.ToUpper(string(ProtectionProfileStrong)))} {
		if err := perfil.Validate(); err == nil {
			t.Errorf("perfil %q debio rechazarse", perfil)
		}
	}
}

func TestProtectionRecipient_Supports(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre       string
		destinatario ProtectionRecipient
		perfil       ProtectionProfile
		soporta      bool
	}{
		{
			nombre:       "compat con clave RSA",
			destinatario: ProtectionRecipient{RSAOAEP256PublicKeyDER: []byte{1}},
			perfil:       ProtectionProfileCompat,
			soporta:      true,
		},
		{
			nombre:       "compat sin clave RSA",
			destinatario: ProtectionRecipient{},
			perfil:       ProtectionProfileCompat,
			soporta:      false,
		},
		{
			nombre:       "fuerte exige ML-KEM y X25519",
			destinatario: ProtectionRecipient{MLKEM768PublicKey: []byte{1}, X25519PublicKey: []byte{2}},
			perfil:       ProtectionProfileStrong,
			soporta:      true,
		},
		{
			nombre:       "fuerte con solo ML-KEM no basta",
			destinatario: ProtectionRecipient{MLKEM768PublicKey: []byte{1}},
			perfil:       ProtectionProfileStrong,
			soporta:      false,
		},
		{
			nombre:       "fuerte con solo X25519 no basta",
			destinatario: ProtectionRecipient{X25519PublicKey: []byte{2}},
			perfil:       ProtectionProfileStrong,
			soporta:      false,
		},
		{
			nombre:       "la clave RSA no habilita el perfil fuerte",
			destinatario: ProtectionRecipient{RSAOAEP256PublicKeyDER: []byte{1}},
			perfil:       ProtectionProfileStrong,
			soporta:      false,
		},
		{
			nombre:       "perfil desconocido nunca se soporta",
			destinatario: ProtectionRecipient{RSAOAEP256PublicKeyDER: []byte{1}, MLKEM768PublicKey: []byte{1}, X25519PublicKey: []byte{2}},
			perfil:       "inventado",
			soporta:      false,
		},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			if got := caso.destinatario.Supports(caso.perfil); got != caso.soporta {
				t.Fatalf("Supports(%q) = %v, se esperaba %v", caso.perfil, got, caso.soporta)
			}
		})
	}
}

func TestProtectionRecipient_Validate(t *testing.T) {
	t.Parallel()

	valido := ProtectionRecipient{ID: "dest-1", RSAOAEP256PublicKeyDER: []byte{1}}
	if err := valido.Validate(ProtectionProfileCompat); err != nil {
		t.Fatalf("destinatario valido rechazado: %v", err)
	}

	sinID := ProtectionRecipient{RSAOAEP256PublicKeyDER: []byte{1}}
	if err := sinID.Validate(ProtectionProfileCompat); err == nil {
		t.Error("un destinatario sin identificador debe rechazarse")
	}

	sinMaterial := ProtectionRecipient{ID: "dest-1"}
	if err := sinMaterial.Validate(ProtectionProfileCompat); err == nil {
		t.Error("un destinatario sin material para el perfil debe rechazarse")
	}

	// Tener material de un perfil no habilita el otro.
	soloRSA := ProtectionRecipient{ID: "dest-1", RSAOAEP256PublicKeyDER: []byte{1}}
	if err := soloRSA.Validate(ProtectionProfileStrong); err == nil {
		t.Error("material compat no debe validar el perfil fuerte")
	}
}

func TestProtectionKeyMaterial_Supports(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		material ProtectionKeyMaterial
		perfil   ProtectionProfile
		soporta  bool
	}{
		{"compat con RSA privada", ProtectionKeyMaterial{RSAOAEP256PrivateKeyPKCS8: []byte{1}}, ProtectionProfileCompat, true},
		{"compat con clave simetrica", ProtectionKeyMaterial{SymmetricKey: []byte{1}}, ProtectionProfileCompat, true},
		{"compat sin nada", ProtectionKeyMaterial{}, ProtectionProfileCompat, false},
		{"fuerte exige seed y X25519", ProtectionKeyMaterial{MLKEM768Seed: []byte{1}, X25519PrivateKey: []byte{2}}, ProtectionProfileStrong, true},
		{"fuerte con solo seed no basta", ProtectionKeyMaterial{MLKEM768Seed: []byte{1}}, ProtectionProfileStrong, false},
		{"perfil desconocido", ProtectionKeyMaterial{SymmetricKey: []byte{1}}, "inventado", false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			if got := caso.material.Supports(caso.perfil); got != caso.soporta {
				t.Fatalf("Supports(%q) = %v, se esperaba %v", caso.perfil, got, caso.soporta)
			}
		})
	}
}

func TestProtectionJob_Validate(t *testing.T) {
	t.Parallel()

	doc, err := NewDocument("a.pdf", []byte("contenido"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}

	if err := (ProtectionJob{Document: doc, Profile: ProtectionProfileStrong}).Validate(); err != nil {
		t.Errorf("trabajo valido rechazado: %v", err)
	}
	if err := (ProtectionJob{Document: doc, Profile: "inventado"}).Validate(); err == nil {
		t.Error("un perfil no soportado debe rechazarse")
	}
	if err := (ProtectionJob{Profile: ProtectionProfileStrong}).Validate(); err == nil {
		t.Error("un trabajo sin documento debe rechazarse")
	}
}

func TestNewVerificationSuccess(t *testing.T) {
	t.Parallel()

	res := NewVerificationSuccess("PAdES", "firma correcta", []string{"detalle"})

	if !res.Valid {
		t.Error("el resultado debe ser valido")
	}
	if res.Format != "PAdES" {
		t.Errorf("Format = %q", res.Format)
	}
	if res.Coverage != "full" {
		t.Errorf("Coverage = %q, se esperaba full", res.Coverage)
	}
	if res.Integrity.Status != VerificationStatusValid {
		t.Errorf("Integrity.Status = %q", res.Integrity.Status)
	}
	// Certificado y confianza quedan explicitamente en desconocido: un exito de
	// integridad no debe leerse como cadena de confianza comprobada.
	if res.Certificate.Status != VerificationStatusUnknown {
		t.Errorf("Certificate.Status = %q, se esperaba unknown", res.Certificate.Status)
	}
	if res.Trust.Status != VerificationStatusUnknown {
		t.Errorf("Trust.Status = %q, se esperaba unknown", res.Trust.Status)
	}
	if len(res.Errors) != 0 {
		t.Errorf("un exito no debe traer errores: %#v", res.Errors)
	}
}

func TestNewVerificationFailure(t *testing.T) {
	t.Parallel()

	res := NewVerificationFailure("CAdES", "firma alterada", []string{"detalle"})

	if res.Valid {
		t.Error("el resultado debe ser invalido")
	}
	if res.Coverage != "partial" {
		t.Errorf("Coverage = %q, se esperaba partial", res.Coverage)
	}
	if res.Integrity.Status != VerificationStatusInvalid {
		t.Errorf("Integrity.Status = %q", res.Integrity.Status)
	}
	// Normalize tambien anade el motivo a Errors; no debe duplicarse.
	if !reflect.DeepEqual(res.Errors, []string{"firma alterada"}) {
		t.Errorf("Errors = %#v, se esperaba exactamente un motivo", res.Errors)
	}
}

func TestVerificationResult_NoComparteElSliceDeDetalles(t *testing.T) {
	t.Parallel()

	detalles := []string{"original"}
	res := NewVerificationSuccess("PAdES", "ok", detalles)
	detalles[0] = "mutado"

	if res.Details[0] != "original" {
		t.Fatalf("el resultado comparte memoria con el slice del llamante: %#v", res.Details)
	}
	if res.Integrity.Details[0] != "original" {
		t.Fatalf("Integrity comparte memoria con el slice del llamante: %#v", res.Integrity.Details)
	}
}

func TestVerificationResult_AddDetail(t *testing.T) {
	t.Parallel()

	res := VerificationResult{}.AddDetail("primero")

	if !reflect.DeepEqual(res.Details, []string{"primero"}) {
		t.Errorf("Details = %#v", res.Details)
	}
	if !reflect.DeepEqual(res.Integrity.Details, []string{"primero"}) {
		t.Errorf("Integrity.Details = %#v", res.Integrity.Details)
	}
	// Un resultado sin estado explicito no debe quedar con la cadena vacia.
	if res.Integrity.Status != VerificationStatusUnknown {
		t.Errorf("Integrity.Status = %q, se esperaba unknown", res.Integrity.Status)
	}
}

func TestVerificationResult_WithSignerSummaries(t *testing.T) {
	t.Parallel()

	firmantes := []CertificateRef{
		{ID: "c1", Subject: "CN=Ana", Issuer: "CN=CA", Fingerprint: "aa"},
		{ID: "c2", Subject: "CN=Luis", Issuer: "CN=CA", Fingerprint: "bb"},
	}
	res := VerificationResult{}.WithSignerSummaries(firmantes)

	esperado := []VerificationSignerSummary{
		{ID: "c1", Subject: "CN=Ana", Issuer: "CN=CA", Fingerprint: "aa"},
		{ID: "c2", Subject: "CN=Luis", Issuer: "CN=CA", Fingerprint: "bb"},
	}
	if !reflect.DeepEqual(res.SignerSummaries, esperado) {
		t.Fatalf("SignerSummaries = %#v", res.SignerSummaries)
	}

	sinFirmantes := VerificationResult{Reason: "intacto"}.WithSignerSummaries(nil)
	if sinFirmantes.SignerSummaries != nil {
		t.Errorf("sin firmantes no debe inventarse un resumen: %#v", sinFirmantes.SignerSummaries)
	}
}

func TestVerificationResult_Normalize(t *testing.T) {
	t.Parallel()

	t.Run("rellena huecos de un resultado invalido", func(t *testing.T) {
		t.Parallel()
		res := VerificationResult{Valid: false, Reason: "sin cobertura"}.Normalize()

		if res.Coverage != "partial" {
			t.Errorf("Coverage = %q", res.Coverage)
		}
		if res.Integrity.Status != VerificationStatusInvalid {
			t.Errorf("Integrity.Status = %q", res.Integrity.Status)
		}
		if res.Integrity.Reason != "sin cobertura" {
			t.Errorf("Integrity.Reason = %q", res.Integrity.Reason)
		}
		if !reflect.DeepEqual(res.Errors, []string{"sin cobertura"}) {
			t.Errorf("Errors = %#v", res.Errors)
		}
	})

	t.Run("copia los detalles a integridad si faltan", func(t *testing.T) {
		t.Parallel()
		res := VerificationResult{Valid: true, Details: []string{"d1", "d2"}}.Normalize()
		if !reflect.DeepEqual(res.Integrity.Details, []string{"d1", "d2"}) {
			t.Errorf("Integrity.Details = %#v", res.Integrity.Details)
		}
	})

	t.Run("es idempotente", func(t *testing.T) {
		t.Parallel()
		una := NewVerificationFailure("XAdES", "motivo", []string{"d"})
		dos := una.Normalize()
		if !reflect.DeepEqual(una, dos) {
			t.Fatalf("Normalize no es idempotente:\n una = %#v\n dos = %#v", una, dos)
		}
	})

	t.Run("no pisa estados ya fijados", func(t *testing.T) {
		t.Parallel()
		res := VerificationResult{
			Valid:       true,
			Coverage:    "custom",
			Certificate: VerificationAspect{Status: VerificationStatusWarning},
			Trust:       VerificationAspect{Status: VerificationStatusInvalid},
		}.Normalize()

		if res.Coverage != "custom" {
			t.Errorf("Coverage = %q, no debia tocarse", res.Coverage)
		}
		if res.Certificate.Status != VerificationStatusWarning {
			t.Errorf("Certificate.Status = %q, no debia tocarse", res.Certificate.Status)
		}
		if res.Trust.Status != VerificationStatusInvalid {
			t.Errorf("Trust.Status = %q, no debia tocarse", res.Trust.Status)
		}
	})
}

func TestDirectoryHashManifestFormat_Validate(t *testing.T) {
	t.Parallel()

	for _, formato := range []DirectoryHashManifestFormat{DirectoryHashFormatXML, DirectoryHashFormatTXT, DirectoryHashFormatCSV} {
		if err := formato.Validate(); err != nil {
			t.Errorf("formato %q debio aceptarse: %v", formato, err)
		}
	}
	for _, formato := range []DirectoryHashManifestFormat{"", "json", "XML"} {
		if err := formato.Validate(); err == nil {
			t.Errorf("formato %q debio rechazarse", formato)
		}
	}
}

func TestDirectoryHashManifest_Validate(t *testing.T) {
	t.Parallel()

	valido := DirectoryHashManifest{
		Algorithm: "SHA-256",
		Entries:   []DirectoryHashEntry{{RelativePath: "a.txt", Digest: []byte{1}}},
	}
	if err := valido.Validate(); err != nil {
		t.Errorf("manifiesto valido rechazado: %v", err)
	}

	// Un manifiesto sin entradas es valido: describe un directorio vacio.
	if err := (DirectoryHashManifest{Algorithm: "SHA-256"}).Validate(); err != nil {
		t.Errorf("manifiesto sin entradas rechazado: %v", err)
	}

	if err := (DirectoryHashManifest{}).Validate(); err == nil {
		t.Error("un manifiesto sin algoritmo debe rechazarse")
	}
	sinRuta := DirectoryHashManifest{Algorithm: "SHA-256", Entries: []DirectoryHashEntry{{Digest: []byte{1}}}}
	if err := sinRuta.Validate(); err == nil {
		t.Error("una entrada sin ruta relativa debe rechazarse")
	}
	sinHuella := DirectoryHashManifest{Algorithm: "SHA-256", Entries: []DirectoryHashEntry{{RelativePath: "a.txt"}}}
	if err := sinHuella.Validate(); err == nil {
		t.Error("una entrada sin huella debe rechazarse")
	}
}

func TestDirectoryHashCheckReport_HasErrors(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		informe  DirectoryHashCheckReport
		conError bool
	}{
		{"todo casa", DirectoryHashCheckReport{MatchingHash: []string{"a.txt"}}, false},
		{"informe vacio", DirectoryHashCheckReport{}, false},
		{"huella distinta", DirectoryHashCheckReport{NotMatchingHash: []string{"a.txt"}}, true},
		{"huella sin fichero", DirectoryHashCheckReport{HashWithoutFile: []string{"a.txt"}}, true},
		{"fichero sin huella", DirectoryHashCheckReport{FileWithoutHash: []string{"a.txt"}}, true},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			if got := caso.informe.HasErrors(); got != caso.conError {
				t.Fatalf("HasErrors() = %v, se esperaba %v", got, caso.conError)
			}
		})
	}
}

func TestCertificateChain_IsEmpty(t *testing.T) {
	t.Parallel()

	if !(CertificateChain{}).IsEmpty() {
		t.Error("una cadena sin certificados esta vacia")
	}
	llena := CertificateChain{Certificates: []CertificateRef{{ID: "c1"}}}
	if llena.IsEmpty() {
		t.Error("una cadena con certificados no esta vacia")
	}
	conDER := CertificateChain{DERCertificates: [][]byte{{0x30, 0x00}}}
	if conDER.IsEmpty() {
		t.Error("una cadena con anclas DER no esta vacia")
	}
	conSistema := CertificateChain{UseSystemRoots: true}
	if conSistema.IsEmpty() {
		t.Error("una cadena que solicita las raíces del sistema no esta vacia")
	}
}

func TestCertificateRef_Validate_SinHuella(t *testing.T) {
	t.Parallel()

	if err := (CertificateRef{ID: "c1"}).Validate(); err == nil {
		t.Error("un certificado sin huella debe rechazarse")
	}
	if err := (CertificateRef{ID: "c1", Fingerprint: "aa"}).Validate(); err != nil {
		t.Errorf("certificado valido rechazado: %v", err)
	}
}

func TestTrustDecision_Validate(t *testing.T) {
	t.Parallel()

	for _, estado := range []TrustStatus{TrustAllowed, TrustDenied, TrustPending} {
		decision := TrustDecision{Origin: "https://sede.dipgra.es", Status: estado}
		if err := decision.Validate(); err != nil {
			t.Errorf("estado %q debio aceptarse: %v", estado, err)
		}
	}
	if err := (TrustDecision{Status: TrustAllowed}).Validate(); err == nil {
		t.Error("una decision sin origen debe rechazarse")
	}
	if err := (TrustDecision{Origin: "x", Status: "inventado"}).Validate(); err == nil {
		t.Error("un estado desconocido debe rechazarse")
	}
	if err := (TrustDecision{Origin: "x"}).Validate(); err == nil {
		t.Error("un estado vacio debe rechazarse")
	}
}

func TestExchangeSession_ValidateYEstado(t *testing.T) {
	t.Parallel()

	completa := ExchangeSession{
		RequestID:        "req-1",
		UploadEndpoint:   "opaque://subir",
		RetrieveEndpoint: "opaque://recuperar",
		State:            SessionActive,
	}
	if err := completa.Validate(); err != nil {
		t.Errorf("sesion valida rechazada: %v", err)
	}
	if !completa.IsActive() {
		t.Error("la sesion debe estar activa")
	}
	if completa.WithState(SessionCompleted).IsActive() {
		t.Error("una sesion completada no esta activa")
	}

	sinSubida := completa
	sinSubida.UploadEndpoint = ""
	if err := sinSubida.Validate(); err == nil {
		t.Error("falta el endpoint de subida y debe rechazarse")
	}
	sinDescarga := completa
	sinDescarga.RetrieveEndpoint = ""
	if err := sinDescarga.Validate(); err == nil {
		t.Error("falta el endpoint de descarga y debe rechazarse")
	}
}

func TestBatchJob_ValidateSenalaElTrabajoQueFalla(t *testing.T) {
	t.Parallel()

	doc, err := NewDocument("a.pdf", []byte("contenido"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	bueno := SignatureJob{Document: doc, Format: FormatPAdES, Action: ActionSign}
	malo := SignatureJob{Document: doc, Format: FormatPAdES, Action: "inventada"}

	lote := BatchJob{Jobs: []SignatureJob{bueno, bueno, malo}}
	err = lote.Validate()
	if err == nil {
		t.Fatal("el lote con un trabajo invalido debe rechazarse")
	}
	// itoa construye el indice sin importar strconv; comprobamos que senala el 2.
	if !strings.Contains(err.Error(), "trabajo 2 del lote") {
		t.Fatalf("el error debe identificar el trabajo que falla, y fue: %v", err)
	}

	if err := (BatchJob{Jobs: []SignatureJob{bueno}}).Validate(); err != nil {
		t.Errorf("lote valido rechazado: %v", err)
	}
}
