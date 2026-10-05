package sign

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"math"
)

const (
	signaturePlaceholderBaseLength = 1024
	// signedAttributesAllowance covers the signed attributes (content type,
	// signing time, message digest, signing-certificate-v2, policy and
	// commitment) and the SignerInfo/SignedData structure.
	signedAttributesAllowance = 1536
	// defaultSignatureValueLength is used when the key size is unknown: it
	// fits RSA-8192 and any ECDSA curve.
	defaultSignatureValueLength = 1024
	// Bound allocations derived from certificates and revocation responses.
	maxSignaturePlaceholderLength = 64 << 20
)

// estimatedSignatureValueLength returns an upper bound of the encoded
// signature value for the signer's key.
func estimatedSignatureValueLength(signer crypto.Signer, cert *x509.Certificate) int {
	var pub crypto.PublicKey
	if signer != nil {
		pub = signer.Public()
	} else if cert != nil {
		pub = cert.PublicKey
	}
	switch k := pub.(type) {
	case *rsa.PublicKey:
		if k.N != nil {
			return (k.N.BitLen()+7)/8 + 16
		}
	case *ecdsa.PublicKey:
		if k.Curve != nil {
			// DER SEQUENCE of two INTEGERs, each up to the curve size plus a
			// sign byte.
			return 2*((k.Curve.Params().BitSize+7)/8+3) + 8
		}
	}
	return defaultSignatureValueLength
}

func checkedAddUint32(base uint32, delta int) (uint32, error) {
	delta32, err := checkedUint32(int64(delta), "increment")
	if err != nil {
		return 0, err
	}
	if delta32 > math.MaxUint32-base {
		return 0, fmt.Errorf("uint32 overflow: %d + %d", base, delta)
	}
	return base + delta32, nil
}

func checkedUint32(value int64, name string) (uint32, error) {
	if value < 0 || value > math.MaxUint32 {
		return 0, fmt.Errorf("%s %d is outside the uint32 range", name, value)
	}
	return uint32(value), nil
}

func signatureEncodedLength(contentLength int) (uint32, error) {
	if contentLength < 0 {
		return 0, fmt.Errorf("negative signature content length: %d", contentLength)
	}
	if contentLength > maxSignaturePlaceholderLength/2 {
		return 0, fmt.Errorf("signature content exceeds the %d-byte encoded placeholder limit", maxSignaturePlaceholderLength)
	}
	return checkedUint32(int64(hex.EncodedLen(contentLength)), "encoded signature length")
}

func (context *SignContext) resetSignaturePlaceholderLength() error {
	if context.SignatureMaxLengthBase > maxSignaturePlaceholderLength {
		return fmt.Errorf("signature placeholder base exceeds the %d-byte limit", maxSignaturePlaceholderLength)
	}
	context.SignatureMaxLength = context.SignatureMaxLengthBase
	return nil
}

func (context *SignContext) addSignatureContentLength(contentLength int) error {
	encodedLength, err := signatureEncodedLength(contentLength)
	if err != nil {
		return err
	}
	return context.addSignaturePlaceholderLength(encodedLength)
}

func (context *SignContext) addSignaturePlaceholderLength(delta uint32) error {
	if context.SignatureMaxLength > maxSignaturePlaceholderLength {
		return fmt.Errorf("signature placeholder already exceeds the %d-byte limit", maxSignaturePlaceholderLength)
	}
	if delta > maxSignaturePlaceholderLength-context.SignatureMaxLength {
		return fmt.Errorf("signature placeholder exceeds the %d-byte limit", maxSignaturePlaceholderLength)
	}
	context.SignatureMaxLength += delta
	return nil
}

func (context *SignContext) growSignaturePlaceholderBase(required uint32) error {
	if required <= context.SignatureMaxLength {
		return nil
	}
	if required >= maxSignaturePlaceholderLength {
		return fmt.Errorf("encoded signature exceeds the %d-byte placeholder limit", maxSignaturePlaceholderLength)
	}
	if context.SignatureMaxLengthBase > maxSignaturePlaceholderLength {
		return fmt.Errorf("signature placeholder base exceeds the %d-byte limit", maxSignaturePlaceholderLength)
	}

	// Keep one byte of headroom because variable-length signatures may change
	// slightly when SignPDF retries with a freshly generated signature.
	growth := required - context.SignatureMaxLength + 1
	if growth > maxSignaturePlaceholderLength-context.SignatureMaxLengthBase {
		return fmt.Errorf("signature placeholder base exceeds the %d-byte limit", maxSignaturePlaceholderLength)
	}
	context.SignatureMaxLengthBase += growth
	return nil
}
