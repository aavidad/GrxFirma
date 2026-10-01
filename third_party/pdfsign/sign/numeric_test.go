package sign

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestCheckedAddUint32Boundaries(t *testing.T) {
	tests := []struct {
		name    string
		base    uint32
		delta   int
		want    uint32
		wantErr bool
	}{
		{name: "zero", base: 0, delta: 0, want: 0},
		{name: "maximum", base: math.MaxUint32 - 1, delta: 1, want: math.MaxUint32},
		{name: "negative", base: 1, delta: -1, wantErr: true},
		{name: "overflow", base: math.MaxUint32, delta: 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkedAddUint32(tt.base, tt.delta)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkedAddUint32() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("checkedAddUint32() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCheckedUint32Boundaries(t *testing.T) {
	for _, value := range []int64{0, math.MaxUint32} {
		got, err := checkedUint32(value, "test value")
		if err != nil {
			t.Fatalf("checkedUint32(%d) returned error: %v", value, err)
		}
		if int64(got) != value {
			t.Fatalf("checkedUint32(%d) = %d", value, got)
		}
	}

	for _, value := range []int64{-1, math.MaxUint32 + 1} {
		if _, err := checkedUint32(value, "test value"); err == nil {
			t.Fatalf("checkedUint32(%d) did not reject an out-of-range value", value)
		}
	}
}

func TestSignatureEncodedLengthBoundaries(t *testing.T) {
	got, err := signatureEncodedLength(maxSignaturePlaceholderLength / 2)
	if err != nil {
		t.Fatalf("maximum accepted signature length returned error: %v", err)
	}
	if got != maxSignaturePlaceholderLength {
		t.Fatalf("encoded length = %d, want %d", got, maxSignaturePlaceholderLength)
	}

	for _, value := range []int{-1, maxSignaturePlaceholderLength/2 + 1} {
		if _, err := signatureEncodedLength(value); err == nil {
			t.Fatalf("signatureEncodedLength(%d) did not reject an invalid length", value)
		}
	}
}

func TestSignaturePlaceholderGrowthIsBounded(t *testing.T) {
	context := &SignContext{
		SignatureMaxLength:     maxSignaturePlaceholderLength - 1,
		SignatureMaxLengthBase: signaturePlaceholderBaseLength,
	}
	if err := context.addSignaturePlaceholderLength(1); err != nil {
		t.Fatalf("adding up to the limit returned error: %v", err)
	}
	if err := context.addSignaturePlaceholderLength(1); err == nil {
		t.Fatal("adding beyond the placeholder limit did not return an error")
	}

	context.SignatureMaxLength = signaturePlaceholderBaseLength
	if err := context.growSignaturePlaceholderBase(maxSignaturePlaceholderLength); err == nil {
		t.Fatal("growing the base to the hard limit did not reserve retry headroom")
	}
}

func TestGeneratedEnumStringsPreserveUnsignedValues(t *testing.T) {
	maxUint := ^uint(0)
	wantCertType := "CertType(" + strconv.FormatUint(uint64(maxUint), 10) + ")"
	wantDocMDPPerm := "DocMDPPerm(" + strconv.FormatUint(uint64(maxUint), 10) + ")"

	if got := CertType(maxUint).String(); got != wantCertType {
		t.Fatalf("CertType maximum string = %q, want %q", got, wantCertType)
	}
	if got := DocMDPPerm(maxUint).String(); got != wantDocMDPPerm {
		t.Fatalf("DocMDPPerm maximum string = %q, want %q", got, wantDocMDPPerm)
	}
	if !strings.HasSuffix(CertType(0).String(), "(0)") || !strings.HasSuffix(DocMDPPerm(0).String(), "(0)") {
		t.Fatal("zero-valued generated enum strings changed unexpectedly")
	}
}
