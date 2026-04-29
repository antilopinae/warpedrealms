package pbwire

import (
	"encoding/binary"
	"errors"
	"math"
)

const (
	Varint  = 0
	Fixed64 = 1
	Bytes   = 2
)

type Writer struct{ b []byte }

func (w *Writer) Bytes() []byte         { return w.b }
func (w *Writer) Tag(field int, wt int) { w.Uvarint(uint64(field<<3 | wt)) }
func (w *Writer) Uvarint(v uint64) {
	var buf [10]byte
	n := binary.PutUvarint(buf[:], v)
	w.b = append(w.b, buf[:n]...)
}
func (w *Writer) Bool(field int, v bool) {
	w.Tag(field, Varint)
	if v {
		w.Uvarint(1)
	} else {
		w.Uvarint(0)
	}
}
func (w *Writer) Uint32(field int, v uint32) { w.Tag(field, Varint); w.Uvarint(uint64(v)) }
func (w *Writer) Uint64(field int, v uint64) { w.Tag(field, Varint); w.Uvarint(v) }
func (w *Writer) Double(field int, v float64) {
	w.Tag(field, Fixed64)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
	w.b = append(w.b, buf[:]...)
}
func (w *Writer) String(field int, s string) {
	w.Tag(field, Bytes)
	w.Uvarint(uint64(len(s)))
	w.b = append(w.b, []byte(s)...)
}
func (w *Writer) Message(field int, p []byte) {
	w.Tag(field, Bytes)
	w.Uvarint(uint64(len(p)))
	w.b = append(w.b, p...)
}

type Reader struct {
	b []byte
	i int
}

func NewReader(b []byte) *Reader { return &Reader{b: b} }
func (r *Reader) Next() (field int, wt int, payload []byte, err error) {
	if r.i >= len(r.b) {
		return 0, 0, nil, errors.New("eof")
	}
	tag, n := binary.Uvarint(r.b[r.i:])
	if n <= 0 {
		return 0, 0, nil, errors.New("bad tag")
	}
	r.i += n
	field = int(tag >> 3)
	wt = int(tag & 0x7)
	switch wt {
	case Varint:
		_, n := binary.Uvarint(r.b[r.i:])
		if n <= 0 {
			return 0, 0, nil, errors.New("bad varint")
		}
		payload = r.b[r.i : r.i+n]
		r.i += n
		return
	case Fixed64:
		if r.i+8 > len(r.b) {
			return 0, 0, nil, errors.New("bad fixed64")
		}
		payload = r.b[r.i : r.i+8]
		r.i += 8
		return
	case Bytes:
		l, n := binary.Uvarint(r.b[r.i:])
		if n <= 0 {
			return 0, 0, nil, errors.New("bad len")
		}
		r.i += n
		if r.i+int(l) > len(r.b) {
			return 0, 0, nil, errors.New("bad bytes")
		}
		payload = r.b[r.i : r.i+int(l)]
		r.i += int(l)
		return
	default:
		return 0, 0, nil, errors.New("unsupported wt")
	}
}
func AsUint(payload []byte) uint64 { v, _ := binary.Uvarint(payload); return v }
func AsDouble(payload []byte) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(payload))
}
