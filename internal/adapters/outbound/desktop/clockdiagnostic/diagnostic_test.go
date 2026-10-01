// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package clockdiagnostic

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type originSourceStub struct {
	origin AuthorizedObservedOrigin
	ok     bool
}

func (s originSourceStub) LastAuthorizedObservedHTTPSOrigin(
	context.Context,
) (AuthorizedObservedOrigin, bool) {
	return s.origin, s.ok
}

type httpDoerStub struct {
	calls   int
	request *http.Request
	date    string
	err     error
}

func (s *httpDoerStub) Do(request *http.Request) (*http.Response, error) {
	s.calls++
	s.request = request
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Header: http.Header{
			"Date": []string{s.date},
		},
		Body: io.NopCloser(strings.NewReader("respuesta que no debe leerse")),
	}, nil
}

func TestDiagnoseWithoutAuthorizedObservedOriginDoesNotUseNetwork(
	t *testing.T,
) {
	client := &httpDoerStub{}
	service := &Service{
		client: client,
		now:    time.Now,
		local: func() localSnapshot {
			return localSnapshot{
				observedAtUTC:   time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC),
				timeService:     "running",
				serviceObserved: true,
			}
		},
	}

	report := service.Diagnose(context.Background())

	if client.calls != 0 {
		t.Fatalf("se realizaron %d peticiones sin origen autorizado", client.calls)
	}
	if report.ThresholdSeconds != 5 || len(report.Steps) != 3 {
		t.Fatalf("informe inesperado: %#v", report)
	}
	if report.Steps[0].Status != statusUnknown ||
		report.Steps[0].EvidenceRef != evidenceLocalClock ||
		!strings.Contains(report.Steps[0].UserMessage, "no certifica") {
		t.Fatalf("fase local no honesta: %#v", report.Steps[0])
	}
	if report.Steps[1].Status != statusUnknown ||
		report.Steps[1].EvidenceRef != "" ||
		report.Steps[2].Status != statusUnknown {
		t.Fatalf("fases remotas no comprobadas inesperadas: %#v", report.Steps)
	}
}

func TestDefaultServiceInspectsLocalClockWithoutNetwork(t *testing.T) {
	report := New(nil).Diagnose(context.Background())

	if len(report.Steps) != 3 ||
		report.Steps[0].Code != localClockCode ||
		report.Steps[0].EvidenceRef != evidenceLocalClock ||
		report.Steps[1].Status != statusUnknown ||
		report.Steps[2].Status != statusUnknown {
		t.Fatalf("diagnóstico local predeterminado inesperado: %#v", report)
	}
}

func TestDiagnoseCompensatesRoundTripLatencyUsingMidpoint(t *testing.T) {
	started := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Second)
	times := []time.Time{started, finished}
	client := &httpDoerStub{date: started.Add(time.Second).Format(http.TimeFormat)}
	service := &Service{
		origins: originSourceStub{
			origin: AuthorizedObservedOrigin{
				URL:         "https://sede.example/",
				EvidenceRef: "observed-origin:request-42",
			},
			ok: true,
		},
		client: client,
		now: func() time.Time {
			result := times[0]
			times = times[1:]
			return result
		},
		local: func() localSnapshot {
			return localSnapshot{observedAtUTC: started}
		},
	}

	report := service.Diagnose(context.Background())

	if report.Steps[0].Status != statusSuccess ||
		report.Steps[1].Status != statusSuccess {
		t.Fatalf("el punto medio no compensó la latencia: %#v", report.Steps)
	}
	if client.calls != 1 || client.request.Method != http.MethodHead {
		t.Fatalf("sonda HTTP inesperada: calls=%d request=%#v", client.calls, client.request)
	}
	for _, header := range []string{
		"Authorization",
		"Cookie",
		"Origin",
		"Referer",
	} {
		if value := client.request.Header.Get(header); value != "" {
			t.Fatalf("cabecera sensible %s=%q", header, value)
		}
	}
	if report.Steps[2].Status != statusUnknown {
		t.Fatalf("Date remoto no debe probar @firma: %#v", report.Steps[2])
	}
}

func TestDiagnoseReportsSkewWithoutAttributingEitherClock(t *testing.T) {
	started := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	times := []time.Time{started, started.Add(200 * time.Millisecond)}
	client := &httpDoerStub{
		date: started.Add(10 * time.Minute).Format(http.TimeFormat),
	}
	service := &Service{
		origins: originSourceStub{
			origin: AuthorizedObservedOrigin{
				URL:         "https://sede.example",
				EvidenceRef: "observed-origin:request-42",
			},
			ok: true,
		},
		client: client,
		now: func() time.Time {
			result := times[0]
			times = times[1:]
			return result
		},
		local: func() localSnapshot {
			return localSnapshot{observedAtUTC: started}
		},
	}

	report := service.Diagnose(context.Background())

	for _, index := range []int{0, 1} {
		step := report.Steps[index]
		if step.Status != statusFailure ||
			step.Owner != ownerUnknown ||
			!strings.Contains(step.UserMessage, "no demuestra") {
			t.Fatalf("desfase atribuido indebidamente: %#v", step)
		}
	}
}

func TestDiagnoseDoesNotClaimDateEvidenceWhenHeaderIsInvalid(t *testing.T) {
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	times := []time.Time{now, now.Add(100 * time.Millisecond)}
	client := &httpDoerStub{date: "fecha-invalida"}
	service := &Service{
		origins: originSourceStub{
			origin: AuthorizedObservedOrigin{
				URL:         "https://sede.example",
				EvidenceRef: "observed-origin:request-42",
			},
			ok: true,
		},
		client: client,
		now: func() time.Time {
			result := times[0]
			times = times[1:]
			return result
		},
		local: func() localSnapshot {
			return localSnapshot{observedAtUTC: now}
		},
	}

	report := service.Diagnose(context.Background())

	if report.Steps[0].Status != statusUnknown ||
		report.Steps[1].Status != statusFailure ||
		report.Steps[1].Owner != ownerUnknown ||
		report.Steps[1].EvidenceRef != evidenceHTTPProbe {
		t.Fatalf("evidencia Date inválida atribuida: %#v", report.Steps)
	}
}

func TestDiagnoseRejectsUnsafeOrNonOriginURLsBeforeNetwork(t *testing.T) {
	for _, raw := range []string{
		"http://sede.example",
		"https://usuario:clave@sede.example",
		"https://sede.example/ruta",
		"https://sede.example/?token=secreto",
		"https://sede.example?",
		"https://sede.example:8443",
		"https://sede.example:invalido",
		"https://127.0.0.1",
		"https://172.16.1.20",
		"https://100.64.0.1",
		"https://198.18.0.1",
		"https://[::1]",
		"https://[2001:db8::1]",
	} {
		t.Run(raw, func(t *testing.T) {
			client := &httpDoerStub{}
			service := &Service{
				origins: originSourceStub{
					origin: AuthorizedObservedOrigin{
						URL:         raw,
						EvidenceRef: "observed-origin:request-42",
					},
					ok: true,
				},
				client: client,
				now:    time.Now,
				local: func() localSnapshot {
					return localSnapshot{observedAtUTC: time.Now().UTC()}
				},
			}

			report := service.Diagnose(context.Background())

			if client.calls != 0 {
				t.Fatalf("la URL rechazada inició %d peticiones", client.calls)
			}
			if report.Steps[1].Status != statusUnknown ||
				report.Steps[1].EvidenceRef != "" {
				t.Fatalf("resultado de URL rechazada inesperado: %#v", report.Steps[1])
			}
		})
	}
}

func TestDiagnoseRequiresObservedAuthorizationEvidence(t *testing.T) {
	client := &httpDoerStub{}
	service := &Service{
		origins: originSourceStub{
			origin: AuthorizedObservedOrigin{
				URL:         "https://sede.example",
				EvidenceRef: "token con espacios",
			},
			ok: true,
		},
		client: client,
		now:    time.Now,
		local: func() localSnapshot {
			return localSnapshot{observedAtUTC: time.Now().UTC()}
		},
	}

	report := service.Diagnose(context.Background())

	if client.calls != 0 {
		t.Fatalf("un origen sin evidencia inició %d peticiones", client.calls)
	}
	if report.Steps[1].Status != statusUnknown ||
		!strings.Contains(
			report.Steps[1].UserMessage,
			"observado y autorizado",
		) {
		t.Fatalf("evidencia inválida no rechazada: %#v", report.Steps[1])
	}
}
