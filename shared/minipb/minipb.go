package minipb

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

type Endian int

const (
	LittleEndian Endian = iota
	BigEndian
)

type Writer struct {
	buf   []byte
	order binary.ByteOrder
}

func NewWriter(e Endian) *Writer {
	var o binary.ByteOrder = binary.LittleEndian
	if e == BigEndian {
		o = binary.BigEndian
	}
	return &Writer{order: o}
}
func (w *Writer) Bytes() []byte { return w.buf }
func (w *Writer) Field(tag byte, payload []byte) {
	w.buf = append(w.buf, tag)
	l := make([]byte, 4)
	w.order.PutUint32(l, uint32(len(payload)))
	w.buf = append(w.buf, l...)
	w.buf = append(w.buf, payload...)
}
func F64(v float64, e Endian) []byte {
	b := make([]byte, 8)
	var o binary.ByteOrder = binary.LittleEndian
	if e == BigEndian {
		o = binary.BigEndian
	}
	o.PutUint64(b, math.Float64bits(v))
	return b
}
func U64(v uint64, e Endian) []byte {
	b := make([]byte, 8)
	var o binary.ByteOrder = binary.LittleEndian
	if e == BigEndian {
		o = binary.BigEndian
	}
	o.PutUint64(b, v)
	return b
}
func U32(v uint32, e Endian) []byte {
	b := make([]byte, 4)
	var o binary.ByteOrder = binary.LittleEndian
	if e == BigEndian {
		o = binary.BigEndian
	}
	o.PutUint32(b, v)
	return b
}
func Str(s string) []byte { return []byte(s) }
func Bool(v bool) []byte {
	if v {
		return []byte{1}
	}
	return []byte{0}
}

type Reader struct {
	data  []byte
	off   int
	order binary.ByteOrder
}

func NewReader(data []byte, e Endian) *Reader {
	var o binary.ByteOrder = binary.LittleEndian
	if e == BigEndian {
		o = binary.BigEndian
	}
	return &Reader{data: data, order: o}
}
func (r *Reader) Next() (byte, []byte, error) {
	if r.off >= len(r.data) {
		return 0, nil, io.EOF
	}
	if r.off+5 > len(r.data) {
		return 0, nil, fmt.Errorf("short header")
	}
	t := r.data[r.off]
	r.off++
	l := int(r.order.Uint32(r.data[r.off : r.off+4]))
	r.off += 4
	if r.off+l > len(r.data) {
		return 0, nil, fmt.Errorf("short payload")
	}
	p := r.data[r.off : r.off+l]
	r.off += l
	return t, p, nil
}
func ReadF64(b []byte, e Endian) float64 {
	var o binary.ByteOrder = binary.LittleEndian
	if e == BigEndian {
		o = binary.BigEndian
	}
	return math.Float64frombits(o.Uint64(b))
}
func ReadU64(b []byte, e Endian) uint64 {
	var o binary.ByteOrder = binary.LittleEndian
	if e == BigEndian {
		o = binary.BigEndian
	}
	return o.Uint64(b)
}
func ReadU32(b []byte, e Endian) uint32 {
	var o binary.ByteOrder = binary.LittleEndian
	if e == BigEndian {
		o = binary.BigEndian
	}
	return o.Uint32(b)
}
func ReadBool(b []byte) bool { return len(b) > 0 && b[0] == 1 }
