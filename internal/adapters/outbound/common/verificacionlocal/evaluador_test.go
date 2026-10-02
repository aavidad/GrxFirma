// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal_test

import (
	"context"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	pdfsign "github.com/digitorus/pdfsign/sign"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/verificacionlocal"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/testsupport/pdffixture"
)

type escenario struct {
	pki      pkiPrueba
	anclas   string
	crlDir   string
	original []byte
}

// nuevoEscenario prepara anclas (la raíz) y un directorio de CRL con la CRL
// de la intermedia y la ARL de la raíz, ambas vigentes y sin revocaciones.
func nuevoEscenario(t *testing.T) escenario {
	t.Helper()
	p := nuevaPKI(t, "Escenario")
	dir := t.TempDir()
	crlDir := filepath.Join(dir, "crl")
	if err := os.Mkdir(crlDir, 0o700); err != nil {
		t.Fatal(err)
	}
	escribir(t, crlDir, "intermedia.crl", crearCRL(t, p.intermedia, opcionesCRL{}))
	escribir(t, crlDir, "raiz.crl", crearCRL(t, p.raiz, opcionesCRL{}))
	return escenario{
		pki:      p,
		anclas:   escribir(t, dir, "anclas.pem", pemCert(p.raiz.cert)),
		crlDir:   crlDir,
		original: []byte("Documento original sintético para verificación autónoma"),
	}
}

func (e escenario) clave() *commonsigner.LocalSigningKey {
	return &commonsigner.LocalSigningKey{
		ID:          "sintetica",
		Signer:      e.pki.firmante.clave,
		Certificate: e.pki.firmante.cert,
		Chain:       []*x509.Certificate{e.pki.intermedia.cert},
	}
}

func (e escenario) firmarCAdES(t *testing.T, tsa ports.TimestampAuthority) []byte {
	t.Helper()
	doc, err := domain.NewDocument("original.txt", e.original, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	job := domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}
	var res domain.SignatureResult
	if tsa != nil {
		res, err = commonsigner.NewSignerCAdEST(commonsigner.NewCAdESBESDetached(), tsa).Sign(context.Background(), job, e.clave())
	} else {
		res, err = commonsigner.NewCAdESBESDetached().Sign(context.Background(), job, e.clave())
	}
	if err != nil {
		t.Fatalf("firmando CAdES sintético: %v", err)
	}
	return res.Data
}

func (e escenario) firmarPAdES(t *testing.T) (original, firmado []byte) {
	t.Helper()
	dir := t.TempDir()
	original = pdffixture.Minimal()
	entrada := escribir(t, dir, "original.pdf", original)
	salida := filepath.Join(dir, "firmado.pdf")
	err := pdfsign.SignFile(entrada, salida, pdfsign.SignData{
		Signature: pdfsign.SignDataSignature{
			Info:       pdfsign.SignDataSignatureInfo{Name: "Firmante sintético", Date: time.Now()},
			CertType:   pdfsign.ApprovalSignature,
			DocMDPPerm: pdfsign.AllowFillingExistingFormFieldsAndSignaturesPerms,
			SubFilter:  pdfsign.SignatureSubFilterETSICAdESDetached,
		},
		Signer:            e.pki.firmante.clave,
		DigestAlgorithm:   crypto.SHA256,
		Certificate:       e.pki.firmante.cert,
		CertificateChains: [][]*x509.Certificate{{e.pki.firmante.cert, e.pki.intermedia.cert}},
	})
	if err != nil {
		t.Fatalf("firmando PAdES sintético: %v", err)
	}
	firmado, err = os.ReadFile(salida)
	if err != nil {
		t.Fatal(err)
	}
	return original, firmado
}

type opcionesVerificacion struct {
	anclas string
	crlDir string
	cfg    verificacionlocal.Configuracion
}

// verificar compone el caso de uso real (verificador de formato sin red,
// anclas locales y evaluador) y devuelve el dictamen.
func verificar(t *testing.T, op opcionesVerificacion, nombre, mime string, firmado, original []byte) application.VerifyResult {
	t.Helper()
	anclas, err := verificacionlocal.CargarAnclas(op.anclas)
	if err != nil {
		t.Fatalf("cargando anclas: %v", err)
	}
	cfg := op.cfg
	if op.crlDir != "" {
		cfg.CRL, err = verificacionlocal.NuevoAlmacenCRL(op.crlDir)
		if err != nil {
			t.Fatal(err)
		}
	}
	uc := application.NuevoVerifySignatureUseCase(anclas, commonsigner.NewMultiVerifierOffline(), nil).
		ConEvaluador(verificacionlocal.Nuevo(cfg))
	doc, err := domain.NewDocument(nombre, firmado, mime)
	if err != nil {
		t.Fatal(err)
	}
	cmd := application.VerifyCommand{SignedDocument: doc}
	if original != nil {
		orig, err := domain.NewDocument(nombre, original, "application/octet-stream")
		if err != nil {
			t.Fatal(err)
		}
		cmd.OriginalDocument = &orig
	}
	res, err := uc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("verificación: %v", err)
	}
	if res.Dictamen == nil {
		t.Fatal("falta el dictamen")
	}
	return res
}

func huella(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func exigir(t *testing.T, d *domain.DictamenVerificacion, estado domain.EstadoDictamen, motivo domain.MotivoDictamen) {
	t.Helper()
	if d.Estado != estado || d.Motivo != motivo {
		t.Fatalf("dictamen=%s/%s, se esperaba %s/%s\n%+v", d.Estado, d.Motivo, estado, motivo, *d)
	}
	if d.Contrato != domain.ContratoDictamenVerificacion {
		t.Fatalf("contrato=%q", d.Contrato)
	}
}

func TestDictamen_CAdESDetachedValidaConFuentesLocales(t *testing.T) {
	e := nuevoEscenario(t)
	firmado := e.firmarCAdES(t, nil)
	res := verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: e.crlDir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
	d := res.Dictamen
	exigir(t, d, domain.EstadoDictamenValida, domain.MotivoDictamenVerificada)
	if d.Integridad.Estado != domain.IntegridadValida || d.Cadena.Estado != domain.CadenaValida ||
		d.Revocacion.Estado != domain.RevocacionVigente || d.Revocacion.Fuente != "crl_local" ||
		d.SelloTiempo.Estado != domain.SelloNoPresente || d.VinculoOriginal.Estado != domain.VinculoAcreditado ||
		d.Certificado.Estado != domain.CertificadoVigente {
		t.Fatalf("aspectos inesperados: %+v", *d)
	}
	if d.HuellaFirmadoSHA256 != huella(firmado) || d.HuellaOriginalSHA256 != huella(e.original) {
		t.Fatal("las huellas de eco no corresponden a los contenidos enviados")
	}
	if d.CertificadoHuellaSHA256 != huella(e.pki.firmante.cert.Raw) {
		t.Fatal("la huella del certificado firmante no corresponde")
	}
	if d.Extensiones.RevocacionRemota != domain.ExtensionDesactivada || d.Extensiones.SelloTiempoRemoto != domain.ExtensionDesactivada {
		t.Fatalf("las extensiones remotas deben estar desactivadas por defecto: %+v", d.Extensiones)
	}
}

func TestDictamen_RevocacionNoConcluyenteNuncaEsValida(t *testing.T) {
	e := nuevoEscenario(t)
	firmado := e.firmarCAdES(t, nil)

	t.Run("sin_directorio_de_crl", func(t *testing.T) {
		res := verificar(t, opcionesVerificacion{anclas: e.anclas}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
		exigir(t, res.Dictamen, domain.EstadoDictamenIndeterminada, domain.MotivoDictamenRevocacionNoAcreditada)
		if res.Dictamen.Revocacion.Estado != domain.RevocacionNoComprobada || res.Dictamen.Revocacion.Motivo != "sin_crl_locales" {
			t.Fatalf("revocación=%+v", res.Dictamen.Revocacion)
		}
		// El aspecto heredado tampoco puede presentar el certificado como válido.
		if res.Verification.Certificate.Status == domain.VerificationStatusValid {
			t.Fatalf("certificate=valid sin revocación concluyente: %+v", res.Verification.Certificate)
		}
	})

	t.Run("falta_la_crl_de_la_raiz", func(t *testing.T) {
		dir := t.TempDir()
		escribir(t, dir, "intermedia.crl", crearCRL(t, e.pki.intermedia, opcionesCRL{}))
		res := verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: dir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
		exigir(t, res.Dictamen, domain.EstadoDictamenIndeterminada, domain.MotivoDictamenRevocacionNoAcreditada)
		if res.Dictamen.Revocacion.Motivo != "intermedio_sin_crl_del_emisor" {
			t.Fatalf("motivo=%q", res.Dictamen.Revocacion.Motivo)
		}
	})

	t.Run("crl_caducada", func(t *testing.T) {
		dir := t.TempDir()
		escribir(t, dir, "intermedia.crl", crearCRL(t, e.pki.intermedia, opcionesCRL{
			thisUpdate: time.Now().Add(-72 * time.Hour), nextUpdate: time.Now().Add(-48 * time.Hour),
		}))
		escribir(t, dir, "raiz.crl", crearCRL(t, e.pki.raiz, opcionesCRL{}))
		res := verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: dir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
		exigir(t, res.Dictamen, domain.EstadoDictamenIndeterminada, domain.MotivoDictamenRevocacionNoAcreditada)
		if res.Dictamen.Revocacion.Motivo != "crl_caducada" {
			t.Fatalf("motivo=%q", res.Dictamen.Revocacion.Motivo)
		}
	})

	t.Run("crl_de_otra_particion", func(t *testing.T) {
		dir := t.TempDir()
		escribir(t, dir, "intermedia.crl", crearCRL(t, e.pki.intermedia, opcionesCRL{
			extra: []pkix.Extension{extensionIDP(t, "http://crl.invalid/particion-7.crl")},
		}))
		escribir(t, dir, "raiz.crl", crearCRL(t, e.pki.raiz, opcionesCRL{}))
		res := verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: dir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
		exigir(t, res.Dictamen, domain.EstadoDictamenIndeterminada, domain.MotivoDictamenRevocacionNoAcreditada)
		if res.Dictamen.Revocacion.Motivo != "crl_de_otra_particion" {
			t.Fatalf("motivo=%q", res.Dictamen.Revocacion.Motivo)
		}
	})

	t.Run("crl_de_la_particion_del_certificado", func(t *testing.T) {
		dir := t.TempDir()
		escribir(t, dir, "intermedia.crl", crearCRL(t, e.pki.intermedia, opcionesCRL{
			extra: []pkix.Extension{extensionIDP(t, "http://crl.invalid/intermedia.crl")},
		}))
		escribir(t, dir, "raiz.crl", crearCRL(t, e.pki.raiz, opcionesCRL{}))
		res := verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: dir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
		exigir(t, res.Dictamen, domain.EstadoDictamenValida, domain.MotivoDictamenVerificada)
	})
}

func TestDictamen_CertificadoRevocadoNoEsValido(t *testing.T) {
	e := nuevoEscenario(t)
	firmado := e.firmarCAdES(t, nil)
	dir := t.TempDir()
	escribir(t, dir, "intermedia.crl", crearCRL(t, e.pki.intermedia, opcionesCRL{revocados: []*big.Int{e.pki.firmante.cert.SerialNumber}}))
	escribir(t, dir, "raiz.crl", crearCRL(t, e.pki.raiz, opcionesCRL{}))
	res := verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: dir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
	exigir(t, res.Dictamen, domain.EstadoDictamenNoValida, domain.MotivoDictamenCertificadoNoValido)
	if res.Dictamen.Revocacion.Estado != domain.RevocacionRevocado || res.Dictamen.Revocacion.Fecha.IsZero() {
		t.Fatalf("revocación=%+v", res.Dictamen.Revocacion)
	}
}

func TestDictamen_AnclasAjenasDejanLaConfianzaSinAcreditar(t *testing.T) {
	e := nuevoEscenario(t)
	firmado := e.firmarCAdES(t, nil)
	otra := nuevaPKI(t, "Ajena")
	anclas := escribir(t, t.TempDir(), "ajena.pem", pemCert(otra.raiz.cert))
	res := verificar(t, opcionesVerificacion{anclas: anclas, crlDir: e.crlDir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
	exigir(t, res.Dictamen, domain.EstadoDictamenIndeterminada, domain.MotivoDictamenConfianzaNoAcreditada)
	if res.Dictamen.Cadena.Motivo != "sin_cadena_hasta_ancla" || res.Dictamen.Revocacion.Motivo != "cadena_no_construida" {
		t.Fatalf("cadena=%+v revocación=%+v", res.Dictamen.Cadena, res.Dictamen.Revocacion)
	}
}

func TestDictamen_SelloDeTiempo(t *testing.T) {
	e := nuevoEscenario(t)

	t.Run("valido", func(t *testing.T) {
		firmado := e.firmarCAdES(t, tsaSintetica{emisor: e.pki.tsa})
		res := verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: e.crlDir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
		exigir(t, res.Dictamen, domain.EstadoDictamenValida, domain.MotivoDictamenVerificada)
		if res.Dictamen.SelloTiempo.Estado != domain.SelloValido || res.Dictamen.SelloTiempo.Fecha.IsZero() {
			t.Fatalf("sello=%+v", res.Dictamen.SelloTiempo)
		}
	})

	t.Run("tsa_fuera_de_las_anclas_no_bloquea", func(t *testing.T) {
		ajena := nuevaPKI(t, "TSA ajena")
		firmado := e.firmarCAdES(t, tsaSintetica{emisor: ajena.tsa})
		res := verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: e.crlDir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
		exigir(t, res.Dictamen, domain.EstadoDictamenValida, domain.MotivoDictamenVerificada)
		if res.Dictamen.SelloTiempo.Estado != domain.SelloNoComprobado || res.Dictamen.SelloTiempo.Motivo != "tsa_sin_cadena_hasta_ancla" {
			t.Fatalf("sello=%+v", res.Dictamen.SelloTiempo)
		}
	})

	t.Run("sello_ajeno_a_la_firma", func(t *testing.T) {
		firmado := e.firmarCAdES(t, tsaSintetica{emisor: e.pki.tsa, alterar: true})
		res := verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: e.crlDir}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
		exigir(t, res.Dictamen, domain.EstadoDictamenIndeterminada, domain.MotivoDictamenSelloTiempoNoAcreditado)
		if res.Dictamen.SelloTiempo.Estado != domain.SelloNoValido || res.Dictamen.SelloTiempo.Motivo != "sello_no_corresponde_a_la_firma" {
			t.Fatalf("sello=%+v", res.Dictamen.SelloTiempo)
		}
	})
}

func TestDictamen_PAdESVinculoConElOriginal(t *testing.T) {
	e := nuevoEscenario(t)
	original, firmado := e.firmarPAdES(t)
	op := opcionesVerificacion{anclas: e.anclas, crlDir: e.crlDir}

	t.Run("sin_crl_local", func(t *testing.T) {
		res := verificar(t, opcionesVerificacion{anclas: e.anclas}, "firmado.pdf", "application/pdf", firmado, original)
		exigir(t, res.Dictamen, domain.EstadoDictamenIndeterminada, domain.MotivoDictamenRevocacionNoAcreditada)
		if res.Verification.Valid || res.Verification.Reason != "revocación no concluyente" {
			t.Fatalf("resultado heredado sin revocación concluyente: %+v", res.Verification)
		}
	})

	t.Run("original_aportado", func(t *testing.T) {
		res := verificar(t, op, "firmado.pdf", "application/pdf", firmado, original)
		exigir(t, res.Dictamen, domain.EstadoDictamenValida, domain.MotivoDictamenVerificada)
		if res.Dictamen.VinculoOriginal.Estado != domain.VinculoAcreditado || res.Dictamen.Formato != string(domain.FormatPAdES) {
			t.Fatalf("vínculo=%+v formato=%s", res.Dictamen.VinculoOriginal, res.Dictamen.Formato)
		}
	})

	t.Run("sin_original", func(t *testing.T) {
		res := verificar(t, op, "firmado.pdf", "application/pdf", firmado, nil)
		exigir(t, res.Dictamen, domain.EstadoDictamenValida, domain.MotivoDictamenVerificada)
		if res.Dictamen.VinculoOriginal.Estado != domain.VinculoNoAportado || res.Dictamen.HuellaOriginalSHA256 != "" {
			t.Fatalf("vínculo=%+v", res.Dictamen.VinculoOriginal)
		}
	})

	t.Run("original_distinto", func(t *testing.T) {
		otro := append([]byte(nil), original...)
		otro[len(otro)/2] ^= 0x01
		res := verificar(t, op, "firmado.pdf", "application/pdf", firmado, otro)
		exigir(t, res.Dictamen, domain.EstadoDictamenIndeterminada, domain.MotivoDictamenVinculoOriginalNoAcreditado)
		if res.Dictamen.VinculoOriginal.Motivo != "original_no_es_revision_previa" {
			t.Fatalf("vínculo=%+v", res.Dictamen.VinculoOriginal)
		}
	})
}

func TestDictamen_XAdESConCRLSintetica(t *testing.T) {
	e := nuevoEscenario(t)
	original := []byte("<expediente><estado>aprobado</estado></expediente>")
	doc, err := domain.NewDocument("expediente.xml", original, "application/xml")
	if err != nil {
		t.Fatal(err)
	}
	firmado, err := commonsigner.NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign,
		Options: map[string]string{"format": "XAdES Enveloping"},
	}, e.clave())
	if err != nil {
		t.Fatal(err)
	}
	// XAdES KeyInfo solo embebe la hoja; la intermedia se configura como
	// ancla explícita para acreditar la ruta sin descargar emisores.
	anclasXML := escribir(t, t.TempDir(), "anclas-xml.pem", append(pemCert(e.pki.raiz.cert), pemCert(e.pki.intermedia.cert)...))
	res := verificar(t, opcionesVerificacion{anclas: anclasXML, crlDir: e.crlDir}, "expediente.xsig", "application/xml", firmado.Data, nil)
	exigir(t, res.Dictamen, domain.EstadoDictamenValida, domain.MotivoDictamenVerificada)
	if res.Dictamen.Revocacion.Estado != domain.RevocacionVigente || res.Dictamen.Revocacion.Fuente != "crl_local" {
		t.Fatalf("la CRL local no acreditó XAdES: %+v", res.Dictamen.Revocacion)
	}
}

// revocacionRemotaFalsa comprueba que el punto de extensión solo se usa
// cuando se configura expresamente.
type revocacionRemotaFalsa struct {
	llamadas *int
	crl      []byte
}

func (r revocacionRemotaFalsa) Fetch(context.Context, *x509.Certificate, *x509.Certificate) (ports.RevocationEvidence, error) {
	*r.llamadas++
	return ports.RevocationEvidence{CRLs: [][]byte{r.crl}}, nil
}

func TestDictamen_ExtensionRevocacionRemotaDesactivadaPorDefecto(t *testing.T) {
	e := nuevoEscenario(t)
	firmado := e.firmarCAdES(t, nil)
	llamadas := 0
	remota := revocacionRemotaFalsa{llamadas: &llamadas, crl: crearCRL(t, e.pki.intermedia, opcionesCRL{})}

	res := verificar(t, opcionesVerificacion{anclas: e.anclas}, "firma.csig", "application/pkcs7-signature", firmado, e.original)
	if llamadas != 0 || res.Dictamen.Extensiones.RevocacionRemota != domain.ExtensionDesactivada {
		t.Fatal("la extensión remota no debe consultarse si no se configura")
	}

	// Activada en código, complementa a las CRL locales: aquí aporta la CRL
	// de la intermedia y la local aporta la ARL de la raíz.
	dir := t.TempDir()
	escribir(t, dir, "raiz.crl", crearCRL(t, e.pki.raiz, opcionesCRL{}))
	res = verificar(t, opcionesVerificacion{anclas: e.anclas, crlDir: dir, cfg: verificacionlocal.Configuracion{RevocacionRemota: remota}},
		"firma.csig", "application/pkcs7-signature", firmado, e.original)
	exigir(t, res.Dictamen, domain.EstadoDictamenValida, domain.MotivoDictamenVerificada)
	if llamadas != 1 || res.Dictamen.Extensiones.RevocacionRemota != domain.ExtensionActiva || res.Dictamen.Revocacion.Fuente != "crl_remota" {
		t.Fatalf("llamadas=%d revocación=%+v", llamadas, res.Dictamen.Revocacion)
	}
}

func TestCargarAnclas_DirectorioYFicheroInvalido(t *testing.T) {
	a := nuevaPKI(t, "A")
	b := nuevaPKI(t, "B")
	dir := t.TempDir()
	escribir(t, dir, "a.pem", pemCert(a.raiz.cert))
	escribir(t, dir, "b.der", b.raiz.cert.Raw)
	escribir(t, dir, "duplicada.crt", pemCert(a.raiz.cert))
	escribir(t, dir, "notas.txt", []byte("ignorado"))
	anclas, err := verificacionlocal.CargarAnclas(dir)
	if err != nil {
		t.Fatal(err)
	}
	if anclas.Cantidad() != 2 {
		t.Fatalf("anclas=%d, se esperaban 2 distintas", anclas.Cantidad())
	}
	cadena, _ := anclas.Anchors(context.Background())
	if cadena.UseSystemRoots {
		t.Fatal("las anclas locales nunca habilitan el almacén del sistema")
	}
	if _, err := verificacionlocal.CargarAnclas(escribir(t, t.TempDir(), "vacio.pem", []byte("sin certificados"))); err == nil {
		t.Fatal("un fichero sin certificados debe rechazarse")
	}
}
