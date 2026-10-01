package sign

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	xrefStreamColumns   = 6 // Column width (1+4+1)
	xrefStreamPredictor = 12
	defaultPredictor    = 1  // No prediction (the default value)
	pngSubPredictor     = 11 // PNG prediction (on encoding, PNG Sub on all rows)
	pngUpPredictor      = 12 // PNG prediction (on encoding, PNG Up on all rows)
)

// writeXrefStream writes the cross-reference stream to the output buffer.
func (context *SignContext) writeXrefStream() error {
	var buffer bytes.Buffer

	predictor := context.PDFReader.Trailer().Key("DecodeParms").Key("Predictor").Int64()
	if predictor == 0 {
		predictor = xrefStreamPredictor
	}

	// El propio stream xref es un objeto más y debe tener su propia entrada
	// (ISO 32000-1 §7.5.8.4); sin ella qpdf y otros lectores avisan de xref
	// incompleta. Su id y su offset son deterministas antes de escribirlo:
	// será el siguiente objeto añadido al final del buffer actual.
	selfID, err := context.getNextObjectID()
	if err != nil {
		return fmt.Errorf("failed to reserve xref stream object id: %w", err)
	}
	selfOffset := int64(context.OutputBuffer.Buff.Len()) + 1

	if err := writeXrefStreamEntries(&buffer, context, selfOffset); err != nil {
		return fmt.Errorf("failed to write xref stream entries: %w", err)
	}

	streamBytes, err := encodeXrefStream(buffer.Bytes(), predictor)
	if err != nil {
		return fmt.Errorf("failed to encode xref stream: %w", err)
	}

	var xrefStreamObject bytes.Buffer

	if err := writeXrefStreamHeader(&xrefStreamObject, context, len(streamBytes), selfID); err != nil {
		return fmt.Errorf("failed to write xref stream header: %w", err)
	}

	if err := writeXrefStreamContent(&xrefStreamObject, streamBytes); err != nil {
		return fmt.Errorf("failed to write xref stream content: %w", err)
	}

	writtenID, err := context.addObject(xrefStreamObject.Bytes())
	if err != nil {
		return fmt.Errorf("failed to add xref stream object: %w", err)
	}
	if writtenID != selfID {
		return fmt.Errorf("xref stream object id mismatch: predicted %d, wrote %d", selfID, writtenID)
	}

	return nil
}

// writeXrefStreamEntries writes the individual entries for the xref stream,
// including the trailing self-entry for the xref stream object itself.
func writeXrefStreamEntries(buffer *bytes.Buffer, context *SignContext, selfOffset int64) error {
	// Write updated entries first
	for _, entry := range context.updatedXrefEntries {
		if err := writeXrefStreamLine(buffer, 1, entry.Offset, 0); err != nil {
			return fmt.Errorf("failed to write updated object %d: %w", entry.ID, err)
		}
	}

	// Write new entries
	for _, entry := range context.newXrefEntries {
		if err := writeXrefStreamLine(buffer, 1, entry.Offset, 0); err != nil {
			return fmt.Errorf("failed to write new object %d: %w", entry.ID, err)
		}
	}

	// Self-entry: the xref stream object is written right after the new
	// entries, so it extends the same contiguous /Index subsection.
	if err := writeXrefStreamLine(buffer, 1, selfOffset, 0); err != nil {
		return fmt.Errorf("failed to write xref stream self-entry: %w", err)
	}

	return nil
}

// encodeXrefStream applies the appropriate encoding to the xref stream.
func encodeXrefStream(data []byte, predictor int64) ([]byte, error) {
	// Use FlateDecode without prediction for xref streams
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// writeXrefStreamHeader writes the header for the xref stream.
func writeXrefStreamHeader(buffer *bytes.Buffer, context *SignContext, streamLength int, selfID uint32) error {
	id := context.PDFReader.Trailer().Key("ID")

	var indexArray []uint32

	// Add existing entries section
	if len(context.updatedXrefEntries) > 0 {
		for _, entry := range context.updatedXrefEntries {
			indexArray = append(indexArray, entry.ID, 1)
		}
	}

	// Add new entries section: the new objects plus the xref stream itself,
	// which is written immediately after and keeps the subsection contiguous.
	indexStart, err := checkedAddUint32(context.lastXrefID, 1)
	if err != nil {
		return fmt.Errorf("failed to calculate xref index start: %w", err)
	}
	indexLength, err := checkedAddUint32(1, len(context.newXrefEntries))
	if err != nil {
		return fmt.Errorf("failed to calculate xref index length: %w", err)
	}
	indexArray = append(indexArray, indexStart, indexLength)

	buffer.WriteString("<< /Type /XRef\n")
	fmt.Fprintf(buffer, "  /Length %d\n", streamLength)
	buffer.WriteString("  /Filter /FlateDecode\n")
	// Change W array to [1 4 1] to accommodate larger offsets
	buffer.WriteString("  /W [ 1 4 1 ]\n")
	fmt.Fprintf(buffer, "  /Prev %d\n", context.PDFReader.XrefInformation.StartPos)
	// Size shall be the highest object number in the file plus one; the xref
	// stream object itself is the highest we write.
	xrefSize, err := checkedAddUint32(selfID, 1)
	if err != nil {
		return fmt.Errorf("failed to calculate xref size: %w", err)
	}
	fmt.Fprintf(buffer, "  /Size %d\n", xrefSize)

	// Write index array if we have entries
	if len(indexArray) > 0 {
		buffer.WriteString("  /Index [")
		for _, idx := range indexArray {
			fmt.Fprintf(buffer, " %d", idx)
		}
		buffer.WriteString(" ]\n")
	}

	fmt.Fprintf(buffer, "  /Root %d 0 R\n", context.CatalogData.ObjectId)
	// Un PDF cifrado conserva su diccionario de cifrado en la actualización.
	if enc := context.PDFReader.Trailer().Key("Encrypt"); !enc.IsNull() {
		ptr := enc.GetPtr()
		fmt.Fprintf(buffer, "  /Encrypt %d %d R\n", ptr.GetID(), ptr.GetGen())
	}

	if !id.IsNull() {
		id0 := hex.EncodeToString([]byte(id.Index(0).RawString()))
		id1 := hex.EncodeToString([]byte(id.Index(1).RawString()))
		fmt.Fprintf(buffer, "  /ID [<%s><%s>]\n", id0, id1)
	}

	buffer.WriteString(">>\n")
	return nil
}

// writeXrefStreamContent writes the content of the xref stream.
func writeXrefStreamContent(buffer *bytes.Buffer, streamBytes []byte) error {
	if _, err := io.WriteString(buffer, "stream\n"); err != nil {
		return err
	}

	if _, err := buffer.Write(streamBytes); err != nil {
		return err
	}

	if _, err := io.WriteString(buffer, "\nendstream\n"); err != nil {
		return err
	}

	return nil
}

// writeXrefStreamLine writes a single line in the xref stream.
func writeXrefStreamLine(b *bytes.Buffer, xreftype byte, offset int64, gen byte) error {
	encodedOffset, err := checkedUint32(offset, "xref offset")
	if err != nil {
		return err
	}

	// Write type (1 byte)
	b.WriteByte(xreftype)

	// Write offset (4 bytes)
	offsetBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(offsetBytes, encodedOffset)
	b.Write(offsetBytes)

	// Write generation (1 byte)
	b.WriteByte(gen)
	return nil
}

// EncodePNGSUBBytes encodes data using PNG SUB filter.
func EncodePNGSUBBytes(columns int, data []byte) ([]byte, error) {
	if columns <= 0 {
		return nil, errors.New("columns must be positive")
	}
	rowCount := len(data) / columns
	if len(data)%columns != 0 {
		return nil, errors.New("invalid row/column length")
	}

	buffer := bytes.NewBuffer(nil)
	tmpRowData := make([]byte, columns)
	for i := 0; i < rowCount; i++ {
		rowData := data[columns*i : columns*(i+1)]
		tmpRowData[0] = rowData[0]
		for j := 1; j < columns; j++ {
			tmpRowData[j] = rowData[j] - rowData[j-1]
		}

		buffer.WriteByte(1)
		buffer.Write(tmpRowData)
	}

	data = buffer.Bytes()

	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

// EncodePNGUPBytes encodes data using PNG UP filter.
func EncodePNGUPBytes(columns int, data []byte) ([]byte, error) {
	if columns <= 0 {
		return nil, errors.New("columns must be positive")
	}
	rowCount := len(data) / columns
	if len(data)%columns != 0 {
		return nil, errors.New("invalid row/column length")
	}

	prevRowData := make([]byte, columns)

	// Initially all previous data is zero.
	for i := 0; i < columns; i++ {
		prevRowData[i] = 0
	}

	buffer := bytes.NewBuffer(nil)
	tmpRowData := make([]byte, columns)
	for i := 0; i < rowCount; i++ {
		rowData := data[columns*i : columns*(i+1)]
		for j := 0; j < columns; j++ {
			tmpRowData[j] = rowData[j] - prevRowData[j]
		}

		// Save the previous row for prediction.
		copy(prevRowData, rowData)

		buffer.WriteByte(2)
		buffer.Write(tmpRowData)
	}

	data = buffer.Bytes()

	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
