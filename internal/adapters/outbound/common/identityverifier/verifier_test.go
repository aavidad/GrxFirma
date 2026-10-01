// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package identityverifier

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var oidPoliticaPrueba = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}

func TestVerificadorAceptaPruebaCompletaReal(t *testing.T) {
	fixture := nuevaFixtureVerificador(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, []asn1.ObjectIdentifier{oidPoliticaPrueba})
	verificador := fixture.nuevo(t, ports.EstadoRevocacionConforme)
	resultado, err := verificador.Verificar(context.Background(), fixture.reto, fixture.prueba)
	if err != nil {
		t.Fatalf("verificar: %v", err)
	}
	if resultado.Resultado != domain.ResultadoIdentidadAceptada || resultado.ValidarEstructura() != nil {
		t.Fatalf("aceptación inválida: %+v", resultado)
	}
	if resultado.IdentidadAcreditada != "identidad:administrativa:opaca" || resultado.EvidenciaRef != "evidencia:durable:1" {
		t.Fatalf("autoridad o evidencia inesperada: %+v", resultado)
	}
}

func TestVerificadorRechazaManipulacionEKUYPolitica(t *testing.T) {
	fixture := nuevaFixtureVerificador(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, []asn1.ObjectIdentifier{oidPoliticaPrueba})
	casos := []struct {
		nombre string
		mutar  func(*fixtureVerificador)
		config Configuracion
	}{
		{"canon manipulado", func(f *fixtureVerificador) { f.reto.ContenidoCanonico = []byte("otro canon") }, fixture.configuracion()},
		{"EKU no admitido", func(*fixtureVerificador) {}, configuracionPrueba([]x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}, []asn1.ObjectIdentifier{oidPoliticaPrueba})},
		{"política no admitida", func(*fixtureVerificador) {}, configuracionPrueba([]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, []asn1.ObjectIdentifier{{1, 2, 3, 4}})},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			local := fixture
			local.reto.ContenidoCanonico = append([]byte(nil), fixture.reto.ContenidoCanonico...)
			caso.mutar(&local)
			verificador := local.nuevoConConfiguracion(t, caso.config, ports.EstadoRevocacionConforme)
			resultado, err := verificador.Verificar(context.Background(), local.reto, local.prueba)
			if err != nil || resultado.Resultado != domain.ResultadoIdentidadRechazada {
				t.Fatalf("no se rechazó: resultado=%+v error=%v", resultado, err)
			}
		})
	}
}

func TestVerificadorDistingueRevocacionEIndeterminacion(t *testing.T) {
	fixture := nuevaFixtureVerificador(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, []asn1.ObjectIdentifier{oidPoliticaPrueba})
	for _, caso := range []struct {
		nombre   string
		estado   ports.EstadoRevocacionIdentidad
		esperado domain.ResultadoIdentidad
		conError bool
	}{
		{"revocado", ports.EstadoRevocacionRevocada, domain.ResultadoIdentidadRechazada, false},
		{"sin evidencia", ports.EstadoRevocacionIndeterminada, domain.ResultadoIdentidadIndeterminada, true},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			resultado, err := fixture.nuevo(t, caso.estado).Verificar(context.Background(), fixture.reto, fixture.prueba)
			if resultado.Resultado != caso.esperado || (err != nil) != caso.conError {
				t.Fatalf("resultado=%+v error=%v", resultado, err)
			}
		})
	}
}

type fixtureVerificador struct {
	ahora      time.Time
	raiz, hoja *x509.Certificate
	reto       domain.RetoIdentidad
	prueba     domain.PruebaIdentidad
}

func nuevaFixtureVerificador(t *testing.T, eku []x509.ExtKeyUsage, politicas []asn1.ObjectIdentifier) fixtureVerificador {
	t.Helper()
	ahora := time.Now().UTC().Truncate(time.Second)
	raizClave, _ := rsa.GenerateKey(rand.Reader, 2048)
	raizPlantilla := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Raíz de pruebas"},
		NotBefore: ahora.Add(-time.Hour), NotAfter: ahora.Add(24 * time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	raizDER, err := x509.CreateCertificate(rand.Reader, raizPlantilla, raizPlantilla, &raizClave.PublicKey, raizClave)
	if err != nil {
		t.Fatalf("crear raíz: %v", err)
	}
	raiz, _ := x509.ParseCertificate(raizDER)
	hojaClave, _ := rsa.GenerateKey(rand.Reader, 2048)
	politicasX509 := make([]x509.OID, 0, len(politicas))
	for _, politica := range politicas {
		componentes := make([]uint64, len(politica))
		for i, componente := range politica {
			componentes[i] = uint64(componente)
		}
		oid, err := x509.OIDFromInts(componentes)
		if err != nil {
			t.Fatalf("crear OID de política: %v", err)
		}
		politicasX509 = append(politicasX509, oid)
	}
	hojaPlantilla := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Identidad sintética"},
		NotBefore: ahora.Add(-time.Hour), NotAfter: ahora.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: eku, Policies: politicasX509}
	hojaDER, err := x509.CreateCertificate(rand.Reader, hojaPlantilla, raiz, &hojaClave.PublicKey, raizClave)
	if err != nil {
		t.Fatalf("crear hoja: %v", err)
	}
	hoja, _ := x509.ParseCertificate(hojaDER)
	canon := []byte(`{"contract":"identidad-reforzada/v1","challengeId":"reto:1"}`)
	documento, _ := domain.NewDocument("reto.json", canon, "application/json")
	firma, err := signer.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{Document: documento, Format: domain.FormatCAdES, Action: domain.ActionSign}, &signer.LocalSigningKey{ID: "clave:1", Signer: hojaClave, Certificate: hoja, Chain: []*x509.Certificate{raiz}})
	if err != nil {
		t.Fatalf("firmar reto: %v", err)
	}
	solicitud := domain.SolicitudRetoIdentidad{Contrato: domain.VersionContratoIdentidadReforzada,
		RetoID: "reto:1", Audiencia: "urn:dipgra:identidad", ClienteRegistrado: "cliente",
		Finalidad: "Acreditar identidad", Operacion: "identidad.reforzar.v1",
		HuellaContextoTenant: "hmac:tenant", VinculoSesion: "hmac:sesion",
		Origen: "https://integrador.example", ConsentimientoID: "consentimiento",
		VersionConsentimiento: "1", PoliticaID: "politica", VersionPolitica: "1",
		Nonce: make([]byte, 32), EmitidoEn: ahora, ExpiraEn: ahora.Add(time.Minute)}
	return fixtureVerificador{ahora: ahora, raiz: raiz, hoja: hoja,
		reto: domain.RetoIdentidad{Solicitud: solicitud, ContenidoCanonico: canon},
		prueba: domain.PruebaIdentidad{RetoID: "reto:1", Formato: "cades-detached",
			AlgoritmoFirma: "sha256-rsa-pkcs1v15", AlgoritmoHuella: "sha-256",
			Firma: firma.Data, Certificado: hoja.Raw, Cadena: [][]byte{raiz.Raw}}}
}

func (f fixtureVerificador) configuracion() Configuracion {
	return configuracionPrueba([]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, []asn1.ObjectIdentifier{oidPoliticaPrueba})
}

func configuracionPrueba(eku []x509.ExtKeyUsage, politicas []asn1.ObjectIdentifier) Configuracion {
	return Configuracion{EKUPermitidos: eku, OIDPoliticas: politicas,
		NivelAseguramiento: "certificado", MetodoAutenticacion: "x509-cades-v1", FuenteDictamen: "grxfirma-servidor/v1"}
}

func (f fixtureVerificador) nuevo(t *testing.T, estado ports.EstadoRevocacionIdentidad) *Verificador {
	return f.nuevoConConfiguracion(t, f.configuracion(), estado)
}

func (f fixtureVerificador) nuevoConConfiguracion(t *testing.T, configuracion Configuracion, estado ports.EstadoRevocacionIdentidad) *Verificador {
	t.Helper()
	verificador, err := Nuevo(configuracion, anclasIdentidadFalsas{raiz: f.raiz.Raw},
		revocacionIdentidadFalsa{estado: estado, ahora: f.ahora}, acreditadorIdentidadFalso{},
		evidenciaIdentidadFalsa{}, relojIdentidadFalso{ahora: f.ahora})
	if err != nil {
		t.Fatalf("construir verificador: %v", err)
	}
	return verificador
}

type anclasIdentidadFalsas struct{ raiz []byte }

func (a anclasIdentidadFalsas) Anchors(context.Context) (domain.CertificateChain, error) {
	return domain.CertificateChain{DERCertificates: [][]byte{a.raiz}}, nil
}

type revocacionIdentidadFalsa struct {
	estado ports.EstadoRevocacionIdentidad
	ahora  time.Time
}

func (r revocacionIdentidadFalsa) ComprobarIdentidad(context.Context, *x509.Certificate, *x509.Certificate) (ports.ResultadoRevocacionIdentidad, error) {
	return ports.ResultadoRevocacionIdentidad{Estado: r.estado, Fuente: "ocsp-sintetico", ComprobadoEn: r.ahora}, nil
}

type acreditadorIdentidadFalso struct{}

func (acreditadorIdentidadFalso) AcreditarCertificado(context.Context, *x509.Certificate) (string, error) {
	return "identidad:administrativa:opaca", nil
}

type evidenciaIdentidadFalsa struct{}

func (evidenciaIdentidadFalsa) RegistrarIdentidad(context.Context, ports.EvidenciaIdentidad) (string, error) {
	return "evidencia:durable:1", nil
}

type relojIdentidadFalso struct{ ahora time.Time }

func (r relojIdentidadFalso) Now() time.Time { return r.ahora }
