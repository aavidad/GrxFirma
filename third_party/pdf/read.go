// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pdf implements reading of PDF files.
//
// # Overview
//
// PDF is Adobe's Portable Document Format, ubiquitous on the internet.
// A PDF document is a complex data format built on a fairly simple structure.
// This package exposes the simple structure along with some wrappers to
// extract basic information. If more complex information is needed, it is
// possible to extract that information by interpreting the structure exposed
// by this package.
//
// Specifically, a PDF is a data structure built from Values, each of which has
// one of the following Kinds:
//
//	Null, for the null object.
//	Integer, for an integer.
//	Real, for a floating-point number.
//	Bool, for a boolean value.
//	Name, for a name constant (as in /Helvetica).
//	String, for a string constant.
//	Dict, for a dictionary of name-value pairs.
//	Array, for an array of values.
//	Stream, for an opaque data stream and associated header dictionary.
//
// The accessors on Value—Int64, Float64, Bool, Name, and so on—return
// a view of the data as the given type. When there is no appropriate view,
// the accessor returns a zero result. For example, the Name accessor returns
// the empty string if called on a Value v for which v.Kind() != Name.
// Returning zero values this way, especially from the Dict and Array accessors,
// which themselves return Values, makes it possible to traverse a PDF quickly
// without writing any error checking. On the other hand, it means that mistakes
// can go unreported.
//
// The basic structure of the PDF file is exposed as the graph of Values.
//
// Most richer data structures in a PDF file are dictionaries with specific interpretations
// of the name-value pairs. The Font and Page wrappers make the interpretation
// of a specific Value as the corresponding type easier. They are only helpers, though:
// they are implemented only in terms of the Value API and could be moved outside
// the package. Equally important, traversal of other PDF data structures can be implemented
// in other packages as needed.
package pdf

// BUG(rsc): The package is incomplete, although it has been used successfully on some
// large real-world PDF files.

// BUG(rsc): There is no support for closing open PDF files. If you drop all references to a Reader,
// the underlying reader will eventually be garbage collected.

// BUG(rsc): The library makes no attempt at efficiency. A value cache maintained in the Reader
// would probably help significantly.

// BUG(rsc): The support for reading encrypted files is weak.

// BUG(rsc): The Value API does not support error reporting. The intent is to allow users to
// set an error reporting callback in Reader, but that code has not been implemented.

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"sync/atomic"
)

// A Reader is a single PDF file open for reading.
type Reader struct {
	f               io.ReaderAt
	end             int64
	xref            []xref
	trailer         dict
	trailerptr      objptr
	key             []byte
	useAES          bool
	cifrado         ParametrosCifrado
	XrefInformation ReaderXrefInformation
	PDFVersion      string
	xrefLimit       int64
	// AutoFirmaV2: objetos comprimidos que se están resolviendo a la vez.
	// Un flujo de objetos cuyo /Length o /Extends lleva a un objeto de ese
	// mismo flujo recursaba sin fin.
	resolviendoObjStm atomic.Int32
}

const (
	maxAnidamientoObjStm = 32
	maxCadenaExtends     = 64
)

type ReaderXrefInformation struct {
	StartPos               int64
	EndPos                 int64
	Length                 int64
	PositionLength         int64
	PositionStartPos       int64
	PositionEndPos         int64
	ItemCount              int64
	Type                   string
	IncludingTrailerEndPos int64
	IncludingTrailerLength int64
}

func (info *ReaderXrefInformation) PrintDebug() {
	log.Printf("Start of xref position bytes: %d", info.PositionStartPos)
	log.Printf("Length of xref position bytes: %d", info.PositionLength)
	log.Printf("End of xref position bytes: %d", info.PositionEndPos)
	log.Printf("xref start position byte: %d", info.StartPos)
	log.Printf("xref end position byte: %d", info.EndPos)
	log.Printf("xref length in bytes: %d", info.Length)
	log.Printf("xref type: %s", info.Type)
	log.Printf("Amount of items in xref: %d", info.ItemCount)
	log.Printf("xref end (including trailer) position byte: %d", info.IncludingTrailerEndPos)
	log.Printf("xref length (including trailer) in bytes: %d", info.IncludingTrailerLength)
}

type xref struct {
	ptr      objptr
	inStream bool
	stream   objptr
	offset   int64
	defined  bool
	free     bool
}

func (x *xref) Ptr() objptr {
	return x.ptr
}

func (x *xref) Stream() objptr {
	return x.stream
}

func GetDict() dict {
	return dict{}
}

func (r *Reader) errorf(format string, args ...interface{}) {
	panic(fmt.Errorf(format, args...))
}

func (r *Reader) Xref() []xref {
	return r.xref
}

// Open opens a file for reading.
func Open(file string) (*Reader, error) {
	// TODO: Deal with closing file.
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return NewReader(f, fi.Size())
}

// NewReader opens a file for reading, using the data in f with the given total size.
func NewReader(f io.ReaderAt, size int64) (*Reader, error) {
	return NewReaderEncrypted(f, size, nil)
}

// NewReaderEncrypted opens a file for reading, using the data in f with the given total size.
// If the PDF is encrypted, NewReaderEncrypted calls pw repeatedly to obtain passwords
// to try. If pw returns the empty string, NewReaderEncrypted stops trying to decrypt
// the file and returns an error.
func NewReaderEncrypted(f io.ReaderAt, size int64, pw func() string) (*Reader, error) {
	return newReaderEncryptedLimited(f, size, pw, maxObjetosPDF+1)
}

// NewReaderWithXRefLimit rejects a declared xref size above limit before
// allocating the table. It is intended for bounded verification services.
func NewReaderWithXRefLimit(f io.ReaderAt, size, limit int64) (*Reader, error) {
	if limit <= 0 || limit > maxObjetosPDF+1 {
		limit = maxObjetosPDF + 1
	}
	return newReaderEncryptedLimited(f, size, nil, limit)
}

func newReaderEncryptedLimited(f io.ReaderAt, size int64, pw func() string, limit int64) (*Reader, error) {
	buf := make([]byte, 10)
	f.ReadAt(buf, 0)
	if (!bytes.HasPrefix(buf, []byte("%PDF-1.")) || buf[7] < '0' || buf[7] > '7') && (!bytes.HasPrefix(buf, []byte("%PDF-2.")) || buf[7] < '0' || buf[7] > '0') {
		return nil, fmt.Errorf("not a PDF file: invalid header")
	}

	version := buf[5:8]

	end := size

	// Some PDF's are quite broken and have a lot of stuff after %%EOF.
	searchSize := int64(200)
	searchSizeRead := int(0)

EOFDetect:
	for {
		buf = make([]byte, searchSize)

		searchSizeRead, _ = f.ReadAt(buf, end-searchSize)
		for len(buf) > 0 && buf[len(buf)-1] == '\n' || buf[len(buf)-1] == '\r' {
			buf = buf[:len(buf)-1]
		}
		buf = bytes.TrimRight(buf, "\r\n\t ")
		for {
			if len(buf) == 5 {
				break
			}

			if bytes.HasSuffix(buf, []byte("%%EOF")) {
				break EOFDetect
			}

			buf = buf[0 : len(buf)-1]
		}

		searchSize += 200

		if searchSize > end {
			return nil, fmt.Errorf("not a PDF file: missing %%%%EOF")
		}
	}

	eofPosition := len(buf)

	// Read 200 bytes before the %%EOF.
	buf = make([]byte, int64(200))
	f.ReadAt(buf, end-(int64(searchSizeRead)-int64(eofPosition))-int64(len(buf)))

	i := findLastLine(buf, "startxref")
	if i < 0 {
		return nil, fmt.Errorf("malformed PDF file: missing final startxref")
	}

	r := &Reader{
		f:               f,
		end:             end,
		xrefLimit:       limit,
		XrefInformation: ReaderXrefInformation{},
		PDFVersion:      string(version),
	}
	pos := (end - (int64(searchSizeRead) - int64(eofPosition)) - int64(len(buf))) + int64(i)

	// Save the position of the startxref element.
	r.XrefInformation.PositionStartPos = pos

	b := newBuffer(io.NewSectionReader(f, pos, end-pos), pos)

	if b.readToken() != keyword("startxref") {
		return nil, fmt.Errorf("malformed PDF file: missing startxref")
	}

	startxref, ok := b.readToken().(int64)
	if !ok {
		return nil, fmt.Errorf("malformed PDF file: startxref not followed by integer")
	}

	// Save length. Useful for calculations later on.
	r.XrefInformation.PositionLength = b.realPos + 1

	// Save end position. Add 1 for the newline character.
	r.XrefInformation.PositionEndPos = r.XrefInformation.PositionStartPos + r.XrefInformation.PositionLength

	// Save start position of xref.
	r.XrefInformation.StartPos = startxref
	if startxref <= 0 || startxref >= end {
		return nil, fmt.Errorf("malformed PDF file: startxref fuera del documento")
	}

	b = newBuffer(io.NewSectionReader(r.f, startxref, r.end-startxref), startxref)
	xref, trailerptr, trailer, err := readXref(r, b)
	if err != nil {
		return nil, err
	}
	r.xref = xref
	r.trailer = trailer
	r.trailerptr = trailerptr
	if trailer["Encrypt"] == nil {
		return r, nil
	}
	err = r.initEncrypt("")
	if err == nil {
		return r, nil
	}
	if pw == nil || err != ErrInvalidPassword {
		return nil, err
	}
	for {
		next := pw()
		if next == "" {
			break
		}
		if r.initEncrypt(next) == nil {
			return r, nil
		}
	}
	return nil, err
}

// Trailer returns the file's Trailer value.
func (r *Reader) Trailer() Value {
	return Value{r, r.trailerptr, r.trailer}
}

func readXref(r *Reader, _ *buffer) ([]xref, objptr, dict, error) {
	return readXrefChain(r, r.XrefInformation.StartPos)
}

// readXrefChain follows /Prev across both xref encodings. A hybrid table's
// /XRefStm belongs to the same revision, so its entries are merged before
// following /Prev; the table wins if both define an object.
func readXrefChain(r *Reader, first int64) ([]xref, objptr, dict, error) {
	var table []xref
	var latest dict
	var latestPtr objptr
	seen := make(map[int64]bool)
	for off, revision := first, 0; ; revision++ {
		if off <= 0 || off >= r.end || seen[off] || revision > 1024 {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev offset or loop")
		}
		seen[off] = true
		b := newBuffer(io.NewSectionReader(r.f, off, r.end-off), off)
		tok := b.readToken()
		var trailer dict
		var ptr objptr
		if tok == keyword("xref") {
			var err error
			table, err = readXrefTableData(b, table, r.xrefLimit)
			if err != nil {
				return nil, objptr{}, nil, err
			}
			afterTable := b.realPos
			var ok bool
			trailer, ok = b.readObject().(dict)
			if !ok {
				return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref trailer missing")
			}
			if revision == 0 {
				trailerLength := int64(len(keyword("trailer"))) + 1
				r.XrefInformation.Type = "table"
				r.XrefInformation.EndPos = off - trailerLength + afterTable
				r.XrefInformation.Length = afterTable - trailerLength + 1
				r.XrefInformation.IncludingTrailerEndPos = off + b.realPos
				r.XrefInformation.IncludingTrailerLength = b.realPos + 1
			}
			if hybrid, exists := trailer["XRefStm"]; exists {
				hybridOff, ok := hybrid.(int64)
				if !ok || hybridOff <= 0 || hybridOff >= off || seen[hybridOff] {
					return nil, objptr{}, nil, fmt.Errorf("malformed PDF: invalid XRefStm")
				}
				seen[hybridOff] = true
				var hybridTrailer dict
				var hybridPtr objptr
				table, hybridPtr, hybridTrailer, err = readOneXrefStream(r, hybridOff, table)
				if err != nil {
					return nil, objptr{}, nil, err
				}
				// In a hybrid revision the table trailer is authoritative for /Size.
				// The xref stream may declare a different size for its own entries.
				for _, key := range []name{"Prev", "Root", "Encrypt", "ID"} {
					if value, present := hybridTrailer[key]; present && objfmt(value) != objfmt(trailer[key]) {
						return nil, objptr{}, nil, fmt.Errorf("malformed PDF: hybrid xref trailer conflict for /%s", key)
					}
				}
				_ = hybridPtr
			}
		} else if _, ok := tok.(int64); ok {
			var err error
			table, ptr, trailer, err = readOneXrefStream(r, off, table)
			if err != nil {
				return nil, objptr{}, nil, err
			}
			if revision == 0 {
				r.XrefInformation.Type = "stream"
			}
		} else {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: cross-reference not found")
		}
		if revision == 0 {
			latest, latestPtr = trailer, ptr
		}
		prev, exists := trailer["Prev"]
		if !exists {
			break
		}
		var ok bool
		off, ok = prev.(int64)
		if !ok {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev is not integer")
		}
	}
	size, ok := latest["Size"].(int64)
	if !ok || size <= 0 || size > r.xrefLimit {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref Size fuera de límite")
	}
	if int64(len(table)) > size {
		table = table[:size]
	}
	r.XrefInformation.ItemCount = int64(len(table))
	return table, latestPtr, latest, nil
}

func readOneXrefStream(r *Reader, off int64, table []xref) ([]xref, objptr, dict, error) {
	b := newBuffer(io.NewSectionReader(r.f, off, r.end-off), off)
	obj, ok := b.readObject().(objdef)
	if !ok {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref stream object missing")
	}
	strm, ok := obj.obj.(stream)
	if !ok || strm.hdr["Type"] != name("XRef") {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref stream type missing")
	}
	size, ok := strm.hdr["Size"].(int64)
	if !ok || size <= 0 || size > r.xrefLimit {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref stream Size fuera de límite")
	}
	if int64(len(table)) < size {
		table = append(table, make([]xref, size-int64(len(table)))...)
	}
	var err error
	table, err = readXrefStreamData(r, strm, table, size)
	if err != nil {
		return nil, objptr{}, nil, err
	}
	return table, obj.ptr, strm.hdr, nil
}

func readXrefStream(r *Reader, b *buffer) ([]xref, objptr, dict, error) {
	obj1 := b.readObject()
	obj, ok := obj1.(objdef)
	if !ok {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: cross-reference table not found: %v", objfmt(obj1))
	}
	strmptr := obj.ptr
	strm, ok := obj.obj.(stream)
	if !ok {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: cross-reference table not found: %v", objfmt(obj))
	}
	if strm.hdr["Type"] != name("XRef") {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref stream does not have type XRef")
	}
	size, ok := strm.hdr["Size"].(int64)
	if !ok {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref stream missing Size")
	}
	if size <= 0 || size > r.xrefLimit {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref stream Size fuera de limite")
	}

	table := make([]xref, size)

	table, err := readXrefStreamData(r, strm, table, size)
	if err != nil {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: %v", err)
	}

	seenPrev := map[int64]bool{}

	for prevoff := strm.hdr["Prev"]; prevoff != nil; {
		off, ok := prevoff.(int64)
		if !ok {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev is not integer: %v", prevoff)
		}

		if _, ok := seenPrev[off]; ok {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev loop detected: %v", off)
		}

		seenPrev[off] = true

		b := newBuffer(io.NewSectionReader(r.f, off, r.end-off), off)
		obj1 := b.readObject()
		obj, ok := obj1.(objdef)
		if !ok {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref prev stream not found: %v", objfmt(obj1))
		}
		prevstrm, ok := obj.obj.(stream)
		if !ok {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref prev stream not found: %v", objfmt(obj))
		}
		prevoff = prevstrm.hdr["Prev"]
		prev := Value{r, objptr{}, prevstrm}
		if prev.Kind() != Stream {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref prev stream is not stream: %v", prev)
		}
		if prev.Key("Type").Name() != "XRef" {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref prev stream does not have type XRef")
		}
		psize := prev.Key("Size").Int64()
		if psize > size {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref prev stream larger than last stream")
		}
		if table, err = readXrefStreamData(r, prev.data.(stream), table, psize); err != nil {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: reading xref prev stream: %v", err)
		}
	}

	// Save the xref type. Useful for adding data to it.
	r.XrefInformation.Type = "stream"
	r.XrefInformation.ItemCount = size

	r.XrefInformation.ItemCount = int64(len(table))

	return table, strmptr, strm.hdr, nil
}

func readXrefStreamData(r *Reader, strm stream, table []xref, size int64) ([]xref, error) {
	index, _ := strm.hdr["Index"].(array)
	if index == nil {
		index = array{int64(0), size}
	}
	if len(index)%2 != 0 {
		return nil, fmt.Errorf("invalid Index array %v", objfmt(index))
	}
	ww, ok := strm.hdr["W"].(array)
	if !ok {
		return nil, fmt.Errorf("xref stream missing W array")
	}

	var w []int
	for _, x := range ww {
		i, ok := x.(int64)
		// AutoFirmaV2: cada campo ocupa como mucho 8 bytes (decodeInt).
		if !ok || i < 0 || i > 8 {
			return nil, fmt.Errorf("invalid W array %v", objfmt(ww))
		}
		w = append(w, int(i))
	}
	if len(w) < 3 {
		return nil, fmt.Errorf("invalid W array %v", objfmt(ww))
	}

	v := Value{r, objptr{}, strm}
	wtotal := 0
	for _, wid := range w {
		wtotal += wid
	}
	buf := make([]byte, wtotal)
	data := v.Reader()
	var totalEntries int64
	for len(index) > 0 {
		start, ok1 := index[0].(int64)
		n, ok2 := index[1].(int64)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("malformed Index pair %v %v %T %T", objfmt(index[0]), objfmt(index[1]), index[0], index[1])
		}
		if err := subseccionXrefValida(start, n); err != nil {
			return nil, err
		}
		if n > size-totalEntries {
			return nil, fmt.Errorf("xref stream Index excede el limite de entradas")
		}
		totalEntries += n
		if start+n > size {
			return nil, fmt.Errorf("malformed xref stream Index fuera de Size")
		}
		index = index[2:]
		for i := 0; i < int(n); i++ {
			_, err := io.ReadFull(data, buf)
			if err != nil {
				return nil, fmt.Errorf("error reading xref stream: %v", err)
			}

			v1 := decodeInt(buf[0:w[0]])
			if w[0] == 0 {
				v1 = 1
			}

			v2 := decodeInt(buf[w[0] : w[0]+w[1]])
			v3 := decodeInt(buf[w[0]+w[1] : w[0]+w[1]+w[2]])
			x := int(start) + i
			if x >= len(table) {
				return nil, fmt.Errorf("malformed xref stream Index fuera de tabla")
			}
			if table[x].defined {
				continue
			}
			switch v1 {
			case 0:
				if v3 < 0 || v3 > math.MaxUint16 {
					return nil, fmt.Errorf("malformed xref stream free generation %d", v3)
				}
				table[x] = xref{ptr: objptr{uint32(x), uint16(v3)}, defined: true, free: true} // #nosec G115 -- x y v3 en rango.
			case 1:
				if v3 < 0 || v3 > math.MaxUint16 {
					return nil, fmt.Errorf("malformed xref stream generation %d", v3)
				}
				table[x] = xref{ptr: objptr{uint32(x), uint16(v3)}, offset: int64(v2), defined: true} // #nosec G115 -- x y v3 en rango.
			case 2:
				if v2 < 0 || v2 > maxObjetosPDF {
					return nil, fmt.Errorf("malformed xref stream object %d", v2)
				}
				table[x] = xref{ptr: objptr{uint32(x), 0}, inStream: true, stream: objptr{uint32(v2), 0}, offset: int64(v3), defined: true} // #nosec G115 -- x y v2 en rango.
			default:
				return nil, fmt.Errorf("invalid xref stream type %d", v1)
			}
		}
	}
	return table, nil
}

func decodeInt(b []byte) int {
	x := 0
	for _, c := range b {
		x = x<<8 | int(c)
	}
	return x
}

func readXrefTable(r *Reader, b *buffer) ([]xref, objptr, dict, error) {
	var table []xref

	table, err := readXrefTableData(b, table, r.xrefLimit)
	if err != nil {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: %v", err)
	}

	// Get length of trailer keyword and newline.
	trailer_length := int64(len(keyword("trailer"))) + 1

	// Save end position.
	r.XrefInformation.EndPos = (r.XrefInformation.StartPos - trailer_length) + b.realPos

	// Save length position. Useful for calculations. Remove trailer keyword length, add 1 for newline.
	r.XrefInformation.Length = (b.realPos - trailer_length) + 1

	trailer, ok := b.readObject().(dict)
	if !ok {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref table not followed by trailer dictionary")
	}
	latestTrailer := trailer

	seenPrev := map[int64]bool{}

	for prevoff := trailer["Prev"]; prevoff != nil; {
		off, ok := prevoff.(int64)
		if !ok {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev is not integer: %v", prevoff)
		}

		if _, ok := seenPrev[off]; ok {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev loop detected: %v", off)
		}

		seenPrev[off] = true

		b := newBuffer(io.NewSectionReader(r.f, off, r.end-off), off)
		tok := b.readToken()
		if tok != keyword("xref") {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev does not point to xref")
		}
		table, err = readXrefTableData(b, table, r.xrefLimit)
		if err != nil {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: %v", err)
		}

		trailer, ok := b.readObject().(dict)
		if !ok {
			return nil, objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev table not followed by trailer dictionary")
		}
		prevoff = trailer["Prev"]
	}

	size, ok := latestTrailer[name("Size")].(int64)
	if !ok {
		return nil, objptr{}, nil, fmt.Errorf("malformed PDF: trailer missing /Size entry")
	}

	if size < int64(len(table)) {
		table = table[:size]
	}

	// Save the xref type. Useful for adding data to it.
	r.XrefInformation.Type = "table"

	// Save the amount of items in the table. Useful for generating a new id for the signature.
	r.XrefInformation.ItemCount = int64(len(table))

	// Save end position. Note that this is including the trailer and startxref (without value).
	r.XrefInformation.IncludingTrailerEndPos = r.XrefInformation.StartPos + b.realPos

	// Save length position. Useful for calculations.
	r.XrefInformation.IncludingTrailerLength = b.realPos + 1

	return table, objptr{}, latestTrailer, nil
}

func readXrefTableData(b *buffer, table []xref, limit int64) ([]xref, error) {
	for {
		tok := b.readToken()
		if tok == keyword("trailer") {
			break
		}
		start, ok1 := tok.(int64)
		n, ok2 := b.readToken().(int64)

		if !ok1 || !ok2 {
			return nil, fmt.Errorf("malformed xref table")
		}
		if err := subseccionXrefValida(start, n); err != nil {
			return nil, err
		}
		if start+n > limit {
			return nil, fmt.Errorf("xref supera el limite de objetos")
		}
		for i := 0; i < int(n); i++ {
			off, ok1 := b.readToken().(int64)
			gen, ok2 := b.readToken().(int64)
			alloc, ok3 := b.readToken().(keyword)
			if !ok1 || !ok2 || !ok3 || alloc != keyword("f") && alloc != keyword("n") {
				return nil, fmt.Errorf("malformed xref table")
			}
			x := int(start) + i
			for cap(table) <= x {
				table = append(table[:cap(table)], xref{})
			}
			if len(table) <= x {
				table = table[:x+1]
			}
			if gen < 0 || gen > math.MaxUint16 {
				return nil, fmt.Errorf("malformed xref table generation %d", gen)
			}
			if !table[x].defined {
				if alloc == "n" {
					table[x] = xref{ptr: objptr{uint32(x), uint16(gen)}, offset: int64(off), defined: true} // #nosec G115 -- x y gen en rango.
				} else {
					table[x] = xref{ptr: objptr{uint32(x), uint16(gen)}, defined: true, free: true} // #nosec G115 -- x y gen en rango.
				}
			}
		}
	}
	return table, nil
}

func findLastLine(buf []byte, s string) int {
	bs := []byte(s)
	max := len(buf)
	for {
		i := bytes.LastIndex(buf[:max], bs)
		if i <= 0 || i+len(bs) >= len(buf) {
			return -1
		}
		if (buf[i-1] == '\n' || buf[i-1] == '\r') && (buf[i+len(bs)] == '\n' || buf[i+len(bs)] == '\r') {
			return i
		}
		max = i
	}
}

// A Value is a single PDF value, such as an integer, dictionary, or array.
// The zero Value is a PDF null (Kind() == Null, IsNull() = true).
type Value struct {
	r    *Reader
	ptr  objptr
	data interface{}
}

// IsNull reports whether the value is a null. It is equivalent to Kind() == Null.
func (v Value) IsNull() bool {
	return v.data == nil
}

func (v Value) RawData() interface{} {
	return v.data
}

// A ValueKind specifies the kind of data underlying a Value.
type ValueKind int

// The PDF value kinds.
const (
	Null ValueKind = iota
	Bool
	Integer
	Real
	String
	Name
	Dict
	Array
	Stream
)

// Kind reports the kind of value underlying v.
func (v Value) Kind() ValueKind {
	switch v.data.(type) {
	default:
		return Null
	case bool:
		return Bool
	case int64:
		return Integer
	case float64:
		return Real
	case string:
		return String
	case name:
		return Name
	case dict:
		return Dict
	case array:
		return Array
	case stream:
		return Stream
	}
}

// String returns a textual representation of the value v.
// Note that String is not the accessor for values with Kind() == String.
// To access such values, see RawString, Text, and TextFromUTF16.
func (v Value) String() string {
	return objfmt(v.data)
}

func objfmt(x interface{}) string {
	switch x := x.(type) {
	default:
		return fmt.Sprint(x)
	case string:
		if isPDFDocEncoded(x) {
			return strconv.Quote(pdfDocDecode(x))
		}
		if isUTF16(x) {
			return strconv.Quote(utf16Decode(x[2:]))
		}
		return strconv.Quote(x)
	case name:
		return "/" + string(x)
	case dict:
		var keys []string
		for k := range x {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)
		var buf bytes.Buffer
		buf.WriteString("<<")
		for i, k := range keys {
			elem := x[name(k)]
			if i > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString("/")
			buf.WriteString(k)
			buf.WriteString(" ")
			buf.WriteString(objfmt(elem))
		}
		buf.WriteString(">>")
		return buf.String()

	case array:
		var buf bytes.Buffer
		buf.WriteString("[")
		for i, elem := range x {
			if i > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString(objfmt(elem))
		}
		buf.WriteString("]")
		return buf.String()

	case stream:
		return fmt.Sprintf("%v@%d", objfmt(x.hdr), x.offset)

	case objptr:
		return fmt.Sprintf("%d %d R", x.id, x.gen)

	case objdef:
		return fmt.Sprintf("{%d %d obj}%v", x.ptr.id, x.ptr.gen, objfmt(x.obj))
	}
}

// Bool returns v's boolean value.
// If v.Kind() != Bool, Bool returns false.
func (v Value) Bool() bool {
	x, ok := v.data.(bool)
	if !ok {
		return false
	}
	return x
}

// Int64 returns v's int64 value.
// If v.Kind() != Int64, Int64 returns 0.
func (v Value) Int64() int64 {
	x, ok := v.data.(int64)
	if !ok {
		return 0
	}
	return x
}

// Float64 returns v's float64 value, converting from integer if necessary.
// If v.Kind() != Float64 and v.Kind() != Int64, Float64 returns 0.
func (v Value) Float64() float64 {
	x, ok := v.data.(float64)
	if !ok {
		x, ok := v.data.(int64)
		if ok {
			return float64(x)
		}
		return 0
	}
	return x
}

// RawString returns v's string value.
// If v.Kind() != String, RawString returns the empty string.
func (v Value) RawString() string {
	x, ok := v.data.(string)
	if !ok {
		return ""
	}
	return x
}

// Text returns v's string value interpreted as a “text string” (defined in the PDF spec)
// and converted to UTF-8.
// If v.Kind() != String, Text returns the empty string.
func (v Value) Text() string {
	x, ok := v.data.(string)
	if !ok {
		return ""
	}
	if isPDFDocEncoded(x) {
		return pdfDocDecode(x)
	}
	if isUTF16(x) {
		return utf16Decode(x[2:])
	}
	return x
}

// TextFromUTF16 returns v's string value interpreted as big-endian UTF-16
// and then converted to UTF-8.
// If v.Kind() != String or if the data is not valid UTF-16, TextFromUTF16 returns
// the empty string.
func (v Value) TextFromUTF16() string {
	x, ok := v.data.(string)
	if !ok {
		return ""
	}
	if len(x)%2 == 1 {
		return ""
	}
	if x == "" {
		return ""
	}
	return utf16Decode(x)
}

// Name returns v's name value.
// If v.Kind() != Name, Name returns the empty string.
// The returned name does not include the leading slash:
// if v corresponds to the name written using the syntax /Helvetica,
// Name() == "Helvetica".
func (v Value) Name() string {
	x, ok := v.data.(name)
	if !ok {
		return ""
	}
	return string(x)
}

// Key returns the value associated with the given name key in the dictionary v.
// Like the result of the Name method, the key should not include a leading slash.
// If v is a stream, Key applies to the stream's header dictionary.
// If v.Kind() != Dict and v.Kind() != Stream, Key returns a null Value.
func (v Value) Key(key string) Value {
	x, ok := v.data.(dict)
	if !ok {
		strm, ok := v.data.(stream)
		if !ok {
			return Value{}
		}
		x = strm.hdr
	}
	return v.r.resolve(v.ptr, x[name(key)])
}

func (v Value) GetPtr() objptr {
	return v.ptr
}

// ObjectReference identifies the indirect object from which this resolved
// value was read. Zero means an inline value or the trailer.
func (v Value) ObjectReference() (number uint32, generation uint16) {
	return v.ptr.id, v.ptr.gen
}

// Keys returns a sorted list of the keys in the dictionary v.
// If v is a stream, Keys applies to the stream's header dictionary.
// If v.Kind() != Dict and v.Kind() != Stream, Keys returns nil.
func (v Value) Keys() []string {
	x, ok := v.data.(dict)
	if !ok {
		strm, ok := v.data.(stream)
		if !ok {
			return nil
		}
		x = strm.hdr
	}
	keys := []string{} // not nil
	for k := range x {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	return keys
}

// Index returns the i'th element in the array v.
// If v.Kind() != Array or if i is outside the array bounds,
// Index returns a null Value.
func (v Value) Index(i int) Value {
	x, ok := v.data.(array)
	if !ok || i < 0 || i >= len(x) {
		return Value{}
	}
	return v.r.resolve(v.ptr, x[i])
}

// Len returns the length of the array v.
// If v.Kind() != Array, Len returns 0.
func (v Value) Len() int {
	x, ok := v.data.(array)
	if !ok {
		return 0
	}
	return len(x)
}

func (r *Reader) Resolve(parent objptr, x interface{}) Value {
	return r.resolve(parent, x)
}

func (r *Reader) resolve(parent objptr, x interface{}) Value {
	if ptr, ok := x.(objptr); ok {
		if int64(ptr.id) >= int64(len(r.xref)) {
			return Value{}
		}
		xref := r.xref[ptr.id]
		if xref.ptr != ptr || !xref.inStream && xref.offset == 0 {
			return Value{}
		}
		var obj object
		if xref.inStream {
			if r.resolviendoObjStm.Add(1) > maxAnidamientoObjStm {
				r.resolviendoObjStm.Add(-1)
				panic(fmt.Errorf("malformed PDF: flujos de objetos recursivos al cargar %v", ptr))
			}
			defer r.resolviendoObjStm.Add(-1)
			strm := r.resolve(parent, xref.stream)
			saltos := 0
		Search:
			for {
				if saltos++; saltos > maxCadenaExtends {
					panic(fmt.Errorf("malformed PDF: cadena /Extends demasiado larga al cargar %v", ptr))
				}
				if strm.Kind() != Stream {
					panic("not a stream")
				}
				if strm.Key("Type").Name() != "ObjStm" {
					panic("not an object stream")
				}
				n := int(strm.Key("N").Int64())
				first := strm.Key("First").Int64()
				if first == 0 {
					panic("missing First")
				}
				b := newBuffer(strm.Reader(), 0)
				b.allowEOF = true
				for i := 0; i < n; i++ {
					id, _ := b.readToken().(int64)
					off, _ := b.readToken().(int64)
					// AutoFirmaV2: identificadores fuera de rango no se truncan.
					if id >= 0 && id <= math.MaxUint32 && uint32(id) == ptr.id { // #nosec G115 -- rango comprobado.
						b.seekForward(first + off)
						x = b.readObject()
						break Search
					}
				}
				ext := strm.Key("Extends")
				if ext.Kind() != Stream {
					panic("cannot find object in stream")
				}
				strm = ext
			}
		} else {
			b := newBuffer(io.NewSectionReader(r.f, xref.offset, r.end-xref.offset), xref.offset)
			// AutoFirmaV2: el diccionario /Encrypt nunca está cifrado (§7.6.1).
			if enc, ok := r.trailer["Encrypt"].(objptr); !ok || enc != ptr {
				b.key = r.key
				b.useAES = r.useAES
			}

			obj = b.readObject()
			def, ok := obj.(objdef)
			if !ok {
				panic(fmt.Errorf("loading %v: found %T instead of objdef", ptr, obj))
				//return Value{}
			}
			if def.ptr != ptr {
				panic(fmt.Errorf("loading %v: found %v", ptr, def.ptr))
			}
			x = def.obj
		}
		parent = ptr
	}

	switch x := x.(type) {
	case nil, bool, int64, float64, name, dict, array, stream:
		return Value{r, parent, x}
	case string:
		return Value{r, parent, x}
	default:
		panic(fmt.Errorf("unexpected value type %T in resolve", x))
	}
}

type errorReadCloser struct {
	err error
}

func (e *errorReadCloser) Read([]byte) (int, error) {
	return 0, e.err
}

func (e *errorReadCloser) Close() error {
	return e.err
}

// Reader returns the data contained in the stream v.
// If v.Kind() != Stream, Reader returns a ReadCloser that
// responds to all reads with a “stream not present” error.
func (v Value) Reader() io.ReadCloser {
	x, ok := v.data.(stream)
	if !ok {
		return &errorReadCloser{fmt.Errorf("stream not present")}
	}
	var rd io.Reader
	rd = io.NewSectionReader(v.r.f, x.offset, v.Key("Length").Int64())
	if v.r.key != nil {
		rd = decryptStream(v.r.key, v.r.useAES, x.ptr, rd)
	}
	filter := v.Key("Filter")
	param := v.Key("DecodeParms")
	switch filter.Kind() {
	default:
		panic(fmt.Errorf("unsupported filter %v", filter))
	case Null:
		// ok
	case Name:
		rd = applyFilter(rd, filter.Name(), param)
	case Array:
		for i := 0; i < filter.Len(); i++ {
			rd = applyFilter(rd, filter.Index(i).Name(), param.Index(i))
		}
	}

	return ioutil.NopCloser(rd)
}

func applyFilter(rd io.Reader, name string, param Value) io.Reader {
	switch name {
	default:
		panic("unknown filter " + name)
	case "FlateDecode":
		zr, err := zlib.NewReader(rd)
		if err != nil {
			panic(err)
		}
		pred := param.Key("Predictor")
		if pred.Kind() == Null {
			return zr
		}
		columns := param.Key("Columns").Int64()
		switch pred.Int64() {
		default:
			fmt.Println("unknown predictor", pred)
			panic("pred")
		case 12:
			return &pngUpReader{r: zr, hist: make([]byte, 1+columns), tmp: make([]byte, 1+columns)}
		}
	}
}

type pngUpReader struct {
	r    io.Reader
	hist []byte
	tmp  []byte
	pend []byte
}

func (r *pngUpReader) Read(b []byte) (int, error) {
	n := 0
	for len(b) > 0 {
		if len(r.pend) > 0 {
			m := copy(b, r.pend)
			n += m
			b = b[m:]
			r.pend = r.pend[m:]
			continue
		}
		_, err := io.ReadFull(r.r, r.tmp)
		if err != nil {
			return n, err
		}
		if r.tmp[0] != 2 {
			return n, fmt.Errorf("malformed PNG-Up encoding")
		}
		for i, b := range r.tmp {
			r.hist[i] += b
		}
		r.pend = r.hist[1:]
	}
	return n, nil
}

var passwordPad = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

func (r *Reader) initEncrypt(password string) error {
	// See PDF 32000-1:2008, §7.6, and ISO 32000-2 for V=5 (AES-256).
	encrypt, _ := r.resolve(objptr{}, r.trailer["Encrypt"]).data.(dict)
	if encrypt["Filter"] != name("Standard") {
		return fmt.Errorf("unsupported PDF: encryption filter %v", objfmt(encrypt["Filter"]))
	}
	V, _ := encrypt["V"].(int64)
	R, _ := encrypt["R"].(int64)
	p, _ := encrypt["P"].(int64)
	// AutoFirmaV2: parámetros de cifrado para el firmante.
	r.cifrado = ParametrosCifrado{Presente: true, V: int(V), R: int(R), P: int32(p)} // #nosec G115 -- /P es un entero de 32 bits en la norma.
	if V == 5 {
		return r.initEncryptV5(encrypt, password)
	}
	n, _ := encrypt["Length"].(int64)
	if n == 0 {
		n = 40
	}
	if n%8 != 0 || n > 128 || n < 40 {
		return fmt.Errorf("malformed PDF: %d-bit encryption key", n)
	}
	if V != 1 && V != 2 && (V != 4 || !okayV4(encrypt)) {
		return fmt.Errorf("unsupported PDF: encryption version V=%d; %v", V, objfmt(encrypt))
	}
	if V == 4 {
		n = 128
	}

	ids, ok := r.trailer["ID"].(array)
	if !ok || len(ids) < 1 {
		return fmt.Errorf("malformed PDF: missing ID in trailer")
	}
	idstr, ok := ids[0].(string)
	if !ok {
		return fmt.Errorf("malformed PDF: missing ID in trailer")
	}
	ID := []byte(idstr)

	if R < 2 {
		return fmt.Errorf("malformed PDF: encryption revision R=%d", R)
	}
	if R > 4 {
		return fmt.Errorf("unsupported PDF: encryption revision R=%d", R)
	}
	O, _ := encrypt["O"].(string)
	U, _ := encrypt["U"].(string)
	if len(O) < 32 || len(U) < 32 {
		return fmt.Errorf("malformed PDF: missing O= or U= encryption parameters")
	}
	O, U = O[:32], U[:32]
	P := uint32(p) // #nosec G115 -- conversión de bits de permisos.

	// AutoFirmaV2: se admite la contraseña de usuario y la de propietario.
	if key, ok := claveUsuarioV4(password, O, U, P, ID, n, R); ok {
		r.key, r.useAES = key, V == 4
		r.cifrado.Clave, r.cifrado.AES = key, V == 4
		r.cifrado.Propietario = password != "" && esPropietarioV4(password, O, U, P, ID, n, R)
		return nil
	}
	if usuario, ok := usuarioDesdePropietarioV4(password, O, n, R); ok {
		if key, ok := claveUsuarioV4(string(usuario), O, U, P, ID, n, R); ok {
			r.key, r.useAES = key, V == 4
			r.cifrado.Clave, r.cifrado.AES, r.cifrado.Propietario = key, V == 4, true
			return nil
		}
	}
	return ErrInvalidPassword
}

// claveUsuarioV4 calcula la clave del fichero con una contraseña de usuario
// (algoritmos 2, 4 y 5) y comprueba que coincide con U.
func claveUsuarioV4(password, O, U string, P uint32, ID []byte, n, R int64) ([]byte, bool) {
	key := claveFicheroV4(rellenarContrasena(password), O, P, ID, n, R)
	c, err := rc4.NewCipher(key)
	if err != nil {
		return nil, false
	}
	var u []byte
	if R == 2 {
		u = make([]byte, 32)
		copy(u, passwordPad)
		c.XORKeyStream(u, u)
	} else {
		h := md5.New()
		h.Write(passwordPad)
		h.Write(ID)
		u = h.Sum(nil)
		c.XORKeyStream(u, u)
		for i := 1; i <= 19; i++ {
			key1 := make([]byte, len(key))
			copy(key1, key)
			for j := range key1 {
				key1[j] ^= byte(i)
			}
			c, _ = rc4.NewCipher(key1)
			c.XORKeyStream(u, u)
		}
	}
	if !bytes.HasPrefix([]byte(U), u) {
		return nil, false
	}
	return key, true
}

func rellenarContrasena(password string) []byte {
	// TODO: Password should be converted to Latin-1.
	pw := []byte(password)
	out := make([]byte, 32)
	if len(pw) >= 32 {
		copy(out, pw[:32])
	} else {
		copy(out, pw)
		copy(out[len(pw):], passwordPad[:32-len(pw)])
	}
	return out
}

func claveFicheroV4(pw32 []byte, O string, P uint32, ID []byte, n, R int64) []byte {
	h := md5.New()
	h.Write(pw32)
	h.Write([]byte(O))
	h.Write(binary.LittleEndian.AppendUint32(nil, P))
	h.Write(ID)
	key := h.Sum(nil)
	if R >= 3 {
		for i := 0; i < 50; i++ {
			h.Reset()
			h.Write(key[:n/8])
			key = h.Sum(key[:0])
		}
		return key[:n/8]
	}
	return key[:40/8]
}

// claveRC4Propietario es la clave con la que se cifra O (algoritmo 3).
func claveRC4Propietario(password string, n, R int64) []byte {
	sum := md5.Sum(rellenarContrasena(password))
	key := sum[:]
	if R >= 3 {
		for i := 0; i < 50; i++ {
			s := md5.Sum(key)
			key = s[:]
		}
		return key[:n/8]
	}
	return key[:5]
}

// usuarioDesdePropietarioV4 recupera la contraseña de usuario rellenada a
// partir de la de propietario (algoritmo 7).
func usuarioDesdePropietarioV4(password, O string, n, R int64) ([]byte, bool) {
	if password == "" {
		return nil, false
	}
	key := claveRC4Propietario(password, n, R)
	u := []byte(O)
	if R == 2 {
		c, err := rc4.NewCipher(key)
		if err != nil {
			return nil, false
		}
		c.XORKeyStream(u, u)
		return u, true
	}
	for i := 19; i >= 0; i-- {
		key1 := make([]byte, len(key))
		for j := range key {
			key1[j] = key[j] ^ byte(i)
		}
		c, err := rc4.NewCipher(key1)
		if err != nil {
			return nil, false
		}
		c.XORKeyStream(u, u)
	}
	return u, true
}

func esPropietarioV4(password, O, U string, P uint32, ID []byte, n, R int64) bool {
	usuario, ok := usuarioDesdePropietarioV4(password, O, n, R)
	if !ok {
		return false
	}
	_, ok = claveUsuarioV4(string(usuario), O, U, P, ID, n, R)
	return ok
}

// initEncryptV5 abre documentos con AES-256 (V=5, R=5 o 6).
func (r *Reader) initEncryptV5(encrypt dict, password string) error {
	R, _ := encrypt["R"].(int64)
	if R != 5 && R != 6 {
		return fmt.Errorf("unsupported PDF: encryption revision R=%d", R)
	}
	cf, _ := encrypt["CF"].(dict)
	stmf, _ := encrypt["StmF"].(name)
	strf, _ := encrypt["StrF"].(name)
	param, _ := cf[stmf].(dict)
	if stmf != strf || param["CFM"] != name("AESV3") {
		return fmt.Errorf("unsupported PDF: AES-256 crypt filter %v", objfmt(encrypt))
	}
	O, _ := encrypt["O"].(string)
	U, _ := encrypt["U"].(string)
	OE, _ := encrypt["OE"].(string)
	UE, _ := encrypt["UE"].(string)
	if len(O) < 48 || len(U) < 48 || len(OE) != 32 || len(UE) != 32 {
		return fmt.Errorf("malformed PDF: missing AES-256 encryption parameters")
	}
	pw := []byte(password)
	if len(pw) > 127 {
		pw = pw[:127]
	}
	o, u := []byte(O[:48]), []byte(U[:48])
	var intermedia, cifrada []byte
	propietario := false
	switch {
	case bytes.Equal(hashV5(pw, o[32:40], u, R), o[:32]):
		intermedia, cifrada, propietario = hashV5(pw, o[40:48], u, R), []byte(OE), true
	case bytes.Equal(hashV5(pw, u[32:40], nil, R), u[:32]):
		intermedia, cifrada = hashV5(pw, u[40:48], nil, R), []byte(UE)
	default:
		return ErrInvalidPassword
	}
	block, err := aes.NewCipher(intermedia)
	if err != nil {
		return err
	}
	key := make([]byte, 32)
	cipher.NewCBCDecrypter(block, make([]byte, 16)).CryptBlocks(key, cifrada)
	r.key, r.useAES = key, true
	r.cifrado.Clave, r.cifrado.AES, r.cifrado.AES256, r.cifrado.Propietario = key, true, true, propietario
	return nil
}

// hashV5 es el algoritmo 2.B de ISO 32000-2 (R=6) o SHA-256 simple (R=5).
func hashV5(password, sal, udata []byte, R int64) []byte {
	entrada := append(append(append([]byte{}, password...), sal...), udata...)
	suma := sha256.Sum256(entrada)
	K := suma[:]
	if R == 5 {
		return K
	}
	var E []byte
	for i := 0; i < 64 || int(E[len(E)-1]) > i-32; i++ {
		bloque := append(append(append([]byte{}, password...), K...), udata...)
		K1 := bytes.Repeat(bloque, 64)
		c, _ := aes.NewCipher(K[:16])
		E = make([]byte, len(K1))
		cipher.NewCBCEncrypter(c, K[16:32]).CryptBlocks(E, K1)
		resto := 0
		for _, b := range E[:16] {
			resto += int(b)
		}
		switch resto % 3 {
		case 0:
			s := sha256.Sum256(E)
			K = s[:]
		case 1:
			s := sha512.Sum384(E)
			K = s[:]
		default:
			s := sha512.Sum512(E)
			K = s[:]
		}
	}
	return K[:32]
}

// ParametrosCifrado describe el cifrado del documento (AutoFirmaV2).
type ParametrosCifrado struct {
	Presente bool
	V, R     int
	P        int32
	// Clave del fichero; en AES-256 se usa tal cual para todos los objetos.
	Clave  []byte
	AES    bool
	AES256 bool
	// Propietario indica que se abrió con la contraseña de propietario.
	Propietario bool
}

// Cifrado devuelve los parámetros de cifrado del documento abierto.
func (r *Reader) Cifrado() ParametrosCifrado {
	out := r.cifrado
	out.Clave = append([]byte(nil), r.cifrado.Clave...)
	return out
}

// NewReaderConContrasena abre un PDF cifrado con una contraseña de usuario o
// de propietario (AutoFirmaV2). Si el documento se abre sin contraseña, la
// indicada se comprueba igualmente: la de propietario da todos los permisos.
func NewReaderConContrasena(f io.ReaderAt, size int64, password string) (*Reader, error) {
	usada := false
	r, err := NewReaderEncrypted(f, size, func() string {
		if usada {
			return ""
		}
		usada = true
		return password
	})
	if err != nil || password == "" || !r.cifrado.Presente || r.cifrado.Propietario {
		return r, err
	}
	cifrado, key, useAES := r.cifrado, r.key, r.useAES
	if r.initEncrypt(password) != nil {
		r.cifrado, r.key, r.useAES = cifrado, key, useAES
	}
	return r, nil
}

// maxObjetosPDF es el mayor número de objeto que admite un PDF (el límite de
// implementación de ISO 32000-1, anexo C). AutoFirmaV2: un documento
// malicioso con subsecciones de xref enormes o negativas agotaba la memoria o
// abortaba con un pánico.
const maxObjetosPDF = 8388607

func subseccionXrefValida(start, n int64) error {
	if start < 0 || n < 0 || start > maxObjetosPDF || n > maxObjetosPDF-start+1 {
		return fmt.Errorf("malformed xref subsection %d %d", start, n)
	}
	return nil
}

var ErrInvalidPassword = fmt.Errorf("encrypted PDF: invalid password")

func okayV4(encrypt dict) bool {
	cf, ok := encrypt["CF"].(dict)
	if !ok {
		return false
	}
	stmf, ok := encrypt["StmF"].(name)
	if !ok {
		return false
	}
	strf, ok := encrypt["StrF"].(name)
	if !ok {
		return false
	}
	if stmf != strf {
		return false
	}
	cfparam, ok := cf[stmf].(dict)
	if !ok {
		return false
	}
	if cfparam["AuthEvent"] != nil && cfparam["AuthEvent"] != name("DocOpen") {
		return false
	}
	if cfparam["Length"] != nil && cfparam["Length"] != int64(16) {
		return false
	}
	if cfparam["CFM"] != name("AESV2") {
		return false
	}
	return true
}

func cryptKey(key []byte, useAES bool, ptr objptr) []byte {
	if len(key) == 32 {
		// AES-256 (V=5): la clave del fichero se usa para todos los objetos.
		return key
	}
	h := md5.New()
	h.Write(key)
	// AutoFirmaV2: 3 bytes del número de objeto y 2 de la generación (§7.6.2).
	h.Write(binary.LittleEndian.AppendUint32(nil, ptr.id)[:3])
	h.Write(binary.LittleEndian.AppendUint16(nil, ptr.gen))
	if useAES {
		h.Write([]byte("sAlT"))
	}
	// AutoFirmaV2: la clave de objeto tiene n+5 bytes, como máximo 16 (§7.6.2).
	clave := h.Sum(nil)
	if n := len(key) + 5; n < len(clave) {
		clave = clave[:n]
	}
	return clave
}

func decryptString(key []byte, useAES bool, ptr objptr, x string) string {
	key = cryptKey(key, useAES, ptr)
	if useAES {
		// AutoFirmaV2: AES-CBC con el vector inicial en los 16 primeros bytes.
		plano, err := descifrarAES(key, []byte(x))
		if err != nil {
			return ""
		}
		return string(plano)
	}
	c, _ := rc4.NewCipher(key)
	data := []byte(x)
	c.XORKeyStream(data, data)
	return string(data)
}

func decryptStream(key []byte, useAES bool, ptr objptr, rd io.Reader) io.Reader {
	key = cryptKey(key, useAES, ptr)
	if useAES {
		// AutoFirmaV2: se descifra entero para quitar el relleno PKCS#7.
		cifrado, err := io.ReadAll(rd)
		if err != nil {
			return &errorReadCloser{err}
		}
		plano, err := descifrarAES(key, cifrado)
		if err != nil {
			return &errorReadCloser{err}
		}
		return bytes.NewReader(plano)
	}
	c, _ := rc4.NewCipher(key)
	return &cipher.StreamReader{S: c, R: rd}
}

// descifrarAES descifra datos AES-CBC con el IV delante y relleno PKCS#7.
func descifrarAES(key, datos []byte) ([]byte, error) {
	if len(datos) < 32 || len(datos)%16 != 0 {
		if len(datos) == 16 {
			return nil, nil // solo el IV: cadena vacía
		}
		return nil, fmt.Errorf("malformed PDF: AES data length %d", len(datos))
	}
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plano := make([]byte, len(datos)-16)
	cipher.NewCBCDecrypter(c, datos[:16]).CryptBlocks(plano, datos[16:])
	relleno := int(plano[len(plano)-1])
	if relleno == 0 || relleno > 16 || relleno > len(plano) {
		return nil, fmt.Errorf("malformed PDF: AES padding")
	}
	return plano[:len(plano)-relleno], nil
}

type cbcReader struct {
	cbc  cipher.BlockMode
	rd   io.Reader
	buf  []byte
	pend []byte
}

func (r *cbcReader) Read(b []byte) (n int, err error) {
	if len(r.pend) == 0 {
		_, err = io.ReadFull(r.rd, r.buf)
		if err != nil {
			return 0, err
		}
		r.cbc.CryptBlocks(r.buf, r.buf)
		r.pend = r.buf
	}
	n = copy(b, r.pend)
	r.pend = r.pend[n:]
	return n, nil
}
