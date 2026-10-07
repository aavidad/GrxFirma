// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// mimeapps.list y las rutas XDG solo existen en sistemas tipo Unix; en Windows
// filepath convierte las rutas de prueba y las comparaciones no tienen sentido.

//go:build !windows

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

type xdgFixture struct {
	t      *testing.T
	home   string
	system string
	env    XDGEnv
}

// newXDGFixture prepara un HOME temporal y un /usr/share falso.
func newXDGFixture(t *testing.T, vars map[string]string) *xdgFixture {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	system := filepath.Join(root, "usr", "share")
	if vars == nil {
		vars = map[string]string{}
	}
	if _, ok := vars["XDG_DATA_DIRS"]; !ok {
		vars["XDG_DATA_DIRS"] = system
	}
	if _, ok := vars["XDG_CONFIG_DIRS"]; !ok {
		vars["XDG_CONFIG_DIRS"] = filepath.Join(root, "etc", "xdg")
	}
	env := XDGEnvFrom(home, func(k string) string { return vars[k] })
	return &xdgFixture{t: t, home: home, system: system, env: env}
}

func (f *xdgFixture) write(path, content string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *xdgFixture) read(path string) string {
	f.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

func (f *xdgFixture) installGrxFirma() {
	f.write(filepath.Join(f.env.DataHome, "applications", GrxFirmaDesktopID), "[Desktop Entry]\n"+
		"Type=Application\nName=GrxFirma\n"+
		"Exec=env GRXFIRMA_PROTOCOL_UI=1 "+filepath.Join(f.home, ".local", "lib", "grxfirma", "afirmauri-handler.sh")+" %u\n"+
		"MimeType=x-scheme-handler/afirmav2;x-scheme-handler/afirma;\n")
}

// installAutoFirma imita el paquete oficial: /usr/share/applications/afirma.desktop
// con Exec=/usr/bin/autofirma %u.
func (f *xdgFixture) installAutoFirma() string {
	bin := filepath.Join(filepath.Dir(f.system), "bin", "autofirma")
	f.write(bin, "#!/bin/sh\n")
	f.write(filepath.Join(f.system, "applications", "afirma.desktop"), "[Desktop Entry]\n"+
		"Encoding=UTF-8\nName=Autofirma\nType=Application\n"+
		"Exec="+bin+" %u\nMimeType=x-scheme-handler/afirma;\n")
	return bin
}

func (f *xdgFixture) mimeapps() string {
	return filepath.Join(f.env.ConfigHome, "mimeapps.list")
}

func TestXDGEnvFromDefaultsAndIgnoresRelativePaths(t *testing.T) {
	env := XDGEnvFrom("/home/ana", func(k string) string {
		return map[string]string{
			"XDG_CONFIG_HOME":     "relativa",
			"XDG_DATA_DIRS":       "otra:/opt/share",
			"XDG_CURRENT_DESKTOP": "ubuntu:GNOME:../x",
		}[k]
	})
	if env.ConfigHome != "/home/ana/.config" || env.DataHome != "/home/ana/.local/share" {
		t.Fatalf("rutas por defecto: %+v", env)
	}
	if len(env.DataDirs) != 1 || env.DataDirs[0] != "/opt/share" {
		t.Fatalf("XDG_DATA_DIRS: %v", env.DataDirs)
	}
	if strings.Join(env.Desktops, ",") != "ubuntu,gnome" {
		t.Fatalf("escritorios: %v", env.Desktops)
	}
	if strings.Join(env.ConfigDirs, ",") != "/etc/xdg" {
		t.Fatalf("XDG_CONFIG_DIRS: %v", env.ConfigDirs)
	}
}

func TestXDGWithoutAutoFirmaDisablesTheOption(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	f.write(f.mimeapps(), "[Default Applications]\nx-scheme-handler/afirma=grxfirma.desktop\n")
	sel := NewXDGSelector(f.env)

	estado, err := sel.Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !estado.Soportado || !estado.GrxFirmaInstalada || estado.AutoFirmaInstalada ||
		estado.Actual != ports.ProgramaAfirmaGrxFirma {
		t.Fatalf("estado: %+v", estado)
	}
	before := f.read(f.mimeapps())
	if _, err := sel.Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma); !errors.Is(err, ports.ErrAutoFirmaNoInstalada) {
		t.Fatalf("Elegir AutoFirma sin instalar: %v", err)
	}
	if f.read(f.mimeapps()) != before {
		t.Fatal("se modificó mimeapps.list sin AutoFirma")
	}
	if _, err := os.Stat(sel.preferencePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se guardó la preferencia: %v", err)
	}
}

func TestXDGSwitchesBetweenGrxFirmaAndAutoFirmaKeepingTheRest(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	bin := f.installAutoFirma()
	original := "# mis asociaciones\n" +
		"[Default Applications]\n" +
		"x-scheme-handler/afirma=grxfirma.desktop\n" +
		"x-scheme-handler/afirmav2=grxfirma.desktop\n" +
		"application/pdf=org.gnome.Evince.desktop\n" +
		"\n" +
		"[Added Associations]\n" +
		"application/pdf=org.gnome.Evince.desktop;firefox.desktop;\n"
	f.write(f.mimeapps(), original)
	if err := os.Chmod(f.mimeapps(), 0o640); err != nil {
		t.Fatal(err)
	}
	sel := NewXDGSelector(f.env)

	estado, err := sel.Elegir(context.Background(), "AutoFirma")
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaAutoFirma || estado.RutaActual != bin ||
		estado.RutaAutoFirma != bin || estado.Preferencia != ports.ProgramaAfirmaAutoFirma {
		t.Fatalf("estado tras elegir AutoFirma: %+v", estado)
	}
	want := strings.Replace(original, "afirma=grxfirma.desktop", "afirma=afirma.desktop", 1)
	if got := f.read(f.mimeapps()); got != want {
		t.Fatalf("mimeapps.list:\n%s\nesperado:\n%s", got, want)
	}
	if info, err := os.Stat(f.mimeapps()); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("permisos no conservados: %v %v", info, err)
	}

	estado, err = sel.Elegir(context.Background(), ports.ProgramaAfirmaGrxFirma)
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaGrxFirma || estado.Preferencia != ports.ProgramaAfirmaGrxFirma {
		t.Fatalf("estado tras volver a GrxFirma: %+v", estado)
	}
	if got := f.read(f.mimeapps()); got != original {
		t.Fatalf("no se recuperó el fichero original:\n%s", got)
	}
	leftovers, _ := filepath.Glob(filepath.Join(f.env.ConfigHome, ".mimeapps.list.tmp-*"))
	if len(leftovers) != 0 {
		t.Fatalf("quedaron temporales: %v", leftovers)
	}
}

func TestXDGCreatesMimeappsWhenMissing(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	f.installAutoFirma()
	sel := NewXDGSelector(f.env)

	estado, err := sel.Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaNinguno {
		t.Fatalf("con los dos instalados y sin elección: %+v", estado)
	}
	if _, err := sel.Elegir(context.Background(), ports.ProgramaAfirmaGrxFirma); err != nil {
		t.Fatal(err)
	}
	if got := f.read(f.mimeapps()); got != "[Default Applications]\nx-scheme-handler/afirma=grxfirma.desktop\n" {
		t.Fatalf("mimeapps.list nuevo: %q", got)
	}
	if got := strings.TrimSpace(f.read(sel.preferencePath())); got != ports.ProgramaAfirmaGrxFirma {
		t.Fatalf("preferencia: %q", got)
	}
}

func TestXDGUpdatesOtherUserFilesOnlyWhenTheyNameAfirma(t *testing.T) {
	f := newXDGFixture(t, map[string]string{"XDG_CURRENT_DESKTOP": "ubuntu:GNOME"})
	f.installGrxFirma()
	f.installAutoFirma()
	gnome := filepath.Join(f.env.ConfigHome, "gnome-mimeapps.list")
	legacy := filepath.Join(f.env.DataHome, "applications", "mimeapps.list")
	snap := filepath.Join(f.home, "snap", "firefox", "common", ".config", "mimeapps.list")
	ubuntu := filepath.Join(f.env.ConfigHome, "ubuntu-mimeapps.list")
	f.write(gnome, "[Default Applications]\nx-scheme-handler/afirma=grxfirma.desktop\n")
	f.write(legacy, "[Default Applications]\r\nx-scheme-handler/afirma=grxfirma.desktop\r\n")
	f.write(snap, "[Default Applications]\nx-scheme-handler/afirma=grxfirma.desktop\n")
	f.write(ubuntu, "[Default Applications]\ntext/plain=gedit.desktop\n")
	sel := NewXDGSelector(f.env)

	estado, err := sel.Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma)
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaAutoFirma {
		t.Fatalf("estado: %+v", estado)
	}
	for _, path := range []string{gnome, snap, f.mimeapps()} {
		if !strings.Contains(f.read(path), "x-scheme-handler/afirma=afirma.desktop\n") {
			t.Fatalf("%s no cambió:\n%s", path, f.read(path))
		}
	}
	if got := f.read(legacy); got != "[Default Applications]\r\nx-scheme-handler/afirma=afirma.desktop\r\n" {
		t.Fatalf("no se conservó CRLF: %q", got)
	}
	if got := f.read(ubuntu); got != "[Default Applications]\ntext/plain=gedit.desktop\n" {
		t.Fatalf("se tocó un fichero que no nombraba afirma://: %q", got)
	}
	if _, err := os.Stat(filepath.Join(f.home, ".var", "app", "org.mozilla.firefox", "config", "mimeapps.list")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se creó el mimeapps.list de Flatpak: %v", err)
	}
}

func TestXDGKeepsSymlinkedMimeapps(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	f.installAutoFirma()
	target := filepath.Join(f.home, "dotfiles", "mimeapps.list")
	f.write(target, "[Default Applications]\nx-scheme-handler/afirma=grxfirma.desktop\n")
	if err := os.MkdirAll(f.env.ConfigHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, f.mimeapps()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewXDGSelector(f.env).Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(f.mimeapps()); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("el enlace se sustituyó: %v %v", info, err)
	}
	if !strings.Contains(f.read(target), "afirma=afirma.desktop") {
		t.Fatalf("el destino no cambió: %s", f.read(target))
	}
}

func TestXDGReportsOtherProgramAndFallbacks(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	f.write(filepath.Join(f.env.DataHome, "applications", "otro.desktop"),
		"[Desktop Entry]\nExec=\"/opt/Otro Firma/otro\" %u\nMimeType=x-scheme-handler/afirma;\n")
	// El primero de la lista no está instalado: se usa el siguiente.
	f.write(f.mimeapps(), "[Default Applications]\nx-scheme-handler/afirma=falta.desktop;otro.desktop;\n")
	sel := NewXDGSelector(f.env)
	estado, err := sel.Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaOtro || estado.RutaActual != "/opt/Otro Firma/otro" {
		t.Fatalf("estado con otro programa: %+v", estado)
	}

	// Sin elección expresa y solo GrxFirma instalado, abre GrxFirma.
	f.write(f.mimeapps(), "[Added Associations]\n")
	estado, err = sel.Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaGrxFirma {
		t.Fatalf("estado por defecto: %+v", estado)
	}
}

func TestXDGIgnoresFakeOrHiddenAutoFirma(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	// Un afirma.desktop que no lanza AutoFirma no cuenta como AutoFirma.
	f.write(filepath.Join(f.system, "applications", "afirma.desktop"),
		"[Desktop Entry]\nExec=/usr/bin/otra-cosa %u\nMimeType=x-scheme-handler/afirma;\n")
	// Uno que apunta a un ejecutable inexistente tampoco.
	f.write(filepath.Join(f.system, "applications", "autofirma.desktop"),
		"[Desktop Entry]\nExec=/no/existe/autofirma %u\nMimeType=x-scheme-handler/afirma;\n")
	sel := NewXDGSelector(f.env)
	estado, err := sel.Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if estado.AutoFirmaInstalada {
		t.Fatalf("AutoFirma falso detectado: %+v", estado)
	}

	// Un lanzador oculto del usuario tapa al del sistema.
	f2 := newXDGFixture(t, nil)
	f2.installAutoFirma()
	f2.write(filepath.Join(f2.env.DataHome, "applications", "afirma.desktop"), "[Desktop Entry]\nHidden=true\n")
	estado, err = NewXDGSelector(f2.env).Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if estado.AutoFirmaInstalada || estado.GrxFirmaInstalada {
		t.Fatalf("lanzador oculto: %+v", estado)
	}
	if _, err := NewXDGSelector(f2.env).Elegir(context.Background(), ports.ProgramaAfirmaGrxFirma); !errors.Is(err, ports.ErrGrxFirmaAfirmaNoInstalada) {
		t.Fatalf("Elegir GrxFirma sin instalar: %v", err)
	}
	if _, err := NewXDGSelector(f2.env).Elegir(context.Background(), "../x"); !errors.Is(err, ports.ErrProgramaAfirmaDesconocido) {
		t.Fatalf("programa no válido: %v", err)
	}
}

func TestXDGRejectsOversizedMimeapps(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	f.installAutoFirma()
	f.write(f.mimeapps(), strings.Repeat("#", maxXDGFileBytes+1))
	sel := NewXDGSelector(f.env)
	if _, err := sel.Estado(context.Background()); err == nil {
		t.Fatal("se aceptó un mimeapps.list desmesurado")
	}
	if _, err := sel.Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma); err == nil {
		t.Fatal("se reescribió un mimeapps.list desmesurado")
	}
}

func TestSetDefaultApplication(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"vacío", "", "[Default Applications]\nk=v\n"},
		{"sin sección", "[Added Associations]\na=b;", "[Added Associations]\na=b;\n\n[Default Applications]\nk=v\n"},
		{"sección sin clave", "[Default Applications]\na=b\n\n[Otra]\nc=d\n", "[Default Applications]\na=b\nk=v\n\n[Otra]\nc=d\n"},
		{"duplicados", "[Default Applications]\nk = x\nk=y\na=b\n", "[Default Applications]\nk=v\na=b\n"},
		{"clave en otra sección", "[Added Associations]\nk=x;\n[Default Applications]\n", "[Added Associations]\nk=x;\n[Default Applications]\nk=v\n"},
		{"dos secciones", "[Default Applications]\na=b\n[X]\n[Default Applications]\nc=d\n", "[Default Applications]\na=b\nk=v\n[X]\n[Default Applications]\nc=d\n"},
	}
	for _, c := range cases {
		got, _ := setDefaultApplication([]byte(c.in), "k", "v")
		if string(got) != c.want {
			t.Errorf("%s: %q, esperado %q", c.name, got, c.want)
		}
	}
}

func TestExecProgram(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/autofirma %u":                            "/usr/bin/autofirma",
		"env GRXFIRMA_PROTOCOL_UI=1 grxfirma-afirmauri %u": "grxfirma-afirmauri",
		`"/opt/Mi Firma/bin/firma" %U`:                     "/opt/Mi Firma/bin/firma",
		"":                                                 "",
	}
	for in, want := range cases {
		if got := execProgram(in); got != want {
			t.Errorf("execProgram(%q) = %q, esperado %q", in, got, want)
		}
	}
}
