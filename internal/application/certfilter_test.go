// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func certFiltro(t *testing.T, cn, emisorO, emisorCN string, ku x509.KeyUsage, caduca time.Time, serie int64) domain.CertificateRef {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	issuer := pkix.Name{CommonName: emisorCN, Organization: []string{emisorO}, Country: []string{"ES"}}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(serie), Subject: pkix.Name{CommonName: cn, Country: []string{"ES"}},
		Issuer: issuer, NotBefore: time.Now().Add(-time.Hour), NotAfter: caduca, KeyUsage: ku,
	}
	parent := &x509.Certificate{Subject: issuer, SerialNumber: big.NewInt(1)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, parent, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	return domain.CertificateRef{ID: cn, Subject: c.Subject.String(), Issuer: c.Issuer.String(), NotAfter: caduca, DER: der}
}

func nombres(refs []domain.CertificateRef) []string {
	var out []string
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

func TestFiltrarCertificados_CompatibilidadJava(t *testing.T) {
	futuro := time.Now().Add(24 * time.Hour)
	firma := x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment
	fnmt := certFiltro(t, "PRUEBAS EIDAS - 99999999R", "FNMT-RCM", "AC FNMT Usuarios", firma, futuro, 10)
	canarias := certFiltro(t, "EMPLEADO", "Gobierno de Canarias", "GobCanCA", firma, futuro, 11)
	caducado := certFiltro(t, "CADUCADO", "FNMT-RCM", "AC FNMT Usuarios", firma, time.Now().Add(-time.Hour), 12)
	dnieFirma := certFiltro(t, "ESPAÑOL (FIRMA)", "DIRECCION GENERAL DE LA POLICIA", "AC DNIE 004", x509.KeyUsageContentCommitment, futuro, 13)
	dnieAuth := certFiltro(t, "ESPAÑOL (AUTENTICACIÓN)", "DIRECCION GENERAL DE LA POLICIA", "AC DNIE 004", x509.KeyUsageDigitalSignature, futuro, 14)
	todos := []domain.CertificateRef{fnmt, canarias, caducado, dnieFirma, dnieAuth}
	h := sha256.Sum256(fnmt.DER)

	casos := []struct {
		nombre  string
		options map[string]string
		want    []string
	}{
		{"sin filtros oculta caducados", nil, []string{fnmt.ID, canarias.ID, dnieFirma.ID, dnieAuth.ID}},
		{"filtro real de Canarias", map[string]string{"filters": "nonexpired:true;issuer.rfc2254:(&(!(CN=CiberCentro*))(!(CN=GobCanCA))(!(O=Gobierno de Canarias))(!(O=PKI))(!(O=DO_NOT_TRUST*)));signingCert:true"},
			[]string{fnmt.ID, dnieFirma.ID}},
		{"dnie", map[string]string{"filters": "dnie:"}, []string{dnieFirma.ID}},
		{"authCert", map[string]string{"filters": "authcert:"}, []string{dnieAuth.ID}},
		{"keyusage", map[string]string{"filters": "keyusage.digitalsignature:true;keyusage.nonrepudiation:true"}, []string{fnmt.ID, canarias.ID, caducado.ID}},
		{"thumbprint", map[string]string{"filters": "thumbprint:SHA-256:" + hex.EncodeToString(h[:])}, []string{fnmt.ID}},
		{"grupos en disyunción", map[string]string{"filters.1": "subject.contains:EMPLEADO", "filters.2": "issuer.rfc2254:(O=DIRECCION*)"},
			[]string{canarias.ID, dnieFirma.ID, dnieAuth.ID}},
		{"ssl por serie", map[string]string{"filters": "ssl:0b"}, []string{canarias.ID}},
		{"filtro LDAP mal formado no deja pasar nada", map[string]string{"filters": "issuer.rfc2254:(&(CN=x)"}, nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := nombres(FiltrarCertificados(todos, c.options, time.Now()).Certificados)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}
