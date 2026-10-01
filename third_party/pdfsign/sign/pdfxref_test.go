package sign

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/digitorus/pdf"
	"github.com/mattetti/filebuffer"
)

func TestGetLastObjectIDFromXref(t *testing.T) {
	testCases := []struct {
		fileName string
		expected uint32
	}{
		{"testfile12.pdf", 11},
		{"testfile14.pdf", 6},
		{"testfile16.pdf", 18},
		{"testfile17.pdf", 20},
		{"testfile20.pdf", 10},
	}

	for _, tc := range testCases {
		t.Run(tc.fileName, func(st *testing.T) {
			st.Parallel()

			input_file, err := os.Open("../testfiles/" + tc.fileName)
			if err != nil {
				st.Fatalf("%s: %s", tc.fileName, err.Error())
			}
			defer func() {
				if err := input_file.Close(); err != nil {
					st.Errorf("Failed to close input_file: %v", err)
				}
			}()

			finfo, err := input_file.Stat()
			if err != nil {
				st.Fatalf("%s: %s", tc.fileName, err.Error())
			}
			size := finfo.Size()

			r, err := pdf.NewReader(input_file, size)
			if err != nil {
				st.Fatalf("%s: %s", tc.fileName, err.Error())
			}

			sc := &SignContext{
				InputFile: input_file,
				PDFReader: r,
			}
			obj, err := sc.getLastObjectIDFromXref()
			if err != nil {
				st.Fatalf("%s: %s", tc.fileName, err.Error())
			}
			if obj != tc.expected {
				st.Fatalf("%s: expected object id %d, got %d", tc.fileName, tc.expected, obj)
			}
		})
	}
}

func TestAddObject(t *testing.T) {
	outputBuf := &filebuffer.Buffer{
		Buff: new(bytes.Buffer),
	}
	context := &SignContext{
		OutputBuffer: outputBuf,
		lastXrefID:   10,
	}

	tests := []struct {
		name         string
		object       []byte
		expectedID   uint32
		expectedText string
		wantErr      bool
	}{
		{
			name:         "valid object",
			object:       []byte("test object"),
			expectedID:   11,
			expectedText: "11 0 obj\ntest object\nendobj\n",
			wantErr:      false,
		},
		{
			name:         "object with whitespace",
			object:       []byte("  test object  "),
			expectedID:   12,
			expectedText: "12 0 obj\ntest object\nendobj\n",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputBuf.Buff.Reset()
			id, err := context.addObject(tt.object)
			if (err != nil) != tt.wantErr {
				t.Errorf("addObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if id != tt.expectedID {
				t.Errorf("addObject() got ID = %v, want %v", id, tt.expectedID)
			}

			got := outputBuf.Buff.String()
			if !strings.Contains(got, tt.expectedText) {
				t.Errorf("addObject() output = %q, want to contain %q", got, tt.expectedText)
			}

			// Check xref entry
			if len(context.newXrefEntries) == 0 {
				t.Error("No xref entry added")
			} else {
				lastEntry := context.newXrefEntries[len(context.newXrefEntries)-1]
				if lastEntry.ID != tt.expectedID {
					t.Errorf("xref entry ID = %v, want %v", lastEntry.ID, tt.expectedID)
				}
			}
		})
	}
}

func TestGetNextObjectIDRejectsOverflow(t *testing.T) {
	context := &SignContext{lastXrefID: math.MaxUint32}
	if _, err := context.getNextObjectID(); err == nil {
		t.Fatal("getNextObjectID() did not reject an overflowing object ID")
	}
}

func TestAddObjectRejectsOverflowWithoutWriting(t *testing.T) {
	outputBuf := &filebuffer.Buffer{Buff: new(bytes.Buffer)}
	context := &SignContext{
		OutputBuffer: outputBuf,
		lastXrefID:   math.MaxUint32,
	}

	if _, err := context.addObject([]byte("must not be written")); err == nil {
		t.Fatal("addObject() did not reject an overflowing object ID")
	}
	if outputBuf.Buff.Len() != 0 || len(context.newXrefEntries) != 0 {
		t.Fatal("addObject() mutated the output after rejecting the object ID")
	}
}

func TestUpdateObject(t *testing.T) {
	context := &SignContext{
		OutputBuffer: &filebuffer.Buffer{
			Buff: new(bytes.Buffer),
		},
		lastXrefID: 10,
	}

	tests := []struct {
		name         string
		objectID     uint32
		object       []byte
		expectedText string
		wantErr      bool
	}{
		{
			name:         "valid update",
			objectID:     5,
			object:       []byte("updated content"),
			expectedText: "5 0 obj\nupdated content\nendobj\n",
			wantErr:      false,
		},
		{
			name:         "update with whitespace",
			objectID:     8,
			object:       []byte("  updated content  "),
			expectedText: "8 0 obj\nupdated content\nendobj\n",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context.OutputBuffer.Buff.Reset()
			err := context.updateObject(tt.objectID, tt.object)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			got := context.OutputBuffer.Buff.String()
			if !strings.Contains(got, tt.expectedText) {
				t.Errorf("updateObject() output = %q, want to contain %q", got, tt.expectedText)
			}

			// Check xref entry
			if len(context.updatedXrefEntries) == 0 {
				t.Error("No updated xref entry added")
			} else {
				lastEntry := context.updatedXrefEntries[len(context.updatedXrefEntries)-1]
				if lastEntry.ID != tt.objectID {
					t.Errorf("xref entry ID = %v, want %v", lastEntry.ID, tt.objectID)
				}
			}
		})
	}
}

func TestWriteObject(t *testing.T) {
	context := &SignContext{
		OutputBuffer: &filebuffer.Buffer{
			Buff: new(bytes.Buffer),
		},
	}

	tests := []struct {
		name         string
		objectID     uint32
		object       []byte
		expectedText string
		wantErr      bool
	}{
		{
			name:         "simple object",
			objectID:     1,
			object:       []byte("test content"),
			expectedText: "1 0 obj\ntest content\nendobj\n",
			wantErr:      false,
		},
		{
			name:         "empty object",
			objectID:     2,
			object:       []byte{},
			expectedText: "2 0 obj\n\nendobj\n",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context.OutputBuffer.Buff.Reset()
			err := context.writeObject(tt.objectID, tt.object)
			if (err != nil) != tt.wantErr {
				t.Errorf("writeObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			got := context.OutputBuffer.Buff.String()
			if !strings.Contains(got, tt.expectedText) {
				t.Errorf("writeObject() output = %q, want to contain %q", got, tt.expectedText)
			}
		})
	}
}

// TestWriteIncrXrefTable tests the writeIncrXref function with a table xref type.
func TestWriteXrefTypeTable(t *testing.T) {
	context := &SignContext{
		OutputBuffer: &filebuffer.Buffer{
			Buff: new(bytes.Buffer),
		},
		newXrefEntries: []xrefEntry{
			{ID: 1, Offset: 100, Generation: 0, Free: false},
			{ID: 2, Offset: 200, Generation: 0, Free: false},
		},
		lastXrefID: 2,
	}

	context.PDFReader = &pdf.Reader{
		XrefInformation: pdf.ReaderXrefInformation{
			Type: "table",
		},
	}

	err := context.writeXref()
	if err != nil {
		t.Errorf("writeXref() error = %v", err)
		return
	}

	got := context.OutputBuffer.Buff.String()
	expect := "\nxref\n3 2\n0000000100 00000 n\r\n0000000200 00000 n\r\n"
	if got != expect {
		t.Errorf("writeXref() output = %q, want %q", got, expect)
	}
}

// TestWriteXrefTypeStream verifies the incremental xref stream dictionary and entries.
func TestWriteXrefTypeStream(t *testing.T) {
	context := &SignContext{
		OutputBuffer: &filebuffer.Buffer{
			Buff: new(bytes.Buffer),
		},
		newXrefEntries: []xrefEntry{
			{ID: 3, Offset: 100, Generation: 0, Free: false},
			{ID: 4, Offset: 200, Generation: 0, Free: false},
		},
		lastXrefID: 2,
	}

	context.PDFReader = &pdf.Reader{
		XrefInformation: pdf.ReaderXrefInformation{
			Type: "stream",
		},
	}

	if err := context.writeXref(); err != nil {
		t.Fatalf("writeXref() error = %v", err)
	}

	got := context.OutputBuffer.Buff.Bytes()
	for _, required := range []string{
		"\n\n5 0 obj\n<< /Type /XRef\n",
		"  /Filter /FlateDecode\n",
		"  /W [ 1 4 1 ]\n",
		"  /Prev 0\n",
		"  /Size 6\n",
		"  /Index [ 3 3 ]\n",
		"  /Root 0 0 R\n",
		"\nendstream\nendobj\n",
	} {
		if !bytes.Contains(got, []byte(required)) {
			t.Errorf("writeXref() output missing %q", required)
		}
	}

	streamStartMarker := []byte("stream\n")
	streamEndMarker := []byte("\nendstream")
	streamStart := bytes.Index(got, streamStartMarker)
	if streamStart < 0 {
		t.Fatal("writeXref() output has no stream start")
	}
	streamStart += len(streamStartMarker)
	streamEnd := bytes.Index(got[streamStart:], streamEndMarker)
	if streamEnd < 0 {
		t.Fatal("writeXref() output has no stream end")
	}
	compressed := got[streamStart : streamStart+streamEnd]
	if !bytes.Contains(got[:streamStart], []byte(fmt.Sprintf("  /Length %d\n", len(compressed)))) {
		t.Errorf("xref stream /Length does not match compressed payload size %d", len(compressed))
	}

	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("open xref stream: %v", err)
	}
	entries, err := io.ReadAll(zr)
	if err != nil {
		_ = zr.Close()
		t.Fatalf("read xref stream: %v", err)
	}
	if err := zr.Close(); err != nil {
		t.Fatalf("close xref stream: %v", err)
	}

	// /Index [3 3] maps these records to objects 3, 4 and the xref stream
	// itself (object 5). The latter starts at byte offset 2 in this fixture.
	wantEntries := []byte{
		1, 0, 0, 0, 100, 0,
		1, 0, 0, 0, 200, 0,
		1, 0, 0, 0, 2, 0,
	}
	if !bytes.Equal(entries, wantEntries) {
		t.Errorf("decoded xref entries = %v, want %v", entries, wantEntries)
	}

	if context.NewXrefStart != 1 {
		t.Errorf("NewXrefStart = %d, want 1", context.NewXrefStart)
	}
	if len(context.newXrefEntries) != 3 {
		t.Fatalf("newXrefEntries has %d entries, want 3", len(context.newXrefEntries))
	}
	lastEntry := context.newXrefEntries[len(context.newXrefEntries)-1]
	if lastEntry.ID != 5 || lastEntry.Offset != 2 {
		t.Errorf("xref self-entry = {ID:%d Offset:%d}, want {ID:5 Offset:2}", lastEntry.ID, lastEntry.Offset)
	}
}
