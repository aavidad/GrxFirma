// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmahandler

import (
	"context"
	"testing"

	"grxfirma/internal/ports"
)

// autoFirmaEnUsuario deja en HKCU el registro que escribe AutoFirma cuando
// queda para el usuario: instalación por usuario, o AutoFirma instalado o
// actualizado con la misma cuenta después de GrxFirma (Windows guarda en
// HKCU\Software\Classes los valores de HKCR cuya clave ya existe allí).
func autoFirmaEnUsuario(fx *fixture) {
	fx.reg.set(CurrentUser, ProtocolKey, "", KindString, "URL:Afirma Protocol")
	fx.reg.set(CurrentUser, ProtocolKey, "URL Protocol", KindString, "")
	fx.reg.set(CurrentUser, ProtocolKey+`\DefaultIcon`, "", KindString, `C:\Program Files\Autofirma\Autofirma\ic_firmar.ico`)
	fx.reg.set(CurrentUser, ProtocolKey+`\shell\open\command`, "", KindString, testAutoFirmaExe+` %1`)
}

func autoFirmaPresente(fx *fixture) {
	fx.sel.fileExists = func(p string) bool { return p == fx.sel.paths.Executable || p == testAutoFirmaExe }
}

// Queja de usuarios (14/11/2026): con AutoFirma instalado, GrxFirma no deja
// elegirlo y se queda fijo. Si el registro de AutoFirma estaba en HKCU, el
// instalador de GrxFirma lo sustituye y lo guarda en la instantánea; HKLM
// está vacío y el selector decía «AutoFirma no está instalado».
func TestQuejaAutoFirmaRegistradoParaElUsuarioAntesDeGrxFirma(t *testing.T) {
	fx := newFixture(t, false)
	autoFirmaPresente(fx)
	autoFirmaEnUsuario(fx)
	fx.installLikeNSIS(t)

	estado, err := fx.sel.Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaGrxFirma {
		t.Fatalf("tras instalar GrxFirma debe atender las firmas: %+v", estado)
	}
	if !estado.AutoFirmaInstalada || estado.RutaAutoFirma != testAutoFirmaExe {
		t.Fatalf("AutoFirma está instalado (su registro está en la instantánea) y no se ofrece: %+v", estado)
	}
	estado, err = fx.sel.Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma)
	if err != nil {
		t.Fatalf("elegir AutoFirma debe devolverle su registro: %v", err)
	}
	cmd, _ := fx.reg.Read(CurrentUser, ProtocolKey+`\shell\open\command`, "")
	if cmd.Data != testAutoFirmaExe+` %1` {
		t.Fatalf("no se restauró la orden de AutoFirma: %+v", cmd)
	}
	if estado.Actual != ports.ProgramaAfirmaAutoFirma {
		t.Fatalf("estado tras elegir AutoFirma: %+v", estado)
	}
	if _, err := fx.sel.Elegir(context.Background(), ports.ProgramaAfirmaGrxFirma); err != nil {
		t.Fatalf("volver a GrxFirma: %v", err)
	}
}

// AutoFirma instalado después de GrxFirma con la misma cuenta: su registro
// sustituye al de GrxFirma en HKCU y HKLM queda vacío. El selector decía a la
// vez «atiende AutoFirma» y «AutoFirma no está instalado».
func TestQuejaAutoFirmaInstaladoDespuesConLaMismaCuenta(t *testing.T) {
	fx := newFixture(t, false)
	autoFirmaPresente(fx)
	fx.installLikeNSIS(t)
	autoFirmaEnUsuario(fx)

	estado, err := fx.sel.Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaAutoFirma || !estado.AutoFirmaInstalada {
		t.Fatalf("AutoFirma atiende las firmas y debe constar como instalado: %+v", estado)
	}
	fx.reg.writes = 0
	if _, err := fx.sel.Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma); err != nil {
		t.Fatalf("elegir AutoFirma cuando ya atiende las firmas no es un error: %v", err)
	}
	if fx.reg.writes != 1 {
		t.Fatalf("solo debe guardarse la preferencia; escrituras: %d", fx.reg.writes)
	}
}
