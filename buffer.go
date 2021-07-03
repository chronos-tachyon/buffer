// Package buffer provides multiple byte buffer implementations.
package buffer

import (
	"fmt"
	"io"

	"github.com/chronos-tachyon/assert"
	"github.com/chronos-tachyon/bzero"
)

// Buffer implements a byte buffer.  The Buffer has space for 2**N bytes for
// user-specified N.
type Buffer struct {
	slice []byte
	a     uint32
	b     uint32
	size  uint32
	nbits byte
}

// New is a convenience function that allocates a new Buffer and calls Init on it.
func New(numBits uint) *Buffer {
	buffer := new(Buffer)
	buffer.Init(numBits)
	return buffer
}

// NumBits returns the number of bits used to initialize this Buffer.
func (buffer Buffer) NumBits() uint {
	return uint(buffer.nbits)
}

// Size returns the maximum byte capacity of the Buffer.
func (buffer Buffer) Size() uint {
	return uint(buffer.size)
}

// Len returns the number of bytes currently in the Buffer.
func (buffer Buffer) Len() uint {
	return uint(buffer.b - buffer.a)
}

// IsEmpty returns true iff the Buffer contains no bytes.
func (buffer Buffer) IsEmpty() bool {
	return buffer.a == buffer.b
}

// IsFull returns true iff the Buffer contains the maximum number of bytes.
func (buffer Buffer) IsFull() bool {
	return (buffer.b - buffer.a) >= buffer.size
}

// Init initializes the Buffer.  The Buffer will hold a maximum of 2**N bits,
// where N is the argument provided.  The argument must be a number between 0
// and 31 inclusive.
func (buffer *Buffer) Init(numBits uint) {
	assert.Assertf(numBits <= 31, "numBits %d must not exceed 31", numBits)

	size := (uint32(1) << numBits)
	*buffer = Buffer{
		slice: make([]byte, size*2),
		a:     0,
		b:     0,
		size:  size,
		nbits: byte(numBits),
	}
}

// Clear erases the contents of the Buffer.
func (buffer *Buffer) Clear() {
	bzero.Uint8(buffer.slice)
	buffer.a = 0
	buffer.b = 0
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
	size := buffer.size
	a := buffer.a
	b := buffer.b

	x := (b - a)
	y := (size - x)
	if y == 0 {
		return nil
	}
	if length > uint(y) {
		length = uint(y)
	}

	buffer.shift(uint32(length))
	b = buffer.b
	c := b + uint32(length)
	return buffer.slice[b:c]
}

// CommitBulkWrite completes the bulk write begun by the previous call to
// PrepareBulkWrite.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkWrite.
//
func (buffer *Buffer) CommitBulkWrite(length uint) {
	size := buffer.size
	a := buffer.a
	b := buffer.b

	x := (b - a)
	y := (size - x)
	assert.Assertf(length <= uint(y), "length %d > available space %d", length, uint(y))

	buffer.b = b + uint32(length)
}

// WriteByte writes a single byte to the Buffer.  If the Buffer is full,
// ErrFull is returned.
func (buffer *Buffer) WriteByte(ch byte) error {
	size := buffer.size
	a := buffer.a
	b := buffer.b

	x := (b - a)
	y := (size - x)
	if y == 0 {
		return ErrFull
	}

	buffer.shift(1)
	b = buffer.b
	buffer.slice[b] = ch
	buffer.b = b + 1
	return nil
}

// Write writes a slice of bytes to the Buffer.  If the Buffer is full, as many
// bytes as possible are written to the Buffer and ErrFull is returned.
func (buffer *Buffer) Write(data []byte) (int, error) {
	size := buffer.size
	a := buffer.a
	b := buffer.b

	x := (b - a)
	y := (size - x)
	length := uint(len(data))
	var err error
	if length > uint(y) {
		length = uint(y)
		data = data[:length]
		err = ErrFull
	}

	buffer.shift(uint32(length))
	b = buffer.b
	c := b + uint32(length)
	copy(buffer.slice[b:c], data)
	buffer.b = c
	return int(length), err
}

// ReadFrom attempts to fill this Buffer by reading from the provided Reader.
// May return any error returned by the Reader, including io.EOF.  If a nil
// error is returned, then the buffer is now full.
func (buffer *Buffer) ReadFrom(r io.Reader) (int64, error) {
	var total int64
	var err error

	size := buffer.Size()
	for err == nil {
		buf := buffer.PrepareBulkWrite(size)
		if buf == nil {
			break
		}

		var nn int
		nn, err = r.Read(buf)
		assert.Assertf(nn >= 0, "Read() returned %d, which is < 0", nn)
		assert.Assertf(nn <= len(buf), "Read() returned %d, which is > len(buffer) %d", nn, len(buf))
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
	a := buffer.a
	b := buffer.b
	if a == b {
		return nil
	}

	x := (b - a)
	if length > uint(x) {
		length = uint(x)
	}

	c := a + uint32(length)
	return buffer.slice[a:c]
}

// CommitBulkRead completes the bulk read begun by the previous call to
// PrepareBulkRead.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkRead.
//
func (buffer *Buffer) CommitBulkRead(length uint) {
	a := buffer.a
	b := buffer.b
	x := (b - a)
	assert.Assertf(length <= uint(x), "length %d > available bytes %d", length, uint(x))

	c := a + uint32(length)
	buffer.a = c
}

// ReadByte reads a single byte from the Buffer.  If the buffer is empty,
// ErrEmpty is returned.
func (buffer *Buffer) ReadByte() (byte, error) {
	a := buffer.a
	b := buffer.b
	if a == b {
		return 0, ErrEmpty
	}

	ch := buffer.slice[a]
	buffer.a = a + 1
	return ch, nil
}

// Read reads a slice of bytes from the Buffer.  If the buffer is empty,
// ErrEmpty is returned.
func (buffer *Buffer) Read(data []byte) (int, error) {
	length := uint(len(data))
	if length == 0 {
		return 0, nil
	}

	a := buffer.a
	b := buffer.b
	if a == b {
		return 0, ErrEmpty
	}

	x := (b - a)
	if length > uint(x) {
		length = uint(x)
		data = data[:length]
	}

	c := a + uint32(length)
	copy(data, buffer.slice[a:c])
	buffer.a = c
	return int(length), nil
}

// WriteTo attempts to drain this Buffer by writing to the provided Writer.
// May return any error returned by the Writer.  If a nil error is returned,
// then the Buffer is now empty.
func (buffer *Buffer) WriteTo(w io.Writer) (int64, error) {
	var total int64
	var err error

	size := buffer.Size()
	for err == nil {
		buf := buffer.PrepareBulkRead(size)
		if buf == nil {
			break
		}

		var nn int
		nn, err = w.Write(buf)
		assert.Assertf(nn >= 0, "Write() returned %d, which is < 0", nn)
		assert.Assertf(nn <= len(buf), "Write() returned %d, which is > len(buffer) %d", nn, len(buf))
		buffer.CommitBulkRead(uint(nn))
		total += int64(nn)
	}
	return total, err
}

// BytesView returns a slice into the Buffer's contents.
func (buffer Buffer) BytesView() []byte {
	a := buffer.a
	b := buffer.b
	return buffer.slice[a:b]
}

// Bytes allocates and returns a copy of the Buffer's contents.
func (buffer Buffer) Bytes() []byte {
	a := buffer.a
	b := buffer.b
	x := (b - a)
	out := make([]byte, x)
	copy(out, buffer.slice[a:b])
	return out
}

// DebugString returns a detailed dump of the Buffer's internal state.
func (buffer Buffer) DebugString() string {
	buf := takeStringsBuilder()
	defer giveStringsBuilder(buf)

	nbits := buffer.nbits
	size := buffer.size
	a := buffer.a
	b := buffer.b
	x := (b - a)
	slice := buffer.slice

	buf.WriteString("Buffer(")
	fmt.Fprintf(buf, "nbits=%d, ", nbits)
	fmt.Fprintf(buf, "size=%d, ", size)
	fmt.Fprintf(buf, "a=%d, ", a)
	fmt.Fprintf(buf, "b=%d, ", b)
	fmt.Fprintf(buf, "len=%d, ", x)
	buf.WriteString("[")
	for a < b {
		ch := slice[a]
		a++
		fmt.Fprintf(buf, "%02x ", ch)
	}
	buf.WriteString(" ])")
	return buf.String()
}

// GoString returns a brief dump of the Buffer's internal state.
func (buffer Buffer) GoString() string {
	return fmt.Sprintf("Buffer(size=%d,a=%d,b=%d,len=%d)", buffer.size, buffer.a, buffer.b, buffer.Len())
}

// String returns a plain-text description of the buffer.
func (buffer Buffer) String() string {
	return fmt.Sprintf("(buffer with %d bytes)", buffer.Len())
}

func (buffer *Buffer) shift(n uint32) {
	slice := buffer.slice
	a := buffer.a
	b := buffer.b
	c := b + n
	if c <= uint32(len(slice)) {
		return
	}

	x := (b - a)
	copy(slice[0:x], slice[a:b])
	bzero.Uint8(slice[x:])
	buffer.a = 0
	buffer.b = x
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
