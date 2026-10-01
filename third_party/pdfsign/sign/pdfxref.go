package sign

import (
	"bytes"
	"fmt"
)

type xrefEntry struct {
	ID         uint32
	Offset     int64
	Generation int
	Free       bool
}

const (
	objectFooter = "\nendobj\n"
)

func (context *SignContext) getNextObjectID() (uint32, error) {
	if context.lastXrefID == 0 {
		lastXrefID, err := context.getLastObjectIDFromXref()
		if err != nil {
			return 0, fmt.Errorf("failed to get last object ID: %w", err)
		}
		context.lastXrefID = lastXrefID
	}

	objectID, err := checkedAddUint32(context.lastXrefID, len(context.newXrefEntries))
	if err != nil {
		return 0, fmt.Errorf("failed to calculate next object ID: %w", err)
	}
	objectID, err = checkedAddUint32(objectID, 1)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate next object ID: %w", err)
	}
	return objectID, nil
}

func (context *SignContext) addObject(object []byte) (uint32, error) {
	objectID, err := context.getNextObjectID()
	if err != nil {
		return 0, err
	}
	context.newXrefEntries = append(context.newXrefEntries, xrefEntry{
		ID:     objectID,
		Offset: int64(context.OutputBuffer.Buff.Len()) + 1,
	})

	err = context.writeObject(objectID, object)
	if err != nil {
		return 0, fmt.Errorf("failed to write object: %w", err)
	}

	return objectID, nil
}

func (context *SignContext) updateObject(id uint32, object []byte) error {
	context.updatedXrefEntries = append(context.updatedXrefEntries, xrefEntry{
		ID:     id,
		Offset: int64(context.OutputBuffer.Buff.Len()) + 1,
	})

	err := context.writeObject(id, object)
	if err != nil {
		return fmt.Errorf("failed to write object: %w", err)
	}

	return nil
}

func (context *SignContext) writeObject(id uint32, object []byte) error {
	// Write the object header
	if _, err := fmt.Fprintf(context.OutputBuffer, "\n%d 0 obj\n", id); err != nil {
		return fmt.Errorf("failed to write object header: %w", err)
	}

	// Write the object content
	object = bytes.TrimSpace(object)
	if context.cifrador != nil {
		cifrado, err := context.cifrador.cifrarObjeto(id, object)
		if err != nil {
			return fmt.Errorf("failed to encrypt object %d: %w", id, err)
		}
		object = cifrado
	}
	if _, err := context.OutputBuffer.Write(object); err != nil {
		return fmt.Errorf("failed to write object content: %w", err)
	}

	// Write the object footer
	if _, err := context.OutputBuffer.Write([]byte(objectFooter)); err != nil {
		return fmt.Errorf("failed to write object footer: %w", err)
	}

	return nil
}

// writeXref writes the cross-reference table or stream based on the PDF type.
func (context *SignContext) writeXref() error {
	if _, err := context.OutputBuffer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline before xref: %w", err)
	}
	context.NewXrefStart = int64(context.OutputBuffer.Buff.Len())

	switch context.PDFReader.XrefInformation.Type {
	case "table":
		return context.writeIncrXrefTable()
	case "stream":
		return context.writeXrefStream()
	default:
		return fmt.Errorf("unknown xref type: %s", context.PDFReader.XrefInformation.Type)
	}
}

func (context *SignContext) getLastObjectIDFromXref() (uint32, error) {
	xref := context.PDFReader.Xref()
	if len(xref) == 0 {
		return 0, fmt.Errorf("no xref entries found")
	}

	// Find highest used object ID
	var maxID uint32
	for _, entry := range xref {
		ptr := entry.Ptr()

		// TODO: Check if in use (&& entry.offset != 0)
		if ptr.GetID() > maxID {
			maxID = ptr.GetID()
		}
	}

	lastObjectID, err := checkedAddUint32(maxID, 1)
	if err != nil {
		return 0, fmt.Errorf("xref object ID exceeds the supported range: %w", err)
	}
	return lastObjectID, nil
}
