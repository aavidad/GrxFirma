// Copyright 2026 Alberto Avidad Fernández. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pdf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

func TestReadXRefSnapshots_FreeAndReusedObject(t *testing.T) {
	var file bytes.Buffer
	file.WriteString("%PDF-1.4\n%" + strings.Repeat("x", 230) + "\n")
	object1 := file.Len()
	file.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	xref1 := file.Len()
	file.WriteString("xref\n0 2\n0000000000 65535 f \n")
	fmt.Fprintf(&file, "%010d 00000 n \n", object1)
	fmt.Fprintf(&file, "trailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref1)
	first := file.Len()
	object2 := file.Len()
	file.WriteString("2 0 obj\n<< /Type /Catalog >>\nendobj\n")
	xref2 := file.Len()
	file.WriteString("xref\n1 2\n0000000000 00001 f \n")
	fmt.Fprintf(&file, "%010d 00000 n \n", object2)
	fmt.Fprintf(&file, "trailer\n<< /Size 3 /Root 2 0 R /ID [<01> <02>] /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", xref1, xref2)
	second := file.Len()
	object1Reused := file.Len()
	file.WriteString("1 1 obj\n<< /Type /Catalog >>\nendobj\n")
	xref3 := file.Len()
	fmt.Fprintf(&file, "xref\n1 1\n%010d 00001 n \n", object1Reused)
	fmt.Fprintf(&file, "trailer\n<< /Size 3 /Root 1 1 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", xref2, xref3)
	third := file.Len()

	snapshots, err := ReadXRefSnapshots(file.Bytes(), []int{first, second, third}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshots[0].Entries[1].Active || snapshots[0].Entries[1].Generation != 0 || snapshots[0].RootObject != 1 {
		t.Fatalf("primera revisión: %+v", snapshots[0])
	}
	if snapshots[1].Entries[1].Active || snapshots[1].Entries[1].Generation != 1 || snapshots[1].RootObject != 2 {
		t.Fatalf("objeto libre resucitado o trailer antiguo: %+v", snapshots[1])
	}
	if snapshots[0].DocumentID == snapshots[1].DocumentID {
		t.Fatal("no se detectó el cambio de /ID del tráiler")
	}
	if !snapshots[2].Entries[1].Active || snapshots[2].Entries[1].Generation != 1 || snapshots[2].RootObject != 1 {
		t.Fatalf("objeto reutilizado perdido: %+v", snapshots[2])
	}
	if changed := ChangedXRefObjects(snapshots[0], snapshots[1]); len(changed) != 2 || changed[0] != 1 || changed[1] != 2 {
		t.Fatalf("cambios primera a segunda revisión: %v", changed)
	}
	if _, err := ReadXRefSnapshots(file.Bytes(), []int{first, second, third}, 2); err == nil {
		t.Fatal("se ignoró el límite de revisiones")
	}
	if _, err := ReadXRefSnapshots(file.Bytes(), []int{first - 5}, 3); err == nil {
		t.Fatal("se aceptó una revisión sin marcador EOF")
	}
}

func TestReadXRefSnapshots_MixedTableStreamAndHybrid(t *testing.T) {
	var file bytes.Buffer
	file.WriteString("%PDF-1.7\n%" + strings.Repeat("x", 230) + "\n")
	root := file.Len()
	file.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	firstXref := file.Len()
	fmt.Fprintf(&file, "xref\n0 2\n0000000000 65535 f \n%010d 00000 n \ntrailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", root, firstXref)
	first := file.Len()
	object2 := file.Len()
	file.WriteString("2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n")
	streamXref := file.Len()
	appendXRefStreamTest(&file, 3, 4, firstXref, map[uint32]int{1: root, 2: object2, 3: streamXref})
	fmt.Fprintf(&file, "startxref\n%d\n%%%%EOF\n", streamXref)
	second := file.Len()
	object4 := file.Len()
	file.WriteString("4 0 obj\n<< /Type /Metadata >>\nendobj\n")
	hybridStream := file.Len()
	appendXRefStreamTest(&file, 5, 6, 0, map[uint32]int{4: object4, 5: hybridStream})
	hybridTable := file.Len()
	fmt.Fprintf(&file, "xref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size 6 /Root 1 0 R /Prev %d /XRefStm %d >>\nstartxref\n%d\n%%%%EOF\n", streamXref, hybridStream, hybridTable)
	third := file.Len()

	snapshots, err := ReadXRefSnapshots(file.Bytes(), []int{first, second, third}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if snapshots[1].XRefType != "stream" || !snapshots[1].Entries[2].Active || snapshots[1].Entries[2].Offset != int64(object2) {
		t.Fatalf("actualización a stream mal leída: %+v", snapshots[1])
	}
	if snapshots[2].XRefType != "table" || !snapshots[2].Entries[4].Active || snapshots[2].Entries[4].Offset != int64(object4) || !snapshots[2].Entries[2].Active {
		t.Fatalf("actualización híbrida mal leída: %+v", snapshots[2])
	}
}

// A hybrid xref stream may advertise a different /Size; the table trailer
// controls the effective revision size. The stream can still supply entries
// within that size.
func TestReadXRefSnapshots_HybridTableSizeTakesPrecedence(t *testing.T) {
	var file bytes.Buffer
	file.WriteString("%PDF-1.7\n%" + strings.Repeat("x", 230) + "\n")
	root := file.Len()
	file.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	streamOffset := file.Len()
	appendXRefStreamTest(&file, 2, 4, 0, map[uint32]int{1: root, 2: streamOffset})
	tableOffset := file.Len()
	fmt.Fprintf(&file, "xref\n0 2\n0000000000 65535 f \n%010d 00000 n \ntrailer\n<< /Size 3 /Root 1 0 R /XRefStm %d >>\nstartxref\n%d\n%%%%EOF\n", root, streamOffset, tableOffset)

	snapshots, err := ReadXRefSnapshots(file.Bytes(), []int{file.Len()}, 1)
	if err != nil {
		t.Fatalf("se rechazó un xref híbrido válido con /Size diferente: %v", err)
	}
	snapshot := snapshots[0]
	if snapshot.XRefType != "table" || snapshot.RootObject != 1 || len(snapshot.Entries) != 3 || !snapshot.Entries[2].Active || snapshot.Entries[2].Offset != int64(streamOffset) {
		t.Fatalf("el tráiler de la tabla o las entradas del flujo no prevalecieron: %+v", snapshot)
	}
}

func TestReadXRefSnapshots_HybridSizeDoesNotHideTrailerConflicts(t *testing.T) {
	for _, tc := range []struct {
		name, streamTrailer, tableTrailer, conflict string
	}{
		{"Root", "/Root 2 0 R", "/Root 1 0 R", "/Root"},
		{"Encrypt", "/Root 1 0 R /Encrypt 2 0 R", "/Root 1 0 R /Encrypt 1 0 R", "/Encrypt"},
		{"ID", "/Root 1 0 R /ID [<01> <02>]", "/Root 1 0 R /ID [<01> <03>]", "/ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var file bytes.Buffer
			file.WriteString("%PDF-1.7\n%" + strings.Repeat("x", 230) + "\n")
			root := file.Len()
			file.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")
			streamOffset := file.Len()
			entry := make([]byte, 7)
			entry[0] = 1
			binary.BigEndian.PutUint32(entry[1:5], uint32(streamOffset))
			fmt.Fprintf(&file, "2 0 obj\n<< /Type /XRef /Size 4 %s /W [1 4 2] /Index [2 1] /Length 7 >>\nstream\n", tc.streamTrailer)
			file.Write(entry)
			file.WriteString("\nendstream\nendobj\n")
			tableOffset := file.Len()
			fmt.Fprintf(&file, "xref\n0 2\n0000000000 65535 f \n%010d 00000 n \ntrailer\n<< /Size 3 %s /XRefStm %d >>\nstartxref\n%d\n%%%%EOF\n", root, tc.tableTrailer, streamOffset, tableOffset)
			_, err := ReadXRefSnapshots(file.Bytes(), []int{file.Len()}, 1)
			if err == nil || !strings.Contains(err.Error(), tc.conflict) {
				t.Fatalf("se esperaba conflicto %s, obtenido %v", tc.conflict, err)
			}
		})
	}
}

func appendXRefStreamTest(file *bytes.Buffer, object, size int, prev int, active map[uint32]int) {
	start, count := 0, size
	if prev == 0 {
		start, count = object-1, 2
	}
	data := make([]byte, count*7)
	for i := start; i < start+count; i++ {
		pos := (i - start) * 7
		if i == 0 {
			binary.BigEndian.PutUint16(data[pos+5:pos+7], 65535)
			continue
		}
		if off, ok := active[uint32(i)]; ok {
			data[pos] = 1
			binary.BigEndian.PutUint32(data[pos+1:pos+5], uint32(off))
		}
	}
	fmt.Fprintf(file, "%d 0 obj\n<< /Type /XRef /Size %d /Root 1 0 R /W [1 4 2] /Length %d", object, size, len(data))
	if prev == 0 {
		fmt.Fprintf(file, " /Index [%d %d]", start, count)
	}
	if prev > 0 {
		fmt.Fprintf(file, " /Prev %d", prev)
	}
	file.WriteString(" >>\nstream\n")
	file.Write(data)
	file.WriteString("\nendstream\nendobj\n")
}

func TestReadXRefSnapshots_RejectsHugeDeclaredStreamSize(t *testing.T) {
	var file bytes.Buffer
	file.WriteString("%PDF-1.7\n%" + strings.Repeat("x", 230) + "\n")
	offset := file.Len()
	file.WriteString("1 0 obj\n<< /Type /XRef /Size 8388608 /Root 1 0 R /W [1 4 2] /Length 0 >>\nstream\n\nendstream\nendobj\n")
	fmt.Fprintf(&file, "startxref\n%d\n%%%%EOF\n", offset)
	if _, err := ReadXRefSnapshots(file.Bytes(), []int{file.Len()}, 20); err == nil {
		t.Fatal("se intentó cargar un xref stream de millones de entradas")
	}
}

func TestReadXRefSnapshots_RejectsConflictingHybridPrev(t *testing.T) {
	var file bytes.Buffer
	file.WriteString("%PDF-1.7\n%" + strings.Repeat("x", 230) + "\n")
	root := file.Len()
	file.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	firstXRef := file.Len()
	fmt.Fprintf(&file, "xref\n0 2\n0000000000 65535 f \n%010d 00000 n \ntrailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", root, firstXRef)
	first := file.Len()
	streamOffset := file.Len()
	entry := make([]byte, 7)
	entry[0] = 1
	binary.BigEndian.PutUint32(entry[1:5], uint32(streamOffset))
	fmt.Fprintf(&file, "2 0 obj\n<< /Type /XRef /Size 3 /Root 1 0 R /Prev %d /W [1 4 2] /Index [2 1] /Length 7 >>\nstream\n", firstXRef+1)
	file.Write(entry)
	file.WriteString("\nendstream\nendobj\n")
	secondXRef := file.Len()
	fmt.Fprintf(&file, "xref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size 3 /Root 1 0 R /Prev %d /XRefStm %d >>\nstartxref\n%d\n%%%%EOF\n", firstXRef, streamOffset, secondXRef)
	if _, err := ReadXRefSnapshots(file.Bytes(), []int{first, file.Len()}, 3); err == nil {
		t.Fatal("se aceptaron dos cadenas /Prev incompatibles en una revisión híbrida")
	}
}
