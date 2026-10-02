// Copyright 2026 Alberto Avidad Fernández. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pdf

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
)

// XRefEntrySnapshot preserves an object's active/free state and generation.
// Offset is the file offset, or the index within ObjectStream for compressed
// objects. An entry is not evidence that the referenced object is benign.
type XRefEntrySnapshot struct {
	Number       uint32
	Generation   uint16
	Active       bool
	InStream     bool
	ObjectStream uint32
	Offset       int64
}

// XRefSnapshot is the effective xref at the end of one PDF update. Reopening
// every prefix is intentional: the final xref must not stand in for earlier
// revisions when a later update frees or reuses an object.
type XRefSnapshot struct {
	Length            int
	StartXRef         int64
	PrevXRef          int64
	XRefType          string
	RootObject        uint32
	RootGeneration    uint16
	DocumentID        string
	EncryptObject     uint32
	EncryptGeneration uint16
	InfoObject        uint32
	InfoGeneration    uint16
	InfoValue         string
	Entries           map[uint32]XRefEntrySnapshot
}

// ReadXRefSnapshots opens each complete PDF revision independently. Lengths
// must be strictly increasing and end at that revision's %%EOF. The caller
// supplies its revision limit; zero selects the default of 20.
// Unsupported xref combinations return an error, so callers cannot infer
// that changes were permitted from a partial parse.
func ReadXRefSnapshots(data []byte, lengths []int, maxRevisions int) ([]XRefSnapshot, error) {
	if maxRevisions <= 0 {
		maxRevisions = 20
	}
	if len(lengths) == 0 || len(lengths) > maxRevisions {
		return nil, errors.New("numero de revisiones fuera de limite")
	}
	out := make([]XRefSnapshot, 0, len(lengths))
	previous := 0
	for _, length := range lengths {
		if length <= previous || length > len(data) {
			return nil, errors.New("longitudes de revision invalidas")
		}
		prefix := data[:length]
		if !bytes.HasSuffix(bytes.TrimSpace(prefix), []byte("%%EOF")) {
			return nil, errors.New("la revision no termina en %%EOF")
		}
		snapshot, err := readXRefSnapshot(prefix)
		if err != nil {
			return nil, err
		}
		out = append(out, snapshot)
		previous = length
	}
	return out, nil
}

func readXRefSnapshot(data []byte) (snapshot XRefSnapshot, err error) {
	defer func() {
		if recover() != nil {
			snapshot = XRefSnapshot{}
			err = errors.New("xref de revision no analizable")
		}
	}()
	reader, err := NewReaderWithXRefLimit(bytes.NewReader(data), int64(len(data)), 50_000)
	if err != nil {
		return XRefSnapshot{}, fmt.Errorf("xref de revision no analizable: %w", err)
	}
	snapshot = XRefSnapshot{
		Length:    len(data),
		StartXRef: reader.XrefInformation.StartPos,
		XRefType:  reader.XrefInformation.Type,
		Entries:   make(map[uint32]XRefEntrySnapshot, len(reader.xref)),
	}
	for key := range reader.trailer {
		switch key {
		case "Size", "Prev", "Root", "Encrypt", "ID", "Info", "XRefStm", "Type", "W", "Index", "Length", "Filter", "DecodeParms":
		default:
			return XRefSnapshot{}, fmt.Errorf("xref de revision con clave de trailer no clasificada: /%s", key)
		}
	}
	if root, ok := reader.trailer["Root"].(objptr); ok {
		snapshot.RootObject = root.id
		snapshot.RootGeneration = root.gen
	}
	if id, exists := reader.trailer["ID"]; exists {
		snapshot.DocumentID = objfmt(id)
	}
	if enc, ok := reader.trailer["Encrypt"].(objptr); ok {
		snapshot.EncryptObject, snapshot.EncryptGeneration = enc.id, enc.gen
	}
	if info, exists := reader.trailer["Info"]; exists {
		snapshot.InfoValue = objfmt(info)
		if ref, ok := info.(objptr); ok {
			snapshot.InfoObject, snapshot.InfoGeneration = ref.id, ref.gen
		}
	}
	if prev, ok := reader.trailer["Prev"].(int64); ok {
		snapshot.PrevXRef = prev
	}
	for number, entry := range reader.xref {
		if !entry.defined {
			continue
		}
		snapshot.Entries[uint32(number)] = XRefEntrySnapshot{
			Number:       uint32(number),
			Generation:   entry.ptr.gen,
			Active:       !entry.free,
			InStream:     entry.inStream,
			ObjectStream: entry.stream.id,
			Offset:       entry.offset,
		}
	}
	return snapshot, nil
}

// ChangedXRefObjects returns object numbers whose effective xref changed.
// This includes newly free objects and reused numbers with a new generation.
// A caller must still compare active object contents and policy before
// classifying any update as permitted.
func ChangedXRefObjects(before, after XRefSnapshot) []uint32 {
	changed := make(map[uint32]struct{})
	for number, prior := range before.Entries {
		if next, ok := after.Entries[number]; !ok || prior != next {
			changed[number] = struct{}{}
		}
	}
	for number, next := range after.Entries {
		if prior, ok := before.Entries[number]; !ok || prior != next {
			changed[number] = struct{}{}
		}
	}
	out := make([]uint32, 0, len(changed))
	for number := range changed {
		out = append(out, number)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
