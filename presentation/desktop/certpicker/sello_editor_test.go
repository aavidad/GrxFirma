// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/pdffixture"
)

func TestValidarResultadoEditorSello(t *testing.T) {
	valid := `{"action":"place","visibleSealPlacements":[{"page":1,"rect":{"x":0.2,"y":0.1,"w":0.3,"h":0.2},"rotation":45}]}`
	tests := []struct {
		name, raw string
		wantErr   bool
	}{
		{"válido", valid, false},
		{"acción desconocida", `{"action":"other"}`, true},
		{"clave ajena", strings.Replace(valid, `"action":"place"`, `"action":"place","extra":true`, 1), true},
		{"clave ajena en rectángulo", strings.Replace(valid, `"w":0.3`, `"w":0.3,"extra":1`, 1), true},
		{"página inexistente", strings.Replace(valid, `"page":1`, `"page":2`, 1), true},
		{"fuera", strings.Replace(valid, `"x":0.2`, `"x":0.9`, 1), true},
		{"mínimo", strings.Replace(valid, `"w":0.3`, `"w":0.001`, 1), true},
		{"giro", strings.Replace(valid, `"rotation":45`, `"rotation":360`, 1), true},
		{"resto", valid + `{}`, true},
		{"clave duplicada", strings.Replace(valid, `"action":"place"`, `"action":"place","action":"cancel"`, 1), true},
		{"nulo", `null`, true},
		{"apariencia válida", strings.Replace(valid, `"rotation":45}]`, `"rotation":45}],"appearance":{"logo":"institutional","opacityPercent":40}`, 1), false},
		{"apariencia ajena", strings.Replace(valid, `"rotation":45}]`, `"rotation":45}],"appearance":{"logo":"custom","opacityPercent":40}`, 1), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidarResultadoEditorSello([]byte(tc.raw), 1)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
	if _, err := ValidarResultadoEditorSello(make([]byte, MaxResultadoEditorSello+1), 1); err == nil {
		t.Fatal("se aceptó un resultado demasiado grande")
	}
	if _, err := ValidarResultadoEditorSello([]byte(`{"action":"cancel"}`), 1); !errors.Is(err, ErrSeleccionCancelada) {
		t.Fatalf("cancelación=%v", err)
	}
	if _, err := ValidarResultadoEditorSello([]byte(`{"action":"without"}`), 1); !errors.Is(err, ErrSinSelloVisible) {
		t.Fatalf("sin sello=%v", err)
	}
	if _, err := ValidarResultadoEditorSello([]byte(`{"action":"cancel","appearance":{"logo":"text","opacityPercent":100}}`), 1); err == nil || errors.Is(err, ErrSeleccionCancelada) {
		t.Fatalf("cancelación con opciones=%v", err)
	}
}

type selectorEditorPrueba struct {
	raw                        []byte
	err                        error
	editorCalls, fallbackCalls int
}

func (s *selectorEditorPrueba) ElegirSelloEnEditor(context.Context, domain.Document, string) ([]byte, error) {
	s.editorCalls++
	return s.raw, s.err
}
func (s *selectorEditorPrueba) ElegirPosicionSello(context.Context) (string, string, error) {
	s.fallbackCalls++
	return "inferior-derecha", "1", nil
}

func TestResolverSelloVisibleEditorYAlternativas(t *testing.T) {
	pdf := domain.Document{Name: "document.pdf", Content: pdffixture.Minimal()}
	options := map[string]string{"visibleSignature": "want", "visibleSealPlacements": `[{"page":99}]`,
		"signaturePositionOnPageLowerLeftX": "99", "signaturePage": "9", domain.OpcionPosicionSello: "superior-izquierda"}
	valid := []byte(`{"action":"place","visibleSealPlacements":[{"page":1,"rect":{"x":0.2,"y":0.1,"w":0.3,"h":0.2},"rotation":45}]}`)
	for _, tc := range []struct {
		name              string
		doc               domain.Document
		raw               []byte
		editorErr         error
		fallback, wantErr bool
	}{
		{"editor", pdf, valid, nil, false, false},
		{"sin documento", domain.Document{}, valid, nil, true, false},
		{"PDF ilegible", domain.Document{Content: []byte("%PDF cifrado")}, valid, nil, true, false},
		{"interfaz ausente", pdf, nil, ErrEditorSelloNoDisponible, true, false},
		{"fallo de arranque", pdf, nil, ErrEditorSelloNoDisponible, true, false},
		{"resultado inválido", pdf, []byte(`{"action":"place","extra":1}`), nil, false, true},
		{"cancelar", pdf, []byte(`{"action":"cancel"}`), nil, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sel := &selectorEditorPrueba{raw: tc.raw, err: tc.editorErr}
			out, err := ResolverSelloVisible(context.Background(), sel, domain.FormatPAdES, options, tc.doc, "Firmante")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v", err)
			}
			if tc.fallback != (sel.fallbackCalls == 1) {
				t.Fatalf("diálogo de posiciones: %d", sel.fallbackCalls)
			}
			if !tc.wantErr && out["visibleSealPlacements"] == `[{"page":99}]` {
				t.Fatal("se aceptaron coordenadas de la web")
			}
			if !tc.wantErr && (out["signaturePositionOnPageLowerLeftX"] != "" || out["signaturePage"] != "") {
				t.Fatal("se conservaron coordenadas Java de la web")
			}
			if tc.name == "editor" && !strings.Contains(out["visibleSealPlacements"], `"rotation":45`) {
				t.Fatalf("colocación=%v", out)
			}
		})
	}
	withLogo := strings.Replace(string(valid), `"rotation":45}]`, `"rotation":45}],"appearance":{"logo":"institutional","opacityPercent":40}`, 1)
	sel := &selectorEditorPrueba{raw: []byte(withLogo)}
	out, err := ResolverSelloVisible(context.Background(), sel, domain.FormatPAdES, options, pdf, "Firmante")
	if err != nil || out["visibleSealLogo"] != "institucional" || out["visibleSealLogoOpacityPercent"] != "40" {
		t.Fatalf("apariencia: opciones=%v error=%v", out, err)
	}
}

func TestEjecutarEditorSelloBorraTemporales(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("prueba del proceso simulado en Linux")
	}
	root := t.TempDir()
	program := filepath.Join(root, "editor.sh")
	logPath := filepath.Join(root, "request-path")
	script := "#!/bin/sh\nprintf '%s' \"$2\" > " + strconv.Quote(logPath) + "\npython3 -c 'import json,sys; r=json.load(open(sys.argv[1])); open(r[\"resultPath\"],\"w\").write(\"{\\\"action\\\":\\\"without\\\"}\")' \"$2\"\n"
	if err := os.WriteFile(program, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	old := resolverEjecutableEditorSello
	resolverEjecutableEditorSello = func() (string, error) { return program, nil }
	t.Cleanup(func() { resolverEjecutableEditorSello = old })
	raw, err := EjecutarEditorSello(context.Background(), domain.Document{Content: pdffixture.Minimal()}, "Firmante")
	if err != nil || string(raw) != `{"action":"without"}` {
		t.Fatalf("resultado=%q error=%v", raw, err)
	}
	request, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(string(request))); !os.IsNotExist(err) {
		t.Fatalf("el directorio temporal sigue presente: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := EjecutarEditorSello(ctx, domain.Document{Content: pdffixture.Minimal()}, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelación=%v", err)
	}
	failure := "#!/bin/sh\nprintf '%s' \"$2\" > " + strconv.Quote(logPath) + "\nexit 1\n"
	if err := os.WriteFile(program, []byte(failure), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := EjecutarEditorSello(context.Background(), domain.Document{Content: pdffixture.Minimal()}, ""); !errors.Is(err, ErrEditorSelloNoDisponible) {
		t.Fatalf("fallo de arranque=%v", err)
	}
	request, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(string(request))); !os.IsNotExist(err) {
		t.Fatalf("el directorio temporal del fallo sigue presente: %v", err)
	}
	waiting := "#!/bin/sh\nprintf '%s' \"$2\" > " + strconv.Quote(logPath) + "\nexec sleep 5\n"
	if err := os.WriteFile(program, []byte(waiting), 0700); err != nil {
		t.Fatal(err)
	}
	limited, stop := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer stop()
	if _, err := EjecutarEditorSello(limited, domain.Document{Content: pdffixture.Minimal()}, ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("plazo=%v", err)
	}
	request, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(string(request))); !os.IsNotExist(err) {
		t.Fatalf("el directorio temporal del plazo sigue presente: %v", err)
	}
}

// Regresión 0.0.118: el editor WinUI escribía la posición movida y caía al
// salir; el error del proceso descartaba esa decisión y aparecía el diálogo
// de posiciones fijas. Solo se aprovecha un resultado completo y válido.
func TestEjecutarEditorSelloAprovechaResultadoValidoTrasFallo(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("prueba del proceso simulado en Linux")
	}
	root := t.TempDir()
	program := filepath.Join(root, "editor.sh")
	resultFile := filepath.Join(root, "result.txt")
	script := "#!/bin/sh\npython3 -c 'import json,sys; r=json.load(open(sys.argv[1])); open(r[\"resultPath\"],\"w\").write(open(sys.argv[2]).read())' \"$2\" " + strconv.Quote(resultFile) + "\n"
	old := resolverEjecutableEditorSello
	resolverEjecutableEditorSello = func() (string, error) { return program, nil }
	t.Cleanup(func() { resolverEjecutableEditorSello = old })
	movido := `{"action":"place","visibleSealPlacements":[{"page":1,"rect":{"x":0.1,"y":0.7,"w":0.3,"h":0.08},"rotation":0}]}`
	tests := []struct {
		name, result, exit string
		want               string
	}{
		{"posición movida y código de error", movido, "exit 3", movido},
		{"posición movida y caída por señal", movido, "kill -SEGV $$", movido},
		{"cancelación y código de error", `{"action":"cancel"}`, "exit 1", `{"action":"cancel"}`},
		{"sin sello y código de error", `{"action":"without"}`, "exit 1", `{"action":"without"}`},
		{"JSON truncado", movido[:40], "exit 1", ""},
		{"colocación fuera de la página", `{"action":"place","visibleSealPlacements":[{"page":1,"rect":{"x":0.9,"y":0.7,"w":0.3,"h":0.08},"rotation":0}]}`, "exit 1", ""},
		{"página inexistente", `{"action":"place","visibleSealPlacements":[{"page":2,"rect":{"x":0.1,"y":0.1,"w":0.3,"h":0.08},"rotation":0}]}`, "exit 1", ""},
		{"clave duplicada", `{"action":"without","action":"place"}`, "exit 1", ""},
		{"campo desconocido", `{"action":"without","extra":1}`, "exit 1", ""},
		{"datos tras el JSON", `{"action":"without"} {}`, "exit 1", ""},
		{"vacío", "", "exit 1", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(resultFile, []byte(tc.result), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(program, []byte(script+tc.exit+"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			raw, err := EjecutarEditorSello(context.Background(), domain.Document{Content: pdffixture.Minimal()}, "")
			if tc.want == "" {
				if !errors.Is(err, ErrEditorSelloNoDisponible) || raw != nil {
					t.Fatalf("debía volver al diálogo: resultado=%q error=%v", raw, err)
				}
				return
			}
			if err != nil || string(raw) != tc.want {
				t.Fatalf("resultado=%q error=%v", raw, err)
			}
		})
	}
	// Sin result.json el error del proceso sigue llevando al diálogo.
	if err := os.WriteFile(program, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := EjecutarEditorSello(context.Background(), domain.Document{Content: pdffixture.Minimal()}, ""); !errors.Is(err, ErrEditorSelloNoDisponible) {
		t.Fatalf("sin resultado=%v", err)
	}
}

func TestPDFProtegidoVuelveAlDialogo(t *testing.T) {
	qpdf, err := exec.LookPath("qpdf")
	if err != nil {
		t.Skip("qpdf no está instalado")
	}
	dir := t.TempDir()
	in, out := filepath.Join(dir, "plain.pdf"), filepath.Join(dir, "encrypted.pdf")
	if err := os.WriteFile(in, pdffixture.Minimal(), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(qpdf, "--encrypt", "password", "password", "256", "--", in, out).CombinedOutput(); err != nil {
		t.Fatalf("qpdf: %v: %s", err, output)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	sel := &selectorEditorPrueba{}
	options, err := ResolverSelloVisible(context.Background(), sel, domain.FormatPAdES, map[string]string{"visibleSignature": "want"}, domain.Document{Content: data}, "")
	if err != nil || sel.editorCalls != 0 || sel.fallbackCalls != 1 || options[domain.OpcionPosicionSello] != "inferior-derecha" {
		t.Fatalf("fallback cifrado: options=%v editor=%d fallback=%d error=%v", options, sel.editorCalls, sel.fallbackCalls, err)
	}
}

func TestEntornoEditorSelloNoHeredaCredenciales(t *testing.T) {
	out := entornoEditorSello([]string{
		"PATH=/usr/bin", "DISPLAY=:0", "GRXFIRMA_REST_TOKEN=privado",
		"GRXFIRMA_PKCS12_PASSWORD=privado", "AWS_ACCESS_KEY_ID=privado",
		"XDG_ACTIVATION_TOKEN=privado", "AUTOFIRMAV2_REST_TOKEN=privado",
	})
	if len(out) != 2 || out[0] != "PATH=/usr/bin" || out[1] != "DISPLAY=:0" {
		t.Fatalf("entorno filtrado=%v", out)
	}
}

func TestCoordenadasWebSinWantNoSeAplican(t *testing.T) {
	in := map[string]string{"signaturePositionOnPageLowerLeftX": "25", "signatureField": "Firma1",
		"visibleSealPlacements": `[{"page":1}]`, "visibleSeal": "true", "otra": "valor"}
	out, err := ResolverSelloVisible(context.Background(), struct{}{}, domain.FormatPAdES, in, domain.Document{}, "")
	if err != nil || len(out) != 1 || out["otra"] != "valor" {
		t.Fatalf("opciones inseguras: %v error=%v", out, err)
	}
}
