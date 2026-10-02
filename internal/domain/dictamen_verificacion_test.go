// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import "testing"

func firmanteCompleto() DictamenFirmante {
	return DictamenFirmante{
		CertificadoHuellaSHA256: "ab",
		Cadena:                  AspectoDictamen{Estado: CadenaValida},
		Certificado:             AspectoDictamen{Estado: CertificadoVigente},
		Revocacion:              AspectoDictamen{Estado: RevocacionVigente},
		SelloTiempo:             AspectoDictamen{Estado: SelloNoPresente},
	}
}

func dictamenBase(firmantes ...DictamenFirmante) DictamenVerificacion {
	return DictamenVerificacion{
		Integridad:      AspectoDictamen{Estado: IntegridadValida},
		VinculoOriginal: AspectoDictamen{Estado: VinculoAcreditado},
		Firmantes:       firmantes,
	}
}

func TestDictamenComponer_Precedencia(t *testing.T) {
	con := func(cambio func(*DictamenFirmante)) DictamenFirmante {
		f := firmanteCompleto()
		cambio(&f)
		return f
	}
	casos := []struct {
		nombre string
		d      DictamenVerificacion
		estado EstadoDictamen
		motivo MotivoDictamen
	}{
		{"valida", dictamenBase(firmanteCompleto()), EstadoDictamenValida, MotivoDictamenVerificada},
		{"sello_valido", dictamenBase(con(func(f *DictamenFirmante) { f.SelloTiempo.Estado = SelloValido })), EstadoDictamenValida, MotivoDictamenVerificada},
		{"sello_no_comprobado_no_bloquea", dictamenBase(con(func(f *DictamenFirmante) { f.SelloTiempo.Estado = SelloNoComprobado })), EstadoDictamenValida, MotivoDictamenVerificada},
		{"sello_no_valido", dictamenBase(con(func(f *DictamenFirmante) { f.SelloTiempo.Estado = SelloNoValido })), EstadoDictamenIndeterminada, MotivoDictamenSelloTiempoNoAcreditado},
		{"revocacion_no_comprobada", dictamenBase(con(func(f *DictamenFirmante) { f.Revocacion.Estado = RevocacionNoComprobada })), EstadoDictamenIndeterminada, MotivoDictamenRevocacionNoAcreditada},
		{"revocado_prevalece", dictamenBase(con(func(f *DictamenFirmante) {
			f.Revocacion.Estado = RevocacionRevocado
			f.Cadena.Estado = CadenaNoComprobada
		})), EstadoDictamenNoValida, MotivoDictamenCertificadoNoValido},
		{"caducado", dictamenBase(con(func(f *DictamenFirmante) { f.Certificado.Estado = CertificadoNoVigente })), EstadoDictamenNoValida, MotivoDictamenCertificadoNoValido},
		{"cadena_no_valida", dictamenBase(con(func(f *DictamenFirmante) { f.Cadena.Estado = CadenaNoValida })), EstadoDictamenNoValida, MotivoDictamenConfianzaNoValida},
		{"cadena_no_comprobada", dictamenBase(con(func(f *DictamenFirmante) { f.Cadena.Estado = CadenaNoComprobada })), EstadoDictamenIndeterminada, MotivoDictamenConfianzaNoAcreditada},
		{"sin_firmantes", dictamenBase(), EstadoDictamenIndeterminada, MotivoDictamenFirmanteNoIdentificado},
		{"estado_desconocido_no_favorece", dictamenBase(con(func(f *DictamenFirmante) { f.Revocacion.Estado = "good" })), EstadoDictamenIndeterminada, MotivoDictamenRevocacionNoAcreditada},
		{"un_firmante_de_dos_sin_revocacion", dictamenBase(firmanteCompleto(), con(func(f *DictamenFirmante) { f.Revocacion.Estado = RevocacionNoComprobada })), EstadoDictamenIndeterminada, MotivoDictamenRevocacionNoAcreditada},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			got := caso.d.Componer()
			if got.Estado != caso.estado || got.Motivo != caso.motivo {
				t.Fatalf("got %s/%s, want %s/%s", got.Estado, got.Motivo, caso.estado, caso.motivo)
			}
			if got.Contrato != ContratoDictamenVerificacion {
				t.Fatalf("contrato=%q", got.Contrato)
			}
		})
	}

	integridad := dictamenBase(firmanteCompleto())
	integridad.Integridad.Estado = IntegridadParcial
	if got := integridad.Componer(); got.Estado != EstadoDictamenIndeterminada || got.Motivo != MotivoDictamenIntegridadParcial {
		t.Fatalf("integridad parcial: %s/%s", got.Estado, got.Motivo)
	}
	vinculo := dictamenBase(firmanteCompleto())
	vinculo.VinculoOriginal.Estado = VinculoNoAcreditado
	if got := vinculo.Componer(); got.Estado != EstadoDictamenIndeterminada || got.Motivo != MotivoDictamenVinculoOriginalNoAcreditado {
		t.Fatalf("vínculo: %s/%s", got.Estado, got.Motivo)
	}
	sinOriginal := dictamenBase(firmanteCompleto())
	sinOriginal.VinculoOriginal.Estado = VinculoNoAportado
	if got := sinOriginal.Componer(); got.Estado != EstadoDictamenValida || got.CertificadoHuellaSHA256 != "ab" {
		t.Fatalf("sin original: %s/%s", got.Estado, got.Motivo)
	}
	dos := dictamenBase(firmanteCompleto(), firmanteCompleto()).Componer()
	if dos.CertificadoHuellaSHA256 != "" {
		t.Fatal("con varios firmantes no hay una única huella de certificado")
	}
}
