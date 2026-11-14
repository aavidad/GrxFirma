// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"

	"grxfirma/internal/domain"
)

// politicaContada admite los orígenes de la lista y cuenta las preguntas.
type politicaContada struct {
	trustStub
	mu        sync.Mutex
	admitidos map[string]bool
	preguntas map[string]int
}

func nuevaPoliticaContada(admitidos ...string) *politicaContada {
	p := &politicaContada{admitidos: map[string]bool{}, preguntas: map[string]int{}}
	for _, o := range admitidos {
		p.admitidos[o] = true
	}
	return p
}

func (p *politicaContada) Evaluate(_ context.Context, origen string) (domain.TrustDecision, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.preguntas[origen]++
	if p.admitidos[origen] {
		return domain.TrustDecision{Origin: origen, Status: domain.TrustAllowed}, nil
	}
	return domain.TrustDecision{Origin: origen, Status: domain.TrustDenied}, nil
}

func uriSesionRemota(rt, st string) string {
	q := url.Values{}
	q.Set("fileid", "req-h03")
	q.Set("rtservlet", rt)
	q.Set("stservlet", st)
	q.Set("format", "CAdES")
	return "afirma://sign?" + q.Encode()
}

// TestParseSesionConLaInvarianteDelNucleo: H-03. El parser aplica ahora a la
// descarga y la subida la invariante común del núcleo y da el mismo resultado
// que antes, por el camino directo y por el socket: la subida a un host fuera
// de la política se rechaza, dos hosts de confianza siguen valiendo y cada
// origen se pregunta una sola vez.
func TestParseSesionConLaInvarianteDelNucleo(t *testing.T) {
	const rt, st = "https://a.example/afirma-signature-retriever/RetrieveService", "https://b.example/afirma-signature-storage/StorageService"
	for _, socket := range []bool{false, true} {
		parse := func(a *Adaptador, raw string) (Solicitud, error) {
			if socket {
				return a.ParseSocket(context.Background(), raw)
			}
			return a.Parse(context.Background(), raw)
		}

		_, err := parse(New(nuevaPoliticaContada("https://a.example")), uriSesionRemota(rt, st))
		if err == nil || !strings.Contains(err.Error(), "https://b.example") || !strings.Contains(err.Error(), "no esta autorizado") {
			t.Fatalf("socket=%t, subida fuera de la política: err = %v", socket, err)
		}

		politica := nuevaPoliticaContada("https://a.example", "https://b.example")
		solicitud, err := parse(New(politica), uriSesionRemota(rt, st))
		if err != nil {
			t.Fatalf("socket=%t, dos hosts de confianza: %v", socket, err)
		}
		if solicitud.Sesion.UploadEndpoint != st || solicitud.Sesion.RetrieveEndpoint != rt {
			t.Fatalf("socket=%t, sesión: %+v", socket, solicitud.Sesion)
		}
		for origen, n := range politica.preguntas {
			if n != 1 {
				t.Fatalf("socket=%t, %s preguntado %d veces", socket, origen, n)
			}
		}

		// Sin política el parser no decide (lo hace después el orquestador).
		if _, err := parse(New(nil), uriSesionRemota(rt, st)); err != nil {
			t.Fatalf("socket=%t, sin política: %v", socket, err)
		}
	}
}
