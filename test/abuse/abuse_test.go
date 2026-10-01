// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package abuse_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"testing"

	"grxfirma/internal/adapters/inbound/desktop/nativehost"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

func TestAfirmaURI_AbuseCases_NoPanic(t *testing.T) {
	adaptador := afirmauri.New(trustAllow{})

	cases := []struct {
		name string
		uri  string
	}{
		{"vacia", ""},
		{"solo_espacios", "   "},
		{"esquema_invalido", "https://example.invalid"},
		{"sin_operacion", "afirma://"},
		{"operacion_desconocida", "afirma://romper?id=req&stservlet=https%3A%2F%2Fstore.invalid"},
		{"sin_id", "afirma://sign?stservlet=https%3A%2F%2Fstore.invalid"},
		{"sin_servlets", "afirma://sign?id=req"},
		{"endpoint_malformado", "afirma://sign?id=req&stservlet=::::"},
		{"retrieve_malformado", "afirma://sign?id=req&rtservlet=::::"},
		{"base64_invalido", "afirma://sign?id=req&stservlet=https%3A%2F%2Fstore.invalid&dat=%25%25%25"},
		{"formato_invalido", "afirma://sign?id=req&stservlet=https%3A%2F%2Fstore.invalid&format=ROT13"},
		{"version_negativa", "afirma://sign?id=req&stservlet=https%3A%2F%2Fstore.invalid&v=-1"},
		{"version_texto", "afirma://sign?id=req&stservlet=https%3A%2F%2Fstore.invalid&v=nope"},
		{"doble_slash_raro", "afirma:////sign?id=req&stservlet=https%3A%2F%2Fstore.invalid"},
		{"batch_base64_invalido", "afirma://batch?id=req&stservlet=https%3A%2F%2Fstore.invalid&dat=%25bad"},
		{"batch_sin_datos_ni_retrieve", "afirma://batch?id=req&foo=bar"},
		{"selectcert_sin_parametros", "afirma://selectcert?id=req&stservlet=https%3A%2F%2Fstore.invalid"},
		{"fileid_vacio", "afirma://sign?fileId=&stservlet=https%3A%2F%2Fstore.invalid"},
		{"host_invalido_en_storage", "afirma://sign?id=req&stservlet=http://"},
		{"host_invalido_en_retrieve", "afirma://sign?id=req&rtservlet=http://"},
	}
	if len(cases) < 20 {
		t.Fatalf("casos insuficientes de afirmauri: %d", len(cases))
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertNoPanic(t, func() {
				_, _ = adaptador.Parse(context.Background(), tc.uri)
			})
		})
	}
}

func TestAfirmaURI_BatchPayload_AbuseCases_NoPanic(t *testing.T) {
	adaptador := afirmauri.New(trustAllow{})
	sesion := domain.ExchangeSession{
		RequestID:        "req-batch",
		UploadEndpoint:   "https://store.invalid/StorageService",
		RetrieveEndpoint: "https://retrieve.invalid/RetrieveService",
		State:            domain.SessionActive,
	}

	cases := []struct {
		name    string
		payload []byte
	}{
		{"json_vacio", []byte(`{}`)},
		{"json_roto", []byte(`{"format":`)},
		{"json_singlesigns_no_array", []byte(`{"format":"CAdES","singlesigns":{}}`)},
		{"json_datareference_remota", []byte(`{"format":"CAdES","singlesigns":[{"id":"1","datareference":"https://example.invalid/doc"}]}`)},
		{"json_datareference_invalida", []byte(`{"format":"CAdES","singlesigns":[{"id":"1","datareference":"%%%"}]}`)},
		{"xml_roto", []byte(`<signbatch><singlesign></signbatch>`)},
		{"xml_datasource_remota", []byte(`<signbatch><singlesign Id="1"><datasource>https://example.invalid/doc</datasource></singlesign></signbatch>`)},
		{"xml_datasource_data_uri_invalida", []byte(`<signbatch><singlesign Id="1"><datasource>data:text/plain;base64,%%%</datasource></singlesign></signbatch>`)},
		{"payload_binario", []byte{0x00, 0xff, 0x10, 0x20}},
		{"payload_xml_vacio", []byte(`<signbatch/>`)},
		{"payload_json_sin_id", []byte(`{"format":"CAdES","singlesigns":[{"datareference":"` + base64.StdEncoding.EncodeToString([]byte("A")) + `"}]}`)},
		{"payload_json_multipartes", []byte(`{"format":"XAdES","singlesigns":[{"id":"1","datareference":"` + base64.StdEncoding.EncodeToString([]byte("<x/>")) + `","extraparams":"broken=%"}]}`)},
	}
	if len(cases) < 10 {
		t.Fatalf("casos insuficientes de batch abuse: %d", len(cases))
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertNoPanic(t, func() {
				_, _ = adaptador.ParseBatchPayload(tc.payload, sesion)
			})
		})
	}
}

func TestNativeHost_AbuseCases_NoPanic(t *testing.T) {
	adaptador := nativehost.New(catalogAbuse{}, signAbuse{}, verifyAbuse{}, trustNative{allowed: true})

	validBase64 := base64.StdEncoding.EncodeToString([]byte("hola"))
	cases := []struct {
		name    string
		payload []byte
	}{
		{"json_vacio", []byte(`{}`)},
		{"json_roto", []byte(`{"action":`)},
		{"accion_desconocida", []byte(`{"requestId":"1","action":"romper"}`)},
		{"sign_sin_data", []byte(`{"requestId":"2","action":"sign"}`)},
		{"sign_base64_invalido", []byte(`{"requestId":"3","action":"sign","data":"%%%"}`)},
		{"verify_base64_invalido", []byte(`{"requestId":"4","action":"verify","signatureData":"%%%"}`)},
		{"verify_original_invalido", []byte(`{"requestId":"5","action":"verify","signatureData":"` + validBase64 + `","originalData":"%%%"}`)},
		{"requestid_objeto", []byte(`{"requestId":{"nested":1},"action":"ping"}`)},
		{"requestid_array", []byte(`{"requestId":[1,2],"action":"ping"}`)},
		{"getcertificates", []byte(`{"requestId":"6","action":"getCertificates"}`)},
		{"verify_sin_usecase", []byte(`{"requestId":"7","action":"verify","signatureData":"` + validBase64 + `"}`)},
		{"sign_sin_usecase", []byte(`{"requestId":"8","action":"sign","data":"` + validBase64 + `"}`)},
	}
	if len(cases) < 10 {
		t.Fatalf("casos insuficientes de nativehost abuse: %d", len(cases))
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertNoPanic(t, func() {
				_, _ = adaptador.Process(context.Background(), "caller", tc.payload)
			})
		})
	}
}

func TestNativeHost_FramingAbuse_NoPanic(t *testing.T) {
	cases := []struct {
		name string
		buf  []byte
	}{
		{"vacio", nil},
		{"solo_un_byte", []byte{0x01}},
		{"header_truncado", []byte{0x04, 0x00, 0x00}},
		{"length_sin_payload", []byte{0x04, 0x00, 0x00, 0x00}},
		{"payload_corto", append(u32le(10), []byte("abc")...)},
		{"payload_largo_truncado", append(u32le(1024), bytes.Repeat([]byte{'a'}, 16)...)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertNoPanic(t, func() {
				_, _ = nativehost.ReadMessage(bytes.NewReader(tc.buf))
			})
		})
	}
}

func TestAbuse_DevuelveErroresDescriptivos(t *testing.T) {
	uriAdapter := afirmauri.New(trustAllow{})
	if _, err := uriAdapter.Parse(context.Background(), "afirma://sign?foo=bar"); err == nil || err.Error() == "" {
		t.Fatal("se esperaba error descriptivo para URI con parámetros insuficientes")
	}

	batchSession := domain.ExchangeSession{
		RequestID:        "req",
		UploadEndpoint:   "https://store.invalid/StorageService",
		RetrieveEndpoint: "https://retrieve.invalid/RetrieveService",
		State:            domain.SessionActive,
	}
	if _, err := uriAdapter.ParseBatchPayload([]byte(`{"format":"CAdES","singlesigns":[{"id":"1","datareference":"https://example.invalid"}]}`), batchSession); err == nil || err.Error() == "" {
		t.Fatal("se esperaba error descriptivo para batch remoto no soportado")
	}

	native := nativehost.New(catalogAbuse{}, signAbuse{}, verifyAbuse{}, trustNative{allowed: true})
	responses, err := native.Process(context.Background(), "caller", []byte(`{"requestId":"1","action":"sign","data":"%%%"}`))
	if err != nil {
		t.Fatalf("Process no debería devolver error de infraestructura: %v", err)
	}
	if len(responses) == 0 || !bytes.Contains(responses[0], []byte(`"error"`)) {
		t.Fatalf("se esperaba respuesta con error descriptivo, obtenida: %q", responses)
	}
}

func FuzzAfirmaURIParse_NoPanic(f *testing.F) {
	seeds := []string{
		"afirma://sign?id=req&stservlet=https%3A%2F%2Fstore.invalid",
		"afirma://batch?id=req&dat=eyJmb3JtYXQiOiJDQWRFUyJ9",
		"%%%bad%%%",
		"",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	adaptador := afirmauri.New(trustAllow{})
	f.Fuzz(func(t *testing.T, raw string) {
		assertNoPanic(t, func() {
			_, _ = adaptador.Parse(context.Background(), raw)
		})
	})
}

func FuzzNativeReadMessage_NoPanic(f *testing.F) {
	f.Add([]byte{0x00, 0x00, 0x00, 0x00})
	f.Add(append(u32le(4), []byte(`{}`)...))
	f.Add([]byte("not-native"))
	f.Fuzz(func(t *testing.T, payload []byte) {
		assertNoPanic(t, func() {
			_, _ = nativehost.ReadMessage(bytes.NewReader(payload))
		})
	})
}

func assertNoPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic inesperado: %v", r)
		}
	}()
	fn()
}

func u32le(v uint32) []byte {
	var out [4]byte
	binary.LittleEndian.PutUint32(out[:], v)
	return out[:]
}

type trustAllow struct{}

func (trustAllow) Evaluate(context.Context, string) (domain.TrustDecision, error) {
	return domain.TrustDecision{Origin: "test", Status: domain.TrustAllowed}, nil
}
func (trustAllow) Allow(context.Context, string) error  { return nil }
func (trustAllow) Deny(context.Context, string) error   { return nil }
func (trustAllow) Remove(context.Context, string) error { return nil }

type trustNative struct{ allowed bool }

func (t trustNative) Evaluate(context.Context, string) (domain.TrustDecision, error) {
	if t.allowed {
		return domain.TrustDecision{Origin: "caller", Status: domain.TrustAllowed}, nil
	}
	return domain.TrustDecision{Origin: "caller", Status: domain.TrustDenied}, nil
}
func (trustNative) Allow(context.Context, string) error  { return nil }
func (trustNative) Deny(context.Context, string) error   { return nil }
func (trustNative) Remove(context.Context, string) error { return nil }

type catalogAbuse struct{}

func (catalogAbuse) List(context.Context) ([]domain.CertificateRef, error) {
	return nil, errors.New("catalogo no disponible")
}

type signAbuse struct{}

func (signAbuse) Execute(context.Context, application.SignCommand) (application.SignResult, error) {
	return application.SignResult{}, errors.New("firma no disponible")
}

type verifyAbuse struct{}

func (verifyAbuse) Execute(context.Context, application.VerifyCommand) (application.VerifyResult, error) {
	return application.VerifyResult{}, errors.New("verificacion no disponible")
}
