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
	"sort"
	"strings"
	"testing"

	"grxfirma/internal/ports"
)

// fakeRegistry es un registro en memoria con las dos ramas.
type fakeRegistry struct {
	keys      map[Hive]map[string]*fakeKey
	writes    int
	failWrite int // falla la escritura número n (1..), 0 = nunca
}

type fakeKey struct {
	path   string
	values map[string]ValueSnapshot
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{keys: map[Hive]map[string]*fakeKey{CurrentUser: {}, LocalMachine: {}}}
}

func (f *fakeRegistry) ensure(hive Hive, path string) *fakeKey {
	parts := strings.Split(path, `\`)
	var key *fakeKey
	for i := range parts {
		p := strings.Join(parts[:i+1], `\`)
		lower := strings.ToLower(p)
		k, ok := f.keys[hive][lower]
		if !ok {
			k = &fakeKey{path: p, values: map[string]ValueSnapshot{}}
			f.keys[hive][lower] = k
		}
		key = k
	}
	return key
}

func (f *fakeRegistry) set(hive Hive, path, name, kind, data string) {
	f.ensure(hive, path).values[name] = ValueSnapshot{Path: path, Name: name, KeyExisted: true, ValueExisted: true, Kind: kind, Data: data}
}

func (f *fakeRegistry) Read(hive Hive, path, name string) (ValueSnapshot, error) {
	snap := absentSnapshot(path, name)
	k, ok := f.keys[hive][strings.ToLower(path)]
	if !ok {
		return snap, nil
	}
	snap.KeyExisted = true
	v, ok := k.values[name]
	if !ok {
		return snap, nil
	}
	v.Path, v.Name, v.KeyExisted = path, name, true
	return v, nil
}

func (f *fakeRegistry) Write(path, name string, value ValueSnapshot) error {
	f.writes++
	if f.failWrite > 0 && f.writes == f.failWrite {
		return errors.New("fallo simulado de escritura")
	}
	value.Path, value.Name, value.KeyExisted, value.ValueExisted = path, name, true, true
	f.ensure(CurrentUser, path).values[name] = value
	return nil
}

func (f *fakeRegistry) DeleteValue(path, name string) error {
	f.writes++
	if k, ok := f.keys[CurrentUser][strings.ToLower(path)]; ok {
		delete(k.values, name)
	}
	return nil
}

func (f *fakeRegistry) Inspect(path string) (bool, int, []string, error) {
	lower := strings.ToLower(path)
	k, ok := f.keys[CurrentUser][lower]
	if !ok {
		return false, 0, nil, nil
	}
	var subs []string
	for other, sub := range f.keys[CurrentUser] {
		if strings.HasPrefix(other, lower+`\`) && !strings.Contains(other[len(lower)+1:], `\`) {
			subs = append(subs, sub.path[len(path)+1:])
		}
	}
	sort.Strings(subs)
	return true, len(k.values), subs, nil
}

func (f *fakeRegistry) DeleteKey(path string) error {
	f.writes++
	_, _, subs, _ := f.Inspect(path)
	if len(subs) > 0 {
		return errors.New("la clave tiene subclaves")
	}
	delete(f.keys[CurrentUser], strings.ToLower(path))
	return nil
}

func (f *fakeRegistry) hasUserKey(path string) bool {
	_, ok := f.keys[CurrentUser][strings.ToLower(path)]
	return ok
}

const (
	testInstallDir   = `C:\Users\ana\AppData\Local\Programs\GrxFirma\AfirmaURI`
	testAutoFirmaExe = `C:\Program Files\Autofirma\Autofirma\Autofirma.exe`
)

type fixture struct {
	reg      *fakeRegistry
	sel      *Selector
	snapshot string
}

func newFixture(t *testing.T, autoFirma bool) *fixture {
	t.Helper()
	reg := newFakeRegistry()
	paths := PathsFor(testInstallDir)
	dir := t.TempDir()
	paths.Snapshot = filepath.Join(dir, SnapshotName)
	sel := NewSelector(reg, paths)
	existing := map[string]bool{paths.Executable: true}
	if autoFirma {
		existing[testAutoFirmaExe] = true
		// Igual que el instalador oficial (sin comillas en la ruta).
		reg.set(LocalMachine, ProtocolKey, "", KindString, "URL:Afirma Protocol")
		reg.set(LocalMachine, ProtocolKey+`\shell\open\command`, "", KindString, testAutoFirmaExe+` "%1"`)
	}
	sel.fileExists = func(p string) bool { return existing[p] }
	return &fixture{reg: reg, sel: sel, snapshot: paths.Snapshot}
}

// installLikeNSIS reproduce una instalación limpia: instantánea con los
// valores ausentes y el plan escrito en HKCU.
func (fx *fixture) installLikeNSIS(t *testing.T) {
	t.Helper()
	if err := fx.sel.register(); err != nil {
		t.Fatalf("registro inicial: %v", err)
	}
	fx.reg.writes = 0
}

func TestEstadoConGrxFirmaRegistradaYAutoFirmaInstalada(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	estado, err := fx.sel.Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !estado.Soportado || estado.Actual != ports.ProgramaAfirmaGrxFirma ||
		!estado.AutoFirmaInstalada || estado.RutaAutoFirma != testAutoFirmaExe || !estado.GrxFirmaInstalada {
		t.Fatalf("estado inesperado: %+v", estado)
	}
}

func TestElegirAutoFirmaRetiraElRegistroYVolverAGrxFirma(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	estado, err := fx.sel.Elegir(context.Background(), "autofirma")
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaAutoFirma || estado.RutaActual != testAutoFirmaExe ||
		estado.Preferencia != ports.ProgramaAfirmaAutoFirma {
		t.Fatalf("estado tras elegir AutoFirma: %+v", estado)
	}
	if fx.reg.hasUserKey(ProtocolKey) {
		t.Fatal("la clave HKCU de afirma:// debía desaparecer al no existir antes de GrxFirma")
	}
	if _, err := os.Stat(fx.snapshot); err != nil {
		t.Fatalf("la instantánea debe conservarse para volver a GrxFirma: %v", err)
	}
	// Repetir la elección no escribe nada más que la preferencia.
	fx.reg.writes = 0
	if _, err := fx.sel.Elegir(context.Background(), "autofirma"); err != nil {
		t.Fatal(err)
	}
	if fx.reg.writes != 1 {
		t.Fatalf("elegir dos veces AutoFirma hizo %d escrituras", fx.reg.writes)
	}

	estado, err = fx.sel.Elegir(context.Background(), "grxfirma")
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaGrxFirma || estado.Preferencia != ports.ProgramaAfirmaGrxFirma {
		t.Fatalf("estado tras volver a GrxFirma: %+v", estado)
	}
	cmd, _ := fx.reg.Read(CurrentUser, ProtocolKey+`\shell\open\command`, "")
	if cmd.Data != `"`+testInstallDir+`\grxfirma-afirmauri.exe" "%1"` {
		t.Fatalf("orden registrada: %q", cmd.Data)
	}
	// La instantánea conserva «nada antes de GrxFirma».
	data, _ := os.ReadFile(fx.snapshot)
	sets, _ := ownerSets(fx.sel.paths)
	state, err := parseSnapshot(data, sets[0], sets)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range state.Snapshots {
		if s.ValueExisted {
			t.Fatalf("la instantánea perdió el estado previo: %+v", s)
		}
	}
}

func TestElegirAutoFirmaRestauraElValorPrevioDeOtroPrograma(t *testing.T) {
	fx := newFixture(t, true)
	other := `"C:\Otro\firmador.exe" "%1"`
	fx.reg.set(CurrentUser, ProtocolKey+`\shell\open\command`, "", KindString, other)
	fx.installLikeNSIS(t)
	estado, err := fx.sel.Elegir(context.Background(), "autofirma")
	if err != nil {
		t.Fatal(err)
	}
	cmd, _ := fx.reg.Read(CurrentUser, ProtocolKey+`\shell\open\command`, "")
	if !cmd.ValueExisted || cmd.Data != other {
		t.Fatalf("no se restauró el valor previo: %+v", cmd)
	}
	if estado.Actual != ports.ProgramaAfirmaOtro || estado.RutaActual != `C:\Otro\firmador.exe` {
		t.Fatalf("el estado debe nombrar el programa restaurado: %+v", estado)
	}
	if v, _ := fx.reg.Read(CurrentUser, ProtocolKey, ""); v.ValueExisted {
		t.Fatalf("quedó un valor de GrxFirma: %+v", v)
	}
}

func TestNoTocaUnRegistroCambiadoPorOtroPrograma(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	fx.reg.set(CurrentUser, ProtocolKey+`\shell\open\command`, "", KindString, `"C:\Otro\x.exe" "%1"`)
	fx.reg.writes = 0
	if _, err := fx.sel.Elegir(context.Background(), "autofirma"); !errors.Is(err, ports.ErrProtocoloAfirmaAjeno) {
		t.Fatalf("se esperaba ErrProtocoloAfirmaAjeno, llegó %v", err)
	}
	if _, err := fx.sel.Elegir(context.Background(), "grxfirma"); !errors.Is(err, ports.ErrProtocoloAfirmaAjeno) {
		t.Fatalf("volver a GrxFirma debía rechazarse, llegó %v", err)
	}
	if fx.reg.writes != 0 {
		t.Fatalf("se escribieron %d valores en un registro ajeno", fx.reg.writes)
	}
}

func TestNoVuelveAGrxFirmaSiOtroProgramaOcupoElRegistroRetirado(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	if _, err := fx.sel.Elegir(context.Background(), "autofirma"); err != nil {
		t.Fatal(err)
	}
	fx.reg.set(CurrentUser, ProtocolKey+`\shell\open\command`, "", KindString, `"C:\Otro\x.exe" "%1"`)
	fx.reg.writes = 0
	if _, err := fx.sel.Elegir(context.Background(), "grxfirma"); !errors.Is(err, ports.ErrProtocoloAfirmaAjeno) {
		t.Fatalf("se esperaba ErrProtocoloAfirmaAjeno, llegó %v", err)
	}
	if fx.reg.writes != 0 {
		t.Fatalf("se escribieron %d valores", fx.reg.writes)
	}
}

func TestAutoFirmaNoInstaladaNoCambiaNada(t *testing.T) {
	fx := newFixture(t, false)
	fx.installLikeNSIS(t)
	estado, _ := fx.sel.Estado(context.Background())
	if estado.AutoFirmaInstalada {
		t.Fatal("no hay AutoFirma registrado")
	}
	if _, err := fx.sel.Elegir(context.Background(), "autofirma"); !errors.Is(err, ports.ErrAutoFirmaNoInstalada) {
		t.Fatalf("se esperaba ErrAutoFirmaNoInstalada, llegó %v", err)
	}
	if fx.reg.writes != 0 {
		t.Fatalf("se escribieron %d valores", fx.reg.writes)
	}
}

func TestAutoFirmaRegistradaPeroSinEjecutableNoCuenta(t *testing.T) {
	fx := newFixture(t, false)
	fx.reg.set(LocalMachine, ProtocolKey+`\shell\open\command`, "", KindString, testAutoFirmaExe+` "%1"`)
	estado, _ := fx.sel.Estado(context.Background())
	if estado.AutoFirmaInstalada {
		t.Fatal("sin el ejecutable AutoFirma no está instalado")
	}
	fx.reg.set(LocalMachine, ProtocolKey+`\shell\open\command`, "", KindString, `C:\Malo\otro.exe "%1"`)
	fx.sel.fileExists = func(string) bool { return true }
	if estado, _ = fx.sel.Estado(context.Background()); estado.AutoFirmaInstalada {
		t.Fatal("un ejecutable que no es Autofirma.exe no cuenta como AutoFirma")
	}
}

func TestReconoceElIconoDeVersionesAnteriores(t *testing.T) {
	fx := newFixture(t, true)
	plan := registrationPlan(fx.sel.paths.Executable, testInstallDir+`\grxfirma-diputacion.ico`)
	for _, v := range plan {
		fx.reg.set(CurrentUser, v.Path, v.Name, KindString, v.Data)
	}
	estado, err := fx.sel.Estado(context.Background())
	if err != nil || estado.Actual != ports.ProgramaAfirmaGrxFirma {
		t.Fatalf("estado con icono de la 0.0.118: %+v %v", estado, err)
	}
	if _, err := fx.sel.Elegir(context.Background(), "autofirma"); err != nil {
		t.Fatalf("debía retirar el registro de la 0.0.118: %v", err)
	}
	if fx.reg.hasUserKey(ProtocolKey) {
		t.Fatal("quedó la clave de afirma://")
	}
}

func TestRechazaUnaInstantaneaManipulada(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	data, _ := os.ReadFile(fx.snapshot)
	tampered := strings.Replace(string(data), `Software\\Classes\\afirma\\DefaultIcon`, `Software\\Classes\\otro`, 1)
	if err := os.WriteFile(fx.snapshot, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.sel.Elegir(context.Background(), "autofirma"); !errors.Is(err, ports.ErrProtocoloAfirmaAjeno) {
		t.Fatalf("se esperaba rechazo, llegó %v", err)
	}
	if fx.reg.writes != 0 {
		t.Fatalf("se escribieron %d valores", fx.reg.writes)
	}
}

func TestLeeLaInstantaneaDelInstaladorPowerShell(t *testing.T) {
	// Formato de ConvertTo-Json de Write-AfirmaProtocolSnapshot, con un valor
	// previo MultiString de un elemento y otro DWord.
	exe := testInstallDir + `\grxfirma-afirmauri.exe`
	ico := testInstallDir + `\grxfirma-grx.ico`
	esc := func(s string) string { return strings.ReplaceAll(s, `\`, `\\`) }
	doc := `{
    "schema_version":  1,
    "protocol_key":  "Software\\Classes\\afirma",
    "owner_values":  [
        {"Path":"Software\\Classes\\afirma","Name":"","KeyExisted":true,"ValueExisted":true,"Kind":"String","Value":"URL:GrxFirma Protocol"},
        {"Path":"Software\\Classes\\afirma","Name":"URL Protocol","KeyExisted":true,"ValueExisted":true,"Kind":"String","Value":""},
        {"Path":"Software\\Classes\\afirma\\DefaultIcon","Name":"","KeyExisted":true,"ValueExisted":true,"Kind":"String","Value":"` + esc(ico) + `,0"},
        {"Path":"Software\\Classes\\afirma\\shell\\open\\command","Name":"","KeyExisted":true,"ValueExisted":true,"Kind":"String","Value":"\"` + esc(exe) + `\" \"%1\""}
    ],
    "snapshots":  [
        {"Path":"Software\\Classes\\afirma","Name":"","KeyExisted":true,"ValueExisted":true,"Kind":"MultiString","Value":"uno"},
        {"Path":"Software\\Classes\\afirma","Name":"URL Protocol","KeyExisted":true,"ValueExisted":true,"Kind":"DWord","Value":"7"},
        {"Path":"Software\\Classes\\afirma\\DefaultIcon","Name":"","KeyExisted":false,"ValueExisted":false,"Kind":null,"Value":null},
        {"Path":"Software\\Classes\\afirma\\shell\\open\\command","Name":"","KeyExisted":false,"ValueExisted":false,"Kind":null,"Value":null}
    ]
}`
	sets, _ := ownerSets(PathsFor(testInstallDir))
	state, err := parseSnapshot([]byte("\xef\xbb\xbf"+doc), sets[0], sets)
	if err != nil {
		t.Fatal(err)
	}
	if state.Snapshots[0].Kind != KindMultiString || len(state.Snapshots[0].Multi) != 1 ||
		state.Snapshots[1].Data != "7" || state.Snapshots[2].ValueExisted {
		t.Fatalf("instantánea mal leída: %+v", state.Snapshots)
	}
	// Y lo que escribe Go lo vuelve a leer igual.
	encoded, err := encodeSnapshot(state.OwnerValues, state.Snapshots)
	if err != nil {
		t.Fatal(err)
	}
	again, err := parseSnapshot(encoded, sets[0], sets)
	if err != nil || !valuesMatchSet(again.Snapshots, state.Snapshots) {
		t.Fatalf("ida y vuelta: %v", err)
	}
}

func TestRollbackSiFallaUnaEscritura(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	if _, err := fx.sel.Elegir(context.Background(), "autofirma"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(fx.snapshot)
	fx.reg.writes = 0
	fx.reg.failWrite = 3
	if _, err := fx.sel.Elegir(context.Background(), "grxfirma"); err == nil {
		t.Fatal("se esperaba el fallo simulado")
	}
	if fx.reg.hasUserKey(ProtocolKey) {
		t.Fatal("el rollback debía dejar HKCU como estaba (sin afirma://)")
	}
	after, _ := os.ReadFile(fx.snapshot)
	if string(before) != string(after) {
		t.Fatal("el rollback debía restaurar la instantánea anterior")
	}
	if p := fx.sel.preference(); p != ports.ProgramaAfirmaAutoFirma {
		t.Fatalf("la preferencia no debe cambiar si falla: %q", p)
	}
}

func TestProgramaDesconocido(t *testing.T) {
	fx := newFixture(t, true)
	if _, err := fx.sel.Elegir(context.Background(), "otro"); !errors.Is(err, ports.ErrProgramaAfirmaDesconocido) {
		t.Fatalf("llegó %v", err)
	}
}

func TestSinRegistroDeGrxFirmaYSinAutoFirmaNoHayPrograma(t *testing.T) {
	fx := newFixture(t, false)
	estado, err := fx.sel.Estado(context.Background())
	if err != nil || estado.Actual != ports.ProgramaAfirmaNinguno {
		t.Fatalf("%+v %v", estado, err)
	}
}

func TestCommandExecutable(t *testing.T) {
	cases := map[string]string{
		`"C:\Program Files\GrxFirma\a.exe" "%1"`:                  `C:\Program Files\GrxFirma\a.exe`,
		`C:\Program Files\Autofirma\Autofirma\Autofirma.exe "%1"`: `C:\Program Files\Autofirma\Autofirma\Autofirma.exe`,
		`C:\Program Files\Autofirma\Autofirma\Autofirma.EXE`:      `C:\Program Files\Autofirma\Autofirma\Autofirma.EXE`,
		`C:\dir.exe.d\prog.exe %1`:                                `C:\dir.exe.d\prog.exe`,
		``:                                                        ``,
		`"sin cierre`:                                             ``,
	}
	for in, want := range cases {
		if got := commandExecutable(in); got != want {
			t.Errorf("commandExecutable(%q) = %q, se esperaba %q", in, got, want)
		}
	}
}

func TestSelectorNoSoportado(t *testing.T) {
	estado, err := unsupported{}.Estado(context.Background())
	if err != nil || estado.Soportado {
		t.Fatalf("%+v %v", estado, err)
	}
	if _, err := (unsupported{}).Elegir(context.Background(), "grxfirma"); !errors.Is(err, ports.ErrProtocoloAfirmaNoSoportado) {
		t.Fatalf("llegó %v", err)
	}
}
