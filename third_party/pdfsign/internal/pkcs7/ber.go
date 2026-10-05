package pkcs7

import (
	"bytes"
	"encoding/binary"
	"errors"
)

type asn1Object interface {
	EncodeTo(writer *bytes.Buffer) error
}

type asn1Structured struct {
	tagBytes []byte
	content  []asn1Object
}

func (s asn1Structured) EncodeTo(out *bytes.Buffer) error {
	//fmt.Printf("%s--> tag: % X\n", strings.Repeat("| ", encodeIndent), s.tagBytes)
	inner := new(bytes.Buffer)
	for _, obj := range s.content {
		err := obj.EncodeTo(inner)
		if err != nil {
			return err
		}
	}
	out.Write(s.tagBytes)
	encodeLength(out, inner.Len())
	out.Write(inner.Bytes())
	return nil
}

type asn1Primitive struct {
	tagBytes []byte
	length   int
	content  []byte
}

func (p asn1Primitive) EncodeTo(out *bytes.Buffer) error {
	_, err := out.Write(p.tagBytes)
	if err != nil {
		return err
	}
	if err = encodeLength(out, p.length); err != nil {
		return err
	}
	//fmt.Printf("%s--> tag: % X length: %d\n", strings.Repeat("| ", encodeIndent), p.tagBytes, p.length)
	//fmt.Printf("%s--> content length: %d\n", strings.Repeat("| ", encodeIndent), len(p.content))
	out.Write(p.content)

	return nil
}

func ber2der(ber []byte) ([]byte, error) {
	if len(ber) == 0 {
		return nil, errors.New("ber2der: input ber is empty")
	}
	//fmt.Printf("--> ber2der: Transcoding %d bytes\n", len(ber))
	out := new(bytes.Buffer)

	obj, _, err := readObject(ber, 0, 0)
	if err != nil {
		return nil, err
	}
	obj.EncodeTo(out)

	return out.Bytes(), nil
}

// encodeLength writes a DER length. Short form for lengths below 128; long
// form otherwise (0x80 | number of octets, then the big-endian length).
// Lengths are bounds-checked explicitly instead of relying on narrowing
// integer conversions (gosec G115).
func encodeLength(out *bytes.Buffer, length int) error {
	if length < 0 {
		return errors.New("ber2der: negative length")
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(length))
	if length < 128 {
		return out.WriteByte(buf[7])
	}
	first := 0
	for first < len(buf)-1 && buf[first] == 0 {
		first++
	}
	octets := buf[first:]
	if err := out.WriteByte(0x80 | lengthOctetsHeader[len(octets)]); err != nil {
		return err
	}
	_, err := out.Write(octets)
	return err
}

// lengthOctetsHeader maps the number of length octets (1..8) to its byte value.
var lengthOctetsHeader = [9]byte{0, 1, 2, 3, 4, 5, 6, 7, 8}

// maxAnidamientoBER limita los niveles de objetos construidos. AutoFirmaV2:
// la firma de un PDF que se verifica no es de confianza y una cadena de
// SEQUENCE anidadas agotaba la pila.
const maxAnidamientoBER = 128

func readObject(ber []byte, offset, nivel int) (asn1Object, int, error) {
	if nivel > maxAnidamientoBER {
		return nil, 0, errors.New("ber2der: anidamiento excesivo")
	}
	berLen := len(ber)
	if offset >= berLen {
		return nil, 0, errors.New("ber2der: offset is after end of ber data")
	}
	tagStart := offset
	b := ber[offset]
	offset++
	if offset >= berLen {
		return nil, 0, errors.New("ber2der: cannot move offset forward, end of ber data reached")
	}
	tag := b & 0x1F // last 5 bits
	if tag == 0x1F {
		tag = 0
		for ber[offset] >= 0x80 {
			tag = tag*128 + ber[offset] - 0x80
			offset++
			if offset >= berLen {
				return nil, 0, errors.New("ber2der: cannot move offset forward, end of ber data reached")
			}
		}
		// jvehent 20170227: this doesn't appear to be used anywhere...
		//tag = tag*128 + ber[offset] - 0x80
		offset++
		if offset >= berLen {
			return nil, 0, errors.New("ber2der: cannot move offset forward, end of ber data reached")
		}
	}
	tagEnd := offset

	kind := b & 0x20
	if kind == 0 {
		debugprint("--> Primitive\n")
	} else {
		debugprint("--> Constructed\n")
	}
	// read length
	var length int
	l := ber[offset]
	offset++
	if l >= 0x80 && offset >= berLen {
		// if indefinite or multibyte length, we need to verify there is at least one more byte available
		// otherwise we need to be flexible here for length == 0 conditions
		// validation that the length is available is done after the length is correctly parsed
		return nil, 0, errors.New("ber2der: cannot move offset forward, end of ber data reached")
	}
	indefinite := false
	if l > 0x80 {
		numberOfBytes := (int)(l & 0x7F)
		if numberOfBytes > 4 { // int is only guaranteed to be 32bit
			return nil, 0, errors.New("ber2der: BER tag length too long")
		}
		if numberOfBytes == 4 && (int)(ber[offset]) > 0x7F {
			return nil, 0, errors.New("ber2der: BER tag length is negative")
		}
		if offset+numberOfBytes > berLen {
			// == condition is not checked here, this allows for a more descreptive error when the parsed length is
			// compared with the remaining available bytes (`contentEnd > berLen`)
			return nil, 0, errors.New("ber2der: cannot move offset forward, end of ber data reached")
		}
		if (int)(ber[offset]) == 0x0 && (numberOfBytes == 1 || ber[offset+1] <= 0x7F) {
			// `numberOfBytes == 1` is an important conditional to avoid a potential out of bounds panic with `ber[offset+1]`
			return nil, 0, errors.New("ber2der: BER tag length has leading zero")
		}
		debugprint("--> (compute length) indicator byte: %x\n", l)
		//debugprint("--> (compute length) length bytes: %x\n", ber[offset:offset+numberOfBytes])
		for i := 0; i < numberOfBytes; i++ {
			length = length*256 + (int)(ber[offset])
			offset++
		}
	} else if l == 0x80 {
		indefinite = true
	} else {
		length = (int)(l)
	}
	if length < 0 {
		return nil, 0, errors.New("ber2der: invalid negative value found in BER tag length")
	}
	//fmt.Printf("--> length        : %d\n", length)
	contentEnd := offset + length
	if contentEnd > berLen {
		return nil, 0, errors.New("ber2der: BER tag length is more than available data")
	}
	debugprint("--> content start : %d\n", offset)
	debugprint("--> content end   : %d\n", contentEnd)
	//debugprint("--> content       : %x\n", ber[offset:contentEnd])
	var obj asn1Object
	if indefinite && kind == 0 {
		return nil, 0, errors.New("ber2der: Indefinite form tag must have constructed encoding")
	}
	if kind == 0 {
		obj = asn1Primitive{
			tagBytes: ber[tagStart:tagEnd],
			length:   length,
			content:  ber[offset:contentEnd],
		}
	} else {
		var subObjects []asn1Object
		for (offset < contentEnd) || indefinite {
			var subObj asn1Object
			var err error
			subObj, offset, err = readObject(ber, offset, nivel+1)
			if err != nil {
				return nil, 0, err
			}
			subObjects = append(subObjects, subObj)

			if indefinite {
				terminated, err := isIndefiniteTermination(ber, offset)
				if err != nil {
					return nil, 0, err
				}

				if terminated {
					break
				}
			}
		}
		obj = asn1Structured{
			tagBytes: ber[tagStart:tagEnd],
			content:  subObjects,
		}
	}

	// Apply indefinite form length with 0x0000 terminator.
	if indefinite {
		contentEnd = offset + 2
	}

	return obj, contentEnd, nil
}

func isIndefiniteTermination(ber []byte, offset int) (bool, error) {
	if len(ber)-offset < 2 {
		return false, errors.New("ber2der: Invalid BER format")
	}

	return bytes.Index(ber[offset:], []byte{0x0, 0x0}) == 0, nil
}

func debugprint(format string, a ...interface{}) {
	//fmt.Printf(format, a)
}
