// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package padesviewer arranca un servidor HTTP efimero en localhost que sirve
// el visor HTML del documento PAdES. El servidor escucha en un puerto aleatorio
// libre y se cierra automaticamente cuando el contexto se cancela.
//
// Flujo de uso:
//
//  1. El adaptador IPC recibe la accion open_pades_viewer.
//  2. Llama a servidor.Abrir(ctx, pdfData, nombre) que arranca el HTTP server.
//  3. Devuelve la URL al cliente Qt, que la abre en el navegador o en un WebView.
package padesviewer

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/ports"
)

// Servidor gestiona el servidor HTTP efimero del visor PAdES.
type Servidor struct {
	mu      sync.Mutex
	activos map[string]*http.Server
	loc     ports.Localizador
}

// New construye un Servidor de visor PAdES.
func New() *Servidor {
	return &Servidor{activos: make(map[string]*http.Server)}
}

// WithLocalizador inyecta un localizador para el visor HTML.
func (s *Servidor) WithLocalizador(loc ports.Localizador) *Servidor {
	if s == nil {
		return nil
	}
	s.loc = loc
	return s
}

// Abrir arranca un servidor HTTP efimero que sirve el visor del PDF dado.
// Devuelve la URL local donde el cliente puede abrir el visor.
// El servidor se apaga cuando ctx se cancela.
func (s *Servidor) Abrir(ctx context.Context, pdfData []byte, nombre string) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("iniciando servidor visor PAdES: %w", err)
	}
	puerto := ln.Addr().(*net.TCPAddr).Port
	base := fmt.Sprintf("http://127.0.0.1:%d", puerto)
	token, err := nuevoTokenVisor()
	if err != nil {
		_ = ln.Close()
		return "", err
	}

	guardarURL := base + "/pades/guardar?token=" + token
	html, err := renderVisorSimple(pdfData, filepath.Base(nombre), guardarURL, s.loc)
	if err != nil {
		_ = ln.Close()
		return "", err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !metodoSoloLectura(w, r) {
			return
		}
		if !tokenValido(r, token) {
			http.Error(w, "token invalido", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method != http.MethodHead {
			_, _ = fmt.Fprint(w, html)
		}
	})
	mux.HandleFunc("/pades/guardar", func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		if !metodoSoloLectura(w, r) {
			return
		}
		if !tokenValido(r, token) {
			http.Error(w, "token invalido", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		// Sanitizar el nombre para evitar HTTP header injection via CR/LF
		// y para cumplir con RFC 6266 (Content-Disposition).
		nombreSeguro := sanitizarNombreFichero(filepath.Base(nombre))
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, nombreSeguro))
		if r.Method != http.MethodHead {
			_, _ = w.Write(pdfData)
		}
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	s.mu.Lock()
	s.activos[base] = srv
	s.mu.Unlock()

	go func() {
		_ = srv.Serve(ln)
	}()

	go func() {
		<-ctx.Done()
		_ = srv.Close()
		s.mu.Lock()
		delete(s.activos, base)
		s.mu.Unlock()
	}()

	return base + "/?token=" + token, nil
}

func nuevoTokenVisor() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generando token visor PAdES: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func tokenValido(r *http.Request, esperado string) bool {
	recibido := strings.TrimSpace(r.URL.Query().Get("token"))
	if recibido == "" || esperado == "" || len(recibido) != len(esperado) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(recibido), []byte(esperado)) == 1
}

func metodoSoloLectura(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	http.Error(w, "metodo no permitido", http.StatusMethodNotAllowed)
	return false
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, private, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; object-src data:; frame-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}

// sanitizarNombreFichero elimina caracteres que podrian inyectarse en cabeceras HTTP.
func sanitizarNombreFichero(nombre string) string {
	var sb strings.Builder
	for _, r := range nombre {
		// Eliminar CR, LF, NUL y cualquier caracter de control ASCII.
		// Eliminar tambien comillas dobles para no romper el valor del header.
		if r < 32 || r == 127 || r == '"' || r == '\\' {
			continue
		}
		sb.WriteRune(r)
	}
	resultado := sb.String()
	if resultado == "" {
		return "documento.pdf"
	}
	return resultado
}

// CerrarTodos cierra todos los servidores activos.
func (s *Servidor) CerrarTodos() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, srv := range s.activos {
		_ = srv.Close()
	}
	s.activos = make(map[string]*http.Server)
}
