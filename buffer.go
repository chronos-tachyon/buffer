// Package buffer implements ring buffers of bytes.
package buffer

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/chronos-tachyon/assert"
)

// Buffer implements a ring buffer.  The ring buffer has space for 2**N bytes
// for user-specified N.
type Buffer struct {
	slice []byte
	mask  uint32
	a     uint32
	b     uint32
	busy  bool
	nbits byte
}

// New is a convenience function that allocates a new Buffer and calls Init on it.
func New(numBits byte) *Buffer {
	buffer := new(Buffer)
	buffer.Init(numBits)
	return buffer
}

// NumBits returns the number of bits used to initialize this Buffer.
func (buffer Buffer) NumBits() byte {
	return buffer.nbits
}

// Cap returns the maximum byte capacity of the Buffer.
func (buffer Buffer) Cap() uint {
	return uint(len(buffer.slice))
}

// Len returns the number of bytes currently in the Buffer.
func (buffer Buffer) Len() uint {
	a := uint(buffer.a)
	b := uint(buffer.b)
	if buffer.busy && a >= b {
		b += buffer.Cap()
	}
	return (b - a)
}

// IsEmpty returns true iff the Buffer contains no bytes.
func (buffer Buffer) IsEmpty() bool {
	return !buffer.busy
}

// IsFull returns true iff the Buffer contains the maximum number of bytes.
func (buffer Buffer) IsFull() bool {
	return buffer.busy && (buffer.a == buffer.b)
}

// Init initializes the Buffer.  The Buffer will hold a maximum of 2**N bits,
// where N is the argument provided.  The argument must be a number between 0
// and 31 inclusive.
func (buffer *Buffer) Init(numBits byte) {
	assert.Assertf(numBits <= 31, "numBits %d must not exceed 31", numBits)

	size := uint32(1) << numBits
	mask := (size - 1)
	*buffer = Buffer{
		slice: make([]byte, size),
		mask:  mask,
		a:     0,
		b:     0,
		busy:  false,
		nbits: numBits,
	}
}

// Clear erases the contents of the Buffer.
func (buffer *Buffer) Clear() {
	buffer.a = 0
	buffer.b = 0
	buffer.busy = false
}

// PrepareBulkWrite obtains a slice into which the caller can write bytes.  The
// bytes do not become a part of the buffer's contents until CommitBulkWrite is
// called.  If CommitBulkWrite is not subsequently called, the write is
// considered abandoned.
//
// The returned slice may contain fewer bytes than requested; it will return a
// nil slice iff the buffer is full.  The caller must check the slice's length
// before using it.  A short but non-empty return slice does *not* indicate a
// full buffer.
//
// The returned slice is only valid until the next call to any mutating method
// on this Buffer; mutating methods are those which take a pointer receiver.
//
func (buffer *Buffer) PrepareBulkWrite(length uint) []byte {
	bCap := buffer.Cap()
	a := uint(buffer.a)
	b := uint(buffer.b)

	var available uint
	switch {
	case buffer.busy && a == b:
		return nil

	case buffer.busy && a < b:
		available = bCap - b

	case buffer.busy:
		available = a - b

	default:
		buffer.a = 0
		buffer.b = 0
		available = bCap
	}

	if length > available {
		length = available
	}

	c := b + length
	return buffer.slice[b:c]
}

// CommitBulkWrite completes the bulk write begun by the previous call to
// PrepareBulkWrite.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkWrite.
//
func (buffer *Buffer) CommitBulkWrite(length uint) {
	if length == 0 {
		return
	}

	bCap := buffer.Cap()
	bMask := buffer.mask
	a := uint(buffer.a)
	b := uint(buffer.b)

	var available uint
	switch {
	case buffer.busy && a == b:
		available = 0

	case buffer.busy && a < b:
		available = bCap - b

	case buffer.busy:
		available = a - b

	default:
		available = bCap
	}

	assert.Assertf(length <= available, "length %d > available %d", length, available)
	buffer.b = uint32(b+length) & bMask
	buffer.busy = true
}

// WriteByte writes a single byte to the Buffer.  If the Buffer is full,
// ErrFull is returned.
func (buffer *Buffer) WriteByte(ch byte) error {
	if buffer.busy && buffer.a == buffer.b {
		return ErrFull
	}

	if !buffer.busy {
		buffer.a = 0
		buffer.b = 0
	}

	buffer.slice[buffer.b] = ch
	buffer.b = (buffer.b + 1) & buffer.mask
	buffer.busy = true
	return nil
}

// Write writes a slice of bytes to the Buffer.  If the Buffer is full, as many
// bytes as possible are written to the Buffer and ErrFull is returned.
func (buffer *Buffer) Write(p []byte) (int, error) {
	pLen := uint(len(p))
	if pLen == 0 {
		return 0, nil
	}

	if !buffer.busy {
		buffer.a = 0
		buffer.b = 0
	}

	bCap := buffer.Cap()
	bMask := buffer.mask
	bSlice := buffer.slice
	aw := buffer.a
	bw := buffer.b
	a := uint(aw)
	b := uint(bw)
	if buffer.busy && aw >= bw {
		b += bCap
	}

	bUsed := (b - a)
	bFree := bCap - bUsed
	err := error(nil)
	if pLen > bFree {
		pLen = bFree
		p = p[:pLen]
		err = ErrFull
	}

	c := b + pLen
	cw := uint32(c) & bMask

	if bw >= cw {
		x := (bCap - b)
		copy(bSlice[bw:bCap], p[:x])
		copy(bSlice[0:cw], p[x:])
	} else {
		copy(bSlice[bw:cw], p)
	}

	buffer.b = cw
	buffer.busy = true
	return int(pLen), err
}

// ReadFrom attempts to fill this Buffer by reading from the provided Reader.
// May return any error returned by the Reader, including io.EOF.  If a nil
// error is returned, then the buffer is now full.
func (buffer *Buffer) ReadFrom(r io.Reader) (int64, error) {
	var total int64
	var err error

	bCap := buffer.Cap()
	for err == nil {
		p := buffer.PrepareBulkWrite(bCap)
		if p == nil {
			break
		}

		var nn int
		nn, err = r.Read(p)
		assert.Assertf(nn >= 0, "Read() returned %d, which is < 0", nn)
		assert.Assertf(nn <= len(p), "Read() returned %d, which is > len(buffer) %d", nn, len(p))
		buffer.CommitBulkWrite(uint(nn))
		total += int64(nn)
	}
	return total, err
}

// PrepareBulkRead obtains a slice from which the caller can read bytes.  The
// bytes do not leave the buffer's contents until CommitBulkRead is called.  If
// CommitBulkRead is not subsequently called, the read acts as a "peek"
// operation.
//
// The returned slice may contain fewer bytes than requested; it will return a
// zero-length slice iff the buffer is empty.  The caller must check its length
// before using it.  A short but non-empty return slice does *not* indicate an
// empty buffer.
//
// The returned slice is only valid until the next call to any mutating method
// on this Buffer; mutating methods are those which take a pointer receiver.
//
func (buffer *Buffer) PrepareBulkRead(length uint) []byte {
	if !buffer.busy {
		return nil
	}

	bCap := buffer.Cap()
	a := uint(buffer.a)
	b := uint(buffer.b)
	if buffer.busy && a >= b {
		b = bCap
	}

	available := (b - a)
	if length > available {
		length = available
	}

	c := a + length
	return buffer.slice[a:c]
}

// CommitBulkRead completes the bulk read begun by the previous call to
// PrepareBulkRead.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkRead.
//
func (buffer *Buffer) CommitBulkRead(length uint) {
	if length == 0 {
		return
	}

	bCap := buffer.Cap()
	bMask := buffer.mask
	a := uint(buffer.a)
	b := uint(buffer.b)
	if buffer.busy && a >= b {
		b = bCap
	}

	available := (b - a)
	assert.Assertf(length <= available, "length %d > available %d", length, available)

	c := a + length
	buffer.a = uint32(c) & bMask
	buffer.busy = (buffer.a != buffer.b)
}

// ReadByte reads a single byte from the Buffer.  If the buffer is empty,
// ErrEmpty is returned.
func (buffer *Buffer) ReadByte() (byte, error) {
	if !buffer.busy {
		return 0, ErrEmpty
	}

	bMask := buffer.mask
	bSlice := buffer.slice
	aw := buffer.a
	bw := buffer.b
	ch := bSlice[aw]
	aw = (aw + 1) & bMask
	buffer.a = aw
	buffer.busy = (aw != bw)
	return ch, nil
}

// Read reads a slice of bytes from the Buffer.  If the buffer is empty,
// ErrEmpty is returned.
func (buffer *Buffer) Read(p []byte) (int, error) {
	pLen := uint(len(p))
	if pLen == 0 {
		return 0, nil
	}

	if !buffer.busy {
		return 0, ErrEmpty
	}

	bCap := buffer.Cap()
	bMask := buffer.mask
	bSlice := buffer.slice
	aw := buffer.a
	bw := buffer.b
	a := uint(aw)
	b := uint(bw)
	if aw >= bw {
		b += bCap
	}

	bUsed := (b - a)
	if pLen > bUsed {
		pLen = bUsed
		p = p[:pLen]
	}

	c := a + pLen
	cw := uint32(c) & bMask

	if aw >= cw {
		x := (bCap - a)
		copy(p[:x], bSlice[aw:bCap])
		copy(p[x:], bSlice[0:cw])
	} else {
		copy(p, bSlice[aw:cw])
	}

	buffer.a = cw
	buffer.busy = (cw != bw)
	return int(pLen), nil
}

// WriteTo attempts to drain this Buffer by writing to the provided Writer.
// May return any error returned by the Writer.  If a nil error is returned,
// then the Buffer is now empty.
func (buffer *Buffer) WriteTo(w io.Writer) (int64, error) {
	var total int64
	var err error

	bCap := buffer.Cap()
	for err == nil {
		p := buffer.PrepareBulkRead(bCap)
		if p == nil {
			break
		}

		var nn int
		nn, err = w.Write(p)
		assert.Assertf(nn >= 0, "Write() returned %d, which is < 0", nn)
		assert.Assertf(nn <= len(p), "Write() returned %d, which is > len(buffer) %d", nn, len(p))
		buffer.CommitBulkRead(uint(nn))
		total += int64(nn)
	}
	return total, err
}

// Bytes allocates and returns a copy of the Buffer's contents.
func (buffer Buffer) Bytes() []byte {
	bCap := buffer.Cap()
	bSlice := buffer.slice
	aw := uint(buffer.a)
	bw := uint(buffer.b)
	a := aw
	b := bw
	split := false
	if buffer.busy && aw >= bw {
		b += bCap
		split = true
	}
	out := make([]byte, b-a)
	if split {
		x := (bCap - aw)
		copy(out[:x], bSlice[aw:bCap])
		copy(out[x:], bSlice[0:bw])
	} else {
		copy(out, bSlice[aw:bw])
	}
	return out
}

// Slices returns zero or more []byte slices which provide a view of the
// Buffer's contents.  The slices are ordered from oldest to newest, the slices
// are only valid until the next mutating method call, and the contents of the
// slices should not be modified.
func (buffer Buffer) Slices() [][]byte {
	var out [][]byte
	if buffer.busy {
		bCap := buffer.Cap()
		bSlice := buffer.slice
		aw := buffer.a
		bw := buffer.b
		out = make([][]byte, 0, 2)
		if aw >= bw {
			out = append(out, bSlice[aw:bCap])
			out = append(out, bSlice[0:bw])
		} else {
			out = append(out, bSlice[aw:bw])
		}
	}
	return out
}

// DebugString returns a detailed dump of the Buffer's internal state.
func (buffer Buffer) DebugString() string {
	var buf strings.Builder
	buf.WriteString("Buffer(a=")
	buf.WriteString(strconv.FormatUint(uint64(buffer.a), 10))
	buf.WriteString(",b=")
	buf.WriteString(strconv.FormatUint(uint64(buffer.b), 10))
	buf.WriteString(",busy=")
	buf.WriteString(strconv.FormatBool(buffer.busy))
	buf.WriteString(",[ ")
	bCap := buffer.Cap()
	bMask := buffer.mask
	bSlice := buffer.slice
	a := uint(buffer.a)
	b := uint(buffer.b)
	if buffer.busy && a >= b {
		b += bCap
	}
	for c := a; c < b; c++ {
		cw := uint32(c) & bMask
		ch := bSlice[cw]
		fmt.Fprintf(&buf, "%02x ", ch)
	}
	buf.WriteString("])")
	return buf.String()
}

// GoString returns a brief dump of the Buffer's internal state.
func (buffer Buffer) GoString() string {
	return fmt.Sprintf("Buffer(a=%d,b=%d,cap=%d,busy=%t)", buffer.a, buffer.b, buffer.Cap(), buffer.busy)
}

// String returns a plain-text description of the buffer.
func (buffer Buffer) String() string {
	return fmt.Sprintf("(buffer with %d bytes)", buffer.Len())
}

var (
	_ io.Reader      = (*Buffer)(nil)
	_ io.Writer      = (*Buffer)(nil)
	_ io.ByteReader  = (*Buffer)(nil)
	_ io.ByteWriter  = (*Buffer)(nil)
	_ io.WriterTo    = (*Buffer)(nil)
	_ io.ReaderFrom  = (*Buffer)(nil)
	_ fmt.GoStringer = Buffer{}
	_ fmt.Stringer   = Buffer{}
)
