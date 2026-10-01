// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package hashmanifest

import (
	"bytes"
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestCodec_EncodeManifestByteExactoEstable(t *testing.T) {
	t.Parallel()

	digest := make([]byte, 32)
	digest[0] = 0xff
	manifest := domain.DirectoryHashManifest{
		Algorithm: "SHA-256",
		Recursive: true,
		Entries: []domain.DirectoryHashEntry{{
			RelativePath: `carpeta/a & "q".txt`,
			Digest:       digest,
		}},
	}
	codec := NuevoCodec()
	tests := []struct {
		name   string
		format domain.DirectoryHashManifestFormat
		want   string
	}{
		{
			name:   "xml",
			format: domain.DirectoryHashFormatXML,
			want: "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
				"<entries hashAlgorithm=\"SHA-256\" recursive=\"true\">\n" +
				"  <entry name=\"carpeta/a &amp; &#34;q&#34;.txt\" hash=\"/wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\" hexhash=\"ff00000000000000000000000000000000000000000000000000000000000000h\"></entry>\n" +
				"</entries>",
		},
		{
			name:   "txt",
			format: domain.DirectoryHashFormatTXT,
			want: ";charset=UTF-8\r\n;hashAlgorithm=SHA-256\r\n;recursive=true\r\n" +
				"carpeta/a & \"q\".txt;ff00000000000000000000000000000000000000000000000000000000000000\r\n",
		},
		{
			name:   "csv",
			format: domain.DirectoryHashFormatCSV,
			want:   "\"carpeta/a & \"\"q\"\".txt\",\"ff00000000000000000000000000000000000000000000000000000000000000h\"\r\n",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := codec.EncodeManifest(context.Background(), manifest, test.format)
			if err != nil {
				t.Fatalf("EncodeManifest() error = %v", err)
			}
			if string(got) != test.want {
				t.Fatalf("salida no estable\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

func TestCodec_DecodeFixtureXMLRealV1_9(t *testing.T) {
	t.Parallel()

	// Generado con XmlHashDocument de AutoFirma 1.9 sobre OpenJDK 8. El
	// serializador V1 usa Base64 URL-safe y hexadecimal en mayusculas.
	const v1Fixture = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"no\"?>\n" +
		"<entries hashAlgorithm=\"SHA-256\" recursive=\"true\">\n" +
		"<entry hash=\"_wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\" hexhash=\"FF00000000000000000000000000000000000000000000000000000000000000h\" name=\"carpeta/a &amp; &quot;q&quot;.txt\"/>\n" +
		"</entries>\n"

	got, err := NuevoCodec().DecodeManifest(
		context.Background(),
		[]byte(v1Fixture),
		"directorio.hashfiles",
	)
	if err != nil {
		t.Fatalf("DecodeManifest(fixture V1.9) error = %v", err)
	}
	wantDigest := make([]byte, 32)
	wantDigest[0] = 0xff
	if got.Algorithm != "SHA-256" || !got.Recursive || len(got.Entries) != 1 {
		t.Fatalf("manifest V1.9 inesperado: %#v", got)
	}
	if got.Entries[0].RelativePath != `carpeta/a & "q".txt` ||
		!bytes.Equal(got.Entries[0].Digest, wantDigest) {
		t.Fatalf("entrada V1.9 inesperada: %#v", got.Entries[0])
	}
}

func TestCodec_DecodeFixtureTXTRealV1_9(t *testing.T) {
	t.Parallel()

	// Generado con TxtHashDocument de AutoFirma 1.9 sobre OpenJDK 8.
	const v1Fixture = ";charset=UTF-8\r\n" +
		";hashAlgorithm=SHA-256\r\n" +
		";recursive=true\r\n" +
		"carpeta/a.txt;BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD\r\n"

	got, err := NuevoCodec().DecodeManifest(
		context.Background(),
		[]byte(v1Fixture),
		"directorio.txthashfiles",
	)
	if err != nil {
		t.Fatalf("DecodeManifest(fixture TXT V1.9) error = %v", err)
	}
	if got.Algorithm != "SHA-256" || !got.Recursive || len(got.Entries) != 1 {
		t.Fatalf("manifest TXT V1.9 inesperado: %#v", got)
	}
	if got.Entries[0].RelativePath != "carpeta/a.txt" {
		t.Fatalf("ruta TXT V1.9 inesperada: %#v", got.Entries[0])
	}
	const wantHex = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if gotHex := hex.EncodeToString(got.Entries[0].Digest); gotHex != wantHex {
		t.Fatalf("digest TXT V1.9 = %s, want %s", gotHex, wantHex)
	}
}

func TestCodec_XMLRoundTrip(t *testing.T) {
	codec := NuevoCodec()
	manifest := domain.DirectoryHashManifest{
		Algorithm: "SHA-256",
		Recursive: true,
		Entries: []domain.DirectoryHashEntry{
			{RelativePath: "a.txt", Digest: []byte{0x01, 0x02}},
		},
	}
	data, err := codec.EncodeManifest(context.Background(), manifest, domain.DirectoryHashFormatXML)
	if err != nil {
		t.Fatalf("EncodeManifest() error = %v", err)
	}
	got, err := codec.DecodeManifest(context.Background(), data, "directorio.hashfiles")
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if got.Algorithm != manifest.Algorithm || !got.Recursive || len(got.Entries) != 1 || got.Entries[0].RelativePath != "a.txt" {
		t.Fatalf("Manifest inesperado: %#v", got)
	}
}

func TestCodec_TXTRoundTrip(t *testing.T) {
	codec := NuevoCodec()
	manifest := domain.DirectoryHashManifest{
		Algorithm: "SHA-512",
		Recursive: false,
		Entries: []domain.DirectoryHashEntry{
			{RelativePath: "b.txt", Digest: []byte{0xaa, 0xbb}},
		},
	}
	data, err := codec.EncodeManifest(context.Background(), manifest, domain.DirectoryHashFormatTXT)
	if err != nil {
		t.Fatalf("EncodeManifest() error = %v", err)
	}
	got, err := codec.DecodeManifest(context.Background(), data, "directorio.txthashfiles")
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if got.Algorithm != manifest.Algorithm || got.Recursive || len(got.Entries) != 1 || got.Entries[0].RelativePath != "b.txt" {
		t.Fatalf("Manifest inesperado: %#v", got)
	}
}

func TestCodec_XMLDecodeNormalizaSeparadoresLegacy(t *testing.T) {
	codec := NuevoCodec()
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<entries hashAlgorithm="SHA-256" recursive="true">
  <entry name="carpeta\sub\a.txt" hash="AQI=" hexhash="0102h"></entry>
</entries>`)
	got, err := codec.DecodeManifest(context.Background(), data, "directorio.hashfiles")
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].RelativePath != "carpeta/sub/a.txt" {
		t.Fatalf("ruta normalizada inesperada: %#v", got.Entries)
	}
}

func TestCodec_TXTDecodeNormalizaSeparadoresLegacy(t *testing.T) {
	codec := NuevoCodec()
	data := []byte(";charset=UTF-8\r\n;hashAlgorithm=SHA-256\r\n;recursive=true\r\ncarpeta\\sub\\a.txt;0102\r\n")
	got, err := codec.DecodeManifest(context.Background(), data, "directorio.txthashfiles")
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].RelativePath != "carpeta/sub/a.txt" {
		t.Fatalf("ruta normalizada inesperada: %#v", got.Entries)
	}
}

func TestCodec_DecodeRechazaRutasInsegurasODuplicadas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hint string
		data string
	}{
		{
			name: "xml traversal",
			hint: "directorio.hashfiles",
			data: `<entries hashAlgorithm="SHA-256"><entry name="../secreto" hash="AQI=" hexhash="0102h"></entry></entries>`,
		},
		{
			name: "xml absoluta",
			hint: "directorio.hashfiles",
			data: `<entries hashAlgorithm="SHA-256"><entry name="/etc/passwd" hash="AQI=" hexhash="0102h"></entry></entries>`,
		},
		{
			name: "txt traversal normalizado",
			hint: "directorio.txthashfiles",
			data: ";hashAlgorithm=SHA-256\r\na/../b;0102\r\n",
		},
		{
			name: "txt duplicada",
			hint: "directorio.txthashfiles",
			data: ";hashAlgorithm=SHA-256\r\nb;0102\r\nb;0304\r\n",
		},
		{
			name: "txt volumen windows",
			hint: "directorio.txthashfiles",
			data: ";hashAlgorithm=SHA-256\r\nC:\\secreto.txt;0102\r\n",
		},
	}

	codec := NuevoCodec()
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := codec.DecodeManifest(context.Background(), []byte(test.data), test.hint); err == nil {
				t.Fatal("DecodeManifest() no rechazo el manifiesto inseguro")
			}
		})
	}
}

func TestCodec_EncodeReport(t *testing.T) {
	codec := NuevoCodec()
	report := domain.DirectoryHashCheckReport{
		Algorithm:       "SHA-256",
		Recursive:       true,
		MatchingHash:    []string{"uno.txt"},
		NotMatchingHash: []string{"dos.txt"},
		HashWithoutFile: []string{"tres.txt"},
		FileWithoutHash: []string{"cuatro.txt"},
	}

	data, err := codec.EncodeReport(context.Background(), report)
	if err != nil {
		t.Fatalf("EncodeReport() error = %v", err)
	}
	text := string(data)
	for _, needle := range []string{
		`<entries hashAlgorithm="SHA-256" recursive="true">`,
		`<matching_hash>`,
		`<entry name="uno.txt"></entry>`,
		`<not_matching_hash>`,
		`<entry name="dos.txt"></entry>`,
		`<hash_without_file>`,
		`<entry name="tres.txt"></entry>`,
		`<file_without_hash>`,
		`<entry name="cuatro.txt"></entry>`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("informe XML sin %q:\n%s", needle, text)
		}
	}
}

func TestCodec_EncodeReportByteExactoEstable(t *testing.T) {
	t.Parallel()

	report := domain.DirectoryHashCheckReport{
		Algorithm:       "SHA-256",
		Recursive:       true,
		MatchingHash:    []string{"uno.txt"},
		NotMatchingHash: []string{"dos.txt"},
		HashWithoutFile: []string{"tres.txt"},
		FileWithoutHash: []string{"cuatro.txt"},
	}
	got, err := NuevoCodec().EncodeReport(context.Background(), report)
	if err != nil {
		t.Fatalf("EncodeReport() error = %v", err)
	}
	const want = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<entries hashAlgorithm=\"SHA-256\" recursive=\"true\">\n" +
		"  <matching_hash>\n" +
		"    <entry name=\"uno.txt\"></entry>\n" +
		"  </matching_hash>\n" +
		"  <not_matching_hash>\n" +
		"    <entry name=\"dos.txt\"></entry>\n" +
		"  </not_matching_hash>\n" +
		"  <hash_without_file>\n" +
		"    <entry name=\"tres.txt\"></entry>\n" +
		"  </hash_without_file>\n" +
		"  <file_without_hash>\n" +
		"    <entry name=\"cuatro.txt\"></entry>\n" +
		"  </file_without_hash>\n" +
		"</entries>"
	if string(got) != want {
		t.Fatalf("informe no estable\n got: %q\nwant: %q", got, want)
	}
}

func TestCodec_LimitesYContexto(t *testing.T) {
	t.Parallel()

	codec := NuevoCodec()
	if _, err := codec.DecodeManifest(
		context.Background(),
		make([]byte, maxDirectoryHashDocumentBytes+1),
		"directorio.hashfiles",
	); err == nil {
		t.Fatal("DecodeManifest() acepto un manifiesto sobredimensionado")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := codec.DecodeManifest(cancelled, []byte("<entries/>"), "directorio.hashfiles"); err == nil {
		t.Fatal("DecodeManifest() ignoro la cancelacion")
	}

	unsafeReport := domain.DirectoryHashCheckReport{
		Algorithm:    "SHA-256",
		MatchingHash: []string{"../fuera.txt"},
	}
	if _, err := codec.EncodeReport(context.Background(), unsafeReport); err == nil {
		t.Fatal("EncodeReport() acepto traversal")
	}
}

func TestCodec_DecodeRechazaBase64NoCanonico(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{
		`<entry name="a.txt" hash="_wA" hexhash="ff00h"></entry>`,
		`<entry name="a.txt" hash=" /wA=" hexhash="ff00h"></entry>`,
		`<entry name="a.txt" hash="/wA=" hexhash=" ff00h"></entry>`,
	} {
		data := []byte(`<entries hashAlgorithm="SHA-256" recursive="false">` +
			entry +
			`</entries>`)
		if _, err := NuevoCodec().DecodeManifest(context.Background(), data, "directorio.hashfiles"); err == nil {
			t.Fatalf("DecodeManifest() acepto un hash no canonico: %s", entry)
		}
	}
}

func TestCodec_DecodeTXTRechazaEspaciosAmbiguos(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{
		" a.txt;ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"a.txt ;ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"a.txt; ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	} {
		data := []byte(";hashAlgorithm=SHA-256\n" + entry + "\n")
		if _, err := NuevoCodec().DecodeManifest(context.Background(), data, "directorio.txthashfiles"); err == nil {
			t.Fatalf("DecodeManifest() acepto una linea ambigua: %q", entry)
		}
	}
}

func TestCodec_CSVPermaneceSoloComoSalida(t *testing.T) {
	t.Parallel()

	const csv = `"a.txt","ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015adh"` + "\r\n"
	if _, err := NuevoCodec().DecodeManifest(context.Background(), []byte(csv), "directorio.csv"); err == nil {
		t.Fatal("DecodeManifest() trato CSV como un formato verificable")
	}
}
