// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmahandler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/ports"
)

func TestValidarURIReenvio(t *testing.T) {
	validas := []string{
		"afirma://sign?op=sign&id=AbC123&stservlet=https%3A%2F%2Fsede.ejemplo.es%2FStorage",
		"AFIRMA://websocket?ports=63117&idsession=xyz",
		"afirmav2://sign?dat=e30",
		"afirma:sign",
	}
	for _, uri := range validas {
		if err := ValidarURIReenvio(uri); err != nil {
			t.Errorf("%q debería poder reenviarse: %v", uri, err)
		}
	}
	malas := []string{
		"",
		"https://sede.ejemplo.es/",
		"file:///etc/passwd",
		"xafirma://sign",
		`afirma://sign?x=" -Dinyeccion=1 "`,
		"afirma://sign?x=1 -jar otro.jar",
		"afirma://sign?x=1\n-otra",
		"afirma://sign?x=1\r",
		"afirma://sign?x=1\x00",
		"afirma://sign?x=1\t2",
		"afirma://sign?x='1'",
		"afirma://sign?x=\xff",
		"afirma://" + strings.Repeat("a", maxURIReenvio),
	}
	for _, uri := range malas {
		if err := ValidarURIReenvio(uri); !errors.Is(err, ErrURIReenvioNoValida) {
			t.Errorf("%q no debería reenviarse: %v", uri, err)
		}
	}
}

func TestParsearOrdenAutoFirmaWindows(t *testing.T) {
	const exe = `C:\Program Files\Autofirma\Autofirma\Autofirma.exe`
	buenas := []string{exe + ` %1`, exe + ` "%1"`, `"` + exe + `" "%1"`, `"` + exe + `" %1`, "  " + exe + "  \"%1\"  "}
	for _, cmd := range buenas {
		if got, ok := parsearOrdenAutoFirmaWindows(cmd); !ok || got != exe {
			t.Errorf("%q: %q %v", cmd, got, ok)
		}
	}
	malas := []string{
		exe,
		exe + ` -Dx=1 %1`,
		exe + ` %1 %2`,
		exe + ` "%1" -extra`,
		`cmd.exe /c ` + exe + ` %1`,
		`"` + exe + `"%1`,
		`"` + exe,
		`Autofirma.exe %1`,
		`C:\Program Files\Autofirma\Autofirma\Autofirma.bat %1`,
		`C:\%TEMP%\Autofirma.exe %1`,
		`\\servidor\share\Autofirma.exe %1`,
	}
	for _, cmd := range malas {
		if got, ok := parsearOrdenAutoFirmaWindows(cmd); ok {
			t.Errorf("%q no debería aceptarse: %q", cmd, got)
		}
	}
}

func TestEjecutableAutoFirmaWindows(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	exe, err := fx.sel.EjecutableAutoFirma(context.Background())
	if err != nil || exe != testAutoFirmaExe {
		t.Fatalf("AutoFirma del equipo: %q %v", exe, err)
	}

	// Una orden con argumentos de más no es reconocible.
	fx = newFixture(t, true)
	fx.reg.set(LocalMachine, ProtocolKey+`\shell\open\command`, "", KindString, testAutoFirmaExe+` -Djava.ext=1 "%1"`)
	fx.installLikeNSIS(t)
	if _, err := fx.sel.EjecutableAutoFirma(context.Background()); !errors.Is(err, ports.ErrAutoFirmaNoInstalada) {
		t.Fatalf("orden con argumentos de más: %v", err)
	}

	// La orden de GrxFirma nunca es AutoFirma.
	fx = newFixture(t, false)
	fx.installLikeNSIS(t)
	if _, err := fx.sel.EjecutableAutoFirma(context.Background()); !errors.Is(err, ports.ErrAutoFirmaNoInstalada) {
		t.Fatalf("solo GrxFirma registrado: %v", err)
	}
}

func TestEjecutableAutoFirmaConPoliticaDejaPasarLaConsulta(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	exe, err := EjecutableAutoFirma(context.Background(), ConPolitica(fx.sel, politicaFija(ports.ProgramaAfirmaGrxFirma)))
	if err != nil || exe != testAutoFirmaExe {
		t.Fatalf("%q %v", exe, err)
	}
	if _, err := EjecutableAutoFirma(context.Background(), unsupported{}); !errors.Is(err, ports.ErrAutoFirmaNoInstalada) {
		t.Fatalf("sin selector: %v", err)
	}
}

func ficheroEjecutable(t *testing.T, nombre string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), nombre)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReenviarRechazaYNoLanza(t *testing.T) {
	var lanzados [][2]string
	anterior := lanzar
	lanzar = func(_ context.Context, exe, uri string) error {
		lanzados = append(lanzados, [2]string{exe, uri})
		return nil
	}
	t.Cleanup(func() { lanzar = anterior })

	auto := ficheroEjecutable(t, "autofirma")
	const uri = "afirma://sign?id=1"
	casos := map[string]struct {
		exe, uri string
		want     error
	}{
		"enlace con comillas":    {auto, `afirma://sign?x="1"`, ErrURIReenvioNoValida},
		"otro esquema":           {auto, "https://ejemplo.es", ErrURIReenvioNoValida},
		"ruta relativa":          {"autofirma", uri, ErrOrdenAutoFirmaNoValida},
		"el propio GrxFirma":     {ficheroEjecutable(t, "grxfirma-afirmauri"), uri, ErrOrdenAutoFirmaNoValida},
		"otro programa":          {ficheroEjecutable(t, "firefox"), uri, ErrOrdenAutoFirmaNoValida},
		"AutoFirma desaparecido": {filepath.Join(t.TempDir(), "autofirma"), uri, ports.ErrAutoFirmaNoInstalada},
	}
	for nombre, c := range casos {
		if err := Reenviar(context.Background(), c.exe, c.uri); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, se esperaba %v", nombre, err, c.want)
		}
	}
	t.Setenv(EnvReenvioAfirma, "1")
	if err := Reenviar(context.Background(), auto, uri); !errors.Is(err, ErrReenvioEnBucle) {
		t.Errorf("un proceso reenviado no reenvía otra vez: %v", err)
	}
	if len(lanzados) != 0 {
		t.Fatalf("no debía lanzarse nada: %v", lanzados)
	}
	t.Setenv(EnvReenvioAfirma, "")
	if err := Reenviar(context.Background(), auto, uri); err != nil {
		t.Fatal(err)
	}
	if len(lanzados) != 1 || lanzados[0] != [2]string{auto, uri} {
		t.Fatalf("lanzados: %v", lanzados)
	}
}

func TestEntornoSinMarca(t *testing.T) {
	got := entornoSinMarca([]string{"A=1", EnvReenvioAfirma + "=0", "grxfirma_reenvio_afirma=1", "B=2"})
	if strings.Join(got, ",") != "A=1,B=2" {
		t.Fatalf("%v", got)
	}
}
