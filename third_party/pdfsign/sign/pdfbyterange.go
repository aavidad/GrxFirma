package sign

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

func (context *SignContext) updateByteRange() error {
	if _, err := context.OutputBuffer.Seek(0, 0); err != nil {
		return err
	}

	// Los placeholders de esta firma están siempre en la sección incremental,
	// después del contenido original. Es imprescindible acotar la búsqueda a esa
	// región: un PDF de entrada ya firmado contiene su propio relleno de ceros
	// (el hueco /Contents de la firma previa) y una búsqueda global lo
	// encontraría primero, corrompiendo la firma existente al escribir dentro
	// de ella.
	searchStart := int64(0)
	if size, err := context.InputFile.Seek(0, io.SeekEnd); err == nil && size > 0 && size < int64(context.OutputBuffer.Buff.Len()) {
		searchStart = size
	}

	// Set ByteRangeValues by looking for the /Contents< filled with zeros
	contentsPlaceholder := bytes.Repeat([]byte("0"), int(context.SignatureMaxLength))
	contentsIndex := bytes.Index(context.OutputBuffer.Buff.Bytes()[searchStart:], contentsPlaceholder)
	if contentsIndex == -1 {
		return fmt.Errorf("failed to find contents placeholder")
	}
	contentsIndex += int(searchStart)

	// Calculate ByteRangeValues
	signatureContentsStart := int64(contentsIndex) - 1
	signatureContentsEnd := signatureContentsStart + int64(context.SignatureMaxLength) + 2
	context.ByteRangeValues = []int64{
		0,
		signatureContentsStart,
		signatureContentsEnd,
		int64(context.OutputBuffer.Buff.Len()) - signatureContentsEnd,
	}

	new_byte_range := fmt.Sprintf("/ByteRange [%d %d %d %d]", context.ByteRangeValues[0], context.ByteRangeValues[1], context.ByteRangeValues[2], context.ByteRangeValues[3])

	// Make sure our ByteRange string has the same length as the placeholder.
	if len(new_byte_range) < len(signatureByteRangePlaceholder) {
		new_byte_range += strings.Repeat(" ", len(signatureByteRangePlaceholder)-len(new_byte_range))
	} else if len(new_byte_range) != len(signatureByteRangePlaceholder) {
		return fmt.Errorf("new byte range string is the same lenght as the placeholder")
	}

	// Find the placeholder in the buffer (also restricted to the incremental
	// section for the same reason as above).
	placeholderIndex := bytes.Index(context.OutputBuffer.Buff.Bytes()[searchStart:], []byte(signatureByteRangePlaceholder))
	if placeholderIndex == -1 {
		return fmt.Errorf("failed to find ByteRange placeholder")
	}
	placeholderIndex += int(searchStart)

	// Replace the placeholder with the new byte range
	bufferBytes := context.OutputBuffer.Buff.Bytes()
	copy(bufferBytes[placeholderIndex:placeholderIndex+len(new_byte_range)], []byte(new_byte_range))

	// Rewrite the buffer with the updated bytes
	context.OutputBuffer.Buff.Reset()
	if _, err := context.OutputBuffer.Buff.Write(bufferBytes); err != nil {
		return err
	}

	return nil
}
