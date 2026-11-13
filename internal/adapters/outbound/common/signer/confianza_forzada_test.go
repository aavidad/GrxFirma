// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"

	"grxfirma/internal/domain"
)

// Lote 14: el usuario puede aceptar en su equipo cinco obstáculos que hoy
// dejan la firma como no válida. Estas pruebas comprueban que, aceptado, la
// firma sale válida con el aviso a la vista, y que lo que nunca se puede
// forzar sigue sin ser válido aunque el usuario lo acepte todo.

type verificacionConContexto func(ctx context.Context) (domain.VerificationResult, error)

// verificarAceptando verifica primero sin aceptar nada y luego repite
// aceptando cada obstáculo que la verificación ofrezca, como un usuario que
// pulsa «Aceptar en este equipo» todas las veces. inicial son aceptaciones
// previas (por ejemplo, los dos casos globales).
func verificarAceptando(t *testing.T, inicial []domain.AceptacionConfianza, verificar verificacionConContexto) (sin, con domain.VerificationResult, errSin, errCon error, obstaculos []domain.ObstaculoConfianza) {
	t.Helper()
	registro := domain.NuevoRegistroConfianzaForzada(domain.ConfianzaForzada{})
	sin, errSin = verificar(domain.ConRegistroConfianzaForzada(context.Background(), registro))
	aceptadas := append([]domain.AceptacionConfianza(nil), inicial...)
	obstaculos = registro.Obstaculos()
	for ronda := 0; ronda < 5; ronda++ {
		for _, o := range registro.Obstaculos() {
			aceptadas = append(aceptadas, domain.AceptacionConfianza{Tipo: o.Tipo, Huella: o.Huella, AceptadaEn: time.Now()})
		}
		registro = registro.Derivar(domain.ConfianzaForzada{Aceptaciones: aceptadas})
		con, errCon = verificar(domain.ConRegistroConfianzaForzada(context.Background(), registro))
		nuevos := registro.Obstaculos()
		if len(nuevos) == 0 {
			return
		}
		obstaculos = append(obstaculos, nuevos...)
	}
	return
}

// aceptandoTodo acepta además los dos casos globales desde el principio.
func aceptandoTodo() []domain.AceptacionConfianza {
	return []domain.AceptacionConfianza{
		{Tipo: domain.ConfianzaRevocacionNoComprobable, AceptadaEn: time.Now()},
		{Tipo: domain.ConfianzaAlgoritmoSHA1, AceptadaEn: time.Now()},
	}
}

func tieneConfianzaForzada(res domain.VerificationResult, tipo domain.TipoConfianzaForzada) bool {
	for _, e := range res.Evidence {
		if e.Type == domain.EvidenciaConfianzaForzada && e.Summary == string(tipo) {
			return true
		}
	}
	return false
}

func tieneObstaculo(lista []domain.ObstaculoConfianza, tipo domain.TipoConfianzaForzada, cert *x509.Certificate) bool {
	for _, o := range lista {
		if o.Tipo != tipo {
			continue
		}
		if cert == nil || o.Huella == certificateFingerprint(cert) {
			return true
		}
	}
	return false
}

// exigirForzadaValida comprueba lo común a los cinco casos: sin aceptar no
// es válida y se ofrece el obstáculo; aceptado es válida con el aviso, el
// detalle y la evidencia trust.user_forced.
func exigirForzadaValida(t *testing.T, tipo domain.TipoConfianzaForzada, cert *x509.Certificate, verificar verificacionConContexto) domain.VerificationResult {
	t.Helper()
	sin, con, errSin, errCon, obstaculos := verificarAceptando(t, nil, verificar)
	if errSin != nil || errCon != nil {
		t.Fatalf("errores al verificar: sin=%v con=%v", errSin, errCon)
	}
	if sin.Valid {
		t.Fatalf("sin aceptar nada la firma ya era válida: %s %v", sin.Reason, sin.Warnings)
	}
	if !tieneObstaculo(obstaculos, tipo, cert) {
		t.Fatalf("no se ofreció aceptar %s: %+v", tipo, obstaculos)
	}
	if !con.Valid {
		t.Fatalf("aceptado %s la firma sigue sin ser válida: %s avisos=%v detalles=%v", tipo, con.Reason, con.Warnings, con.Details)
	}
	if !tieneConfianzaForzada(con, tipo) {
		t.Fatalf("falta la evidencia %s=%s: %v", domain.EvidenciaConfianzaForzada, tipo, con.Evidence)
	}
	if !containsDetail(con.Warnings, tipo.ClaveAplicada()) {
		t.Fatalf("falta el aviso visible %q: %v", tipo.ClaveAplicada(), con.Warnings)
	}
	// Sin registro (REST, afirma://, host nativo) nada se fuerza.
	if res, err := verificar(context.Background()); err == nil && res.Valid {
		t.Fatalf("sin registro en el contexto la firma salió válida: %v", res.Warnings)
	}
	return con
}

func TestConfianzaForzada_CADesconocidaAceptada(t *testing.T) {
	p := nuevaPKILote3(t, nil)
	contenido := []byte("convenio")
	cms := firmarBESLote3(t, p, contenido, p.intermedia, p.raiz)
	// Las anclas del equipo son de otra PKI: la raíz de la firma es desconocida.
	ajena := nuevaPKILote3(t, nil)
	verificador := NewCAdESVerifierWithChecker(comprobadorLote3())
	con := exigirForzadaValida(t, domain.ConfianzaCADesconocida, p.raiz, func(ctx context.Context) (domain.VerificationResult, error) {
		res, _, err := verificador.VerifyDetachedCMSWithAnchors(ctx, cms, contenido, ajena.anclas)
		return res, err
	})
	if con.Trust.Status != domain.VerificationStatusValid || con.Trust.Reason != domain.ConfianzaCADesconocida.ClaveAplicada() {
		t.Fatalf("confianza=%s %q", con.Trust.Status, con.Trust.Reason)
	}

	// Aceptar otra raíz no sirve para esta.
	otra := nuevaPKILote3(t, nil)
	registro := domain.NuevoRegistroConfianzaForzada(domain.ConfianzaForzada{Aceptaciones: []domain.AceptacionConfianza{
		{Tipo: domain.ConfianzaCADesconocida, Huella: certificateFingerprint(otra.raiz)},
	}})
	res, _, err := verificador.VerifyDetachedCMSWithAnchors(domain.ConRegistroConfianzaForzada(context.Background(), registro), cms, contenido, ajena.anclas)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("la aceptación de otra autoridad dio por válida esta firma")
	}
}

// La raíz aceptada no relaja el resto de comprobaciones de la cadena: un
// certificado caducado sigue sin ser válido.
func TestConfianzaForzada_CADesconocidaNoSaltaCaducidad(t *testing.T) {
	fixture := newXMLTrustFixture(t, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
	document := signXAdESTrustDocument(t, fixture)
	_, con, _, err, _ := verificarAceptando(t, aceptandoTodo(), func(ctx context.Context) (domain.VerificationResult, error) {
		res, _, err := NewXAdESVerifier().Verify(ctx, document, domain.CertificateChain{})
		return res, err
	})
	if err == nil && con.Valid {
		t.Fatalf("un certificado caducado sin sello salió válido al aceptarlo todo: %v", con.Warnings)
	}
}

func TestConfianzaForzada_ClaveRSACortaAceptada(t *testing.T) {
	// La clave corta es justo lo que se prueba.
	clave, err := rsa.GenerateKey(rand.Reader, 1024) // nosemgrep: use-of-weak-rsa-key
	if err != nil {
		t.Fatal(err)
	}
	ahora := time.Now()
	cert := emitirLote3(t, &x509.Certificate{
		SerialNumber: big.NewInt(78), Subject: pkix.Name{CommonName: "RSA corta aceptada"},
		NotBefore: ahora.Add(-time.Hour), NotAfter: ahora.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}, nil, &clave.PublicKey, clave)
	anclas := domain.CertificateChain{DERCertificates: [][]byte{cert.Raw}}
	exigirForzadaValida(t, domain.ConfianzaClaveRSACorta, cert, func(ctx context.Context) (domain.VerificationResult, error) {
		return applySignerVerificationMetadata(domain.RegistroConfianzaForzadaDe(ctx),
			domain.NewVerificationSuccess(string(domain.FormatXAdES), "firma correcta", nil),
			[]*x509.Certificate{cert}, []*x509.Certificate{cert}, anclas,
		), nil
	})
}

func TestConfianzaForzada_CertificadoComponenteAceptado(t *testing.T) {
	for _, eku := range [][]x509.ExtKeyUsage{
		{x509.ExtKeyUsageServerAuth},
		{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	} {
		p := nuevaPKILote3(t, nil, eku...)
		contenido := []byte("resolución de sede")
		cms := firmarBESLote3(t, p, contenido, p.intermedia)
		verificador := NewCAdESVerifierWithChecker(comprobadorLote3())
		exigirForzadaValida(t, domain.ConfianzaCertificadoComponente, p.hoja, func(ctx context.Context) (domain.VerificationResult, error) {
			res, _, err := verificador.VerifyDetachedCMSWithAnchors(ctx, cms, contenido, p.anclas)
			return res, err
		})
	}
}

func TestConfianzaForzada_RevocacionNoComprobableAceptada(t *testing.T) {
	p := nuevaPKILote3(t, nil)
	contenido := []byte("acta")
	cms := firmarBESLote3(t, p, contenido, p.intermedia)
	verificador := NewCAdESVerifierWithChecker(NewRevocationCheckerOffline())
	con := exigirForzadaValida(t, domain.ConfianzaRevocacionNoComprobable, nil, func(ctx context.Context) (domain.VerificationResult, error) {
		res, _, err := verificador.VerifyDetachedCMSWithAnchors(ctx, cms, contenido, p.anclas)
		return res, err
	})
	if con.Certificate.Status != domain.VerificationStatusWarning {
		t.Fatalf("la revocación sigue sin comprobarse y debe verse: certificado=%s", con.Certificate.Status)
	}
}

func cmsSHA1Lote3(t *testing.T, p pkiLote3, contenido []byte) []byte {
	t.Helper()
	datos, err := pkcs7.NewSignedData(contenido)
	if err != nil {
		t.Fatal(err)
	}
	datos.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA1)
	if err := datos.AddSignerChain(p.hoja, p.claveHoja, []*x509.Certificate{p.intermedia}, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	datos.Detach()
	cms, err := datos.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return cms
}

func TestConfianzaForzada_SHA1Aceptado(t *testing.T) {
	p := nuevaPKILote3(t, nil)
	contenido := []byte("documento de 2012")
	cms := cmsSHA1Lote3(t, p, contenido)
	verificador := NewCAdESVerifierWithChecker(comprobadorLote3())
	con := exigirForzadaValida(t, domain.ConfianzaAlgoritmoSHA1, nil, func(ctx context.Context) (domain.VerificationResult, error) {
		res, _, err := verificador.VerifyDetachedCMSWithAnchors(ctx, cms, contenido, p.anclas)
		return res, err
	})
	if !tieneEvidenciaAlgoritmoAntiguo(con) {
		t.Fatalf("la evidencia de algoritmo antiguo debe seguir: %v", con.Evidence)
	}
	if containsDetail(con.Warnings, motivoAlgoritmoAntiguo) {
		t.Fatalf("aceptado SHA-1, el aviso es el de confianza forzada: %v", con.Warnings)
	}
}

// ---------------------------------------------------------------------------
// Lo que nunca se puede forzar, aunque el usuario lo acepte todo
// ---------------------------------------------------------------------------

func exigirNoValidaAunqueSeAcepteTodo(t *testing.T, que string, verificar verificacionConContexto) {
	t.Helper()
	_, con, _, err, _ := verificarAceptando(t, aceptandoTodo(), verificar)
	if err == nil && con.Valid {
		t.Fatalf("%s salió válida aceptándolo todo: %s avisos=%v", que, con.Reason, con.Warnings)
	}
}

func TestConfianzaForzada_NuncaForzable(t *testing.T) {
	p := nuevaPKILote3(t, nil)
	contenido := []byte("contrato")
	cms := firmarBESLote3(t, p, contenido, p.intermedia, p.raiz)

	t.Run("integridad rota", func(t *testing.T) {
		verificador := NewCAdESVerifierWithChecker(comprobadorLote3())
		exigirNoValidaAunqueSeAcepteTodo(t, "contenido alterado", func(ctx context.Context) (domain.VerificationResult, error) {
			res, _, err := verificador.VerifyDetachedCMS(ctx, cms, []byte("contrato alterado"))
			return res, err
		})
	})

	t.Run("firma incorrecta", func(t *testing.T) {
		otra := nuevaPKILote3(t, nil)
		datos, err := pkcs7.NewSignedData(contenido)
		if err != nil {
			t.Fatal(err)
		}
		// Certificado de p con la clave de otra: la firma no cuadra.
		if err := datos.AddSignerChain(p.hoja, otra.claveHoja, []*x509.Certificate{p.intermedia, p.raiz}, pkcs7.SignerInfoConfig{}); err != nil {
			t.Fatal(err)
		}
		datos.Detach()
		malo, err := datos.Finish()
		if err != nil {
			t.Fatal(err)
		}
		verificador := NewCAdESVerifierWithChecker(comprobadorLote3())
		exigirNoValidaAunqueSeAcepteTodo(t, "firma con otra clave", func(ctx context.Context) (domain.VerificationResult, error) {
			res, _, err := verificador.VerifyDetachedCMS(ctx, malo, contenido)
			return res, err
		})
	})

	t.Run("revocado", func(t *testing.T) {
		verificador := NewCAdESVerifierWithChecker(comprobadorLote3(p.hoja))
		exigirNoValidaAunqueSeAcepteTodo(t, "certificado revocado", func(ctx context.Context) (domain.VerificationResult, error) {
			res, _, err := verificador.VerifyDetachedCMS(ctx, cms, contenido)
			return res, err
		})
		intermedia := NewCAdESVerifierWithChecker(comprobadorLote3(p.intermedia))
		exigirNoValidaAunqueSeAcepteTodo(t, "intermedia revocada", func(ctx context.Context) (domain.VerificationResult, error) {
			res, _, err := intermedia.VerifyDetachedCMS(ctx, cms, contenido)
			return res, err
		})
	})

	t.Run("MD5 o algoritmo desconocido", func(t *testing.T) {
		sha1 := cmsSHA1Lote3(t, p, contenido)
		// md5WithRSA (1.3.14.3.2.3) en lugar de SHA-1 (1.3.14.3.2.26): mismo largo.
		md5 := bytes.ReplaceAll(sha1, []byte{0x06, 0x05, 0x2b, 0x0e, 0x03, 0x02, 0x1a}, []byte{0x06, 0x05, 0x2b, 0x0e, 0x03, 0x02, 0x03})
		if bytes.Equal(md5, sha1) {
			t.Fatal("no se encontró el OID de SHA-1 que sustituir")
		}
		verificador := NewCAdESVerifierWithChecker(comprobadorLote3())
		exigirNoValidaAunqueSeAcepteTodo(t, "firma con MD5", func(ctx context.Context) (domain.VerificationResult, error) {
			res, _, err := verificador.VerifyDetachedCMSWithAnchors(ctx, md5, contenido, p.anclas)
			return res, err
		})
	})

	t.Run("caducado sin sello", func(t *testing.T) {
		fixture := newXMLTrustFixture(t, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
		document := signXAdESTrustDocument(t, fixture)
		exigirNoValidaAunqueSeAcepteTodo(t, "certificado caducado", func(ctx context.Context) (domain.VerificationResult, error) {
			res, _, err := NewXAdESVerifier().Verify(ctx, document, domain.CertificateChain{})
			return res, err
		})
	})

	t.Run("firma de código", func(t *testing.T) {
		for _, eku := range []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning, x509.ExtKeyUsageTimeStamping, x509.ExtKeyUsageOCSPSigning} {
			pc := nuevaPKILote3(t, nil, eku)
			firmado := firmarBESLote3(t, pc, contenido, pc.intermedia)
			verificador := NewCAdESVerifierWithChecker(comprobadorLote3())
			exigirNoValidaAunqueSeAcepteTodo(t, "certificado con EKU que no es de firma", func(ctx context.Context) (domain.VerificationResult, error) {
				res, _, err := verificador.VerifyDetachedCMSWithAnchors(ctx, firmado, contenido, pc.anclas)
				return res, err
			})
		}
	})

	t.Run("cobertura parcial", func(t *testing.T) {
		firmado := string(operarXAdES(t, []byte("%PDF-1.7 datos"), "application/pdf", domain.ActionSign, map[string]string{"format": "XAdES Enveloping"}, "Victima"))
		firma := elementoLiteral(t, firmado, "<ds:Signature", "</ds:Signature>")
		doc := domain.Document{Name: "pedido.xml", MIMEType: "application/xml", Content: []byte(`<pedido><importe>9999</importe>` + firma + `</pedido>`)}
		exigirNoValidaAunqueSeAcepteTodo(t, "firma que no cubre el documento", func(ctx context.Context) (domain.VerificationResult, error) {
			res, _, err := NewXAdESVerifier().Verify(ctx, doc, domain.CertificateChain{})
			return res, err
		})
	})

	t.Run("ataque de envoltura", func(t *testing.T) {
		firmado := string(operarXAdES(t, []byte("%PDF-1.7 contrato de la víctima"), "application/pdf", domain.ActionSign, map[string]string{"format": "XAdES Detached"}, "Victima"))
		contenidoXML := elementoLiteral(t, firmado, "<CONTENT", "</CONTENT>")
		firma := elementoLiteral(t, firmado, "<ds:Signature", "</ds:Signature>")
		factura, err := os.ReadFile("testdata/sample-facturae.xml")
		if err != nil {
			t.Fatal(err)
		}
		falsa := strings.Replace(string(factura), "127.01", "999999.99", -1)
		raiz := elementoLiteral(t, falsa, "<Facturae", ">")
		falsa = strings.Replace(falsa, raiz, `<Facturae xmlns="http://www.facturae.es/Facturae/2007/v3.1/Facturae">`, 1)
		cierre := strings.LastIndex(falsa, "</Facturae>")
		doc := domain.Document{Name: "factura.xsig", MIMEType: "application/xml", Content: []byte(falsa[:cierre] + `<Extensiones xmlns="">` + contenidoXML + firma + `</Extensiones>` + falsa[cierre:])}
		exigirNoValidaAunqueSeAcepteTodo(t, "FacturaE con firma ajena envuelta", func(ctx context.Context) (domain.VerificationResult, error) {
			res, _, err := NewFacturaEVerifier().Verify(ctx, doc, domain.CertificateChain{})
			return res, err
		})
		exigirNoValidaAunqueSeAcepteTodo(t, "XAdES con firma ajena envuelta", func(ctx context.Context) (domain.VerificationResult, error) {
			res, _, err := NewXAdESVerifier().Verify(ctx, doc, domain.CertificateChain{})
			return res, err
		})
	})
}

// Sin almacén raíz del sistema (E1-11) la raíz aceptada también vale.
func TestConfianzaForzada_CADesconocidaSinAlmacenRaiz(t *testing.T) {
	original := cargarRaicesSistema
	cargarRaicesSistema = func() (*x509.CertPool, error) { return nil, errors.New("sin almacén raíz") }
	t.Cleanup(func() { cargarRaicesSistema = original })

	p := nuevaPKILote3(t, nil)
	exigirForzadaValida(t, domain.ConfianzaCADesconocida, p.raiz, func(ctx context.Context) (domain.VerificationResult, error) {
		return applySignerVerificationMetadata(domain.RegistroConfianzaForzadaDe(ctx),
			domain.NewVerificationSuccess(string(domain.FormatCAdES), "firma correcta", nil),
			[]*x509.Certificate{p.hoja}, []*x509.Certificate{p.hoja, p.intermedia, p.raiz},
			domain.CertificateChain{UseSystemRoots: true},
		), nil
	})
}
