package pkcs7

import (
	"bytes"
	"encoding/asn1"
	"testing"
)

// encodeLength must produce the same DER length octets as encoding/asn1.
func TestEncodeLengthMatchesEncodingASN1(t *testing.T) {
	for _, n := range []int{0, 1, 127, 128, 255, 256, 65535, 65536, 1 << 24} {
		der, err := asn1.Marshal(make([]byte, n))
		if err != nil {
			t.Fatal(err)
		}
		want := der[1 : len(der)-n]
		var got bytes.Buffer
		if err := encodeLength(&got, n); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("length %d: got % x, want % x", n, got.Bytes(), want)
		}
	}
	if err := encodeLength(&bytes.Buffer{}, -1); err == nil {
		t.Fatal("negative length must be rejected")
	}
}
