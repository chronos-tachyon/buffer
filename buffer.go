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
	i     uint32
	j     uint32
	busy  bool
	nbits byte
}

// New is a convenience function that allocates a new Buffer and calls Init on it.
func New(numBits byte) *Buffer {
	b := new(Buffer)
	b.Init(numBits)
	return b
}

// NumBits returns the number of bits used to initialize this Buffer.
func (b Buffer) NumBits() byte {
	return b.nbits
}

// Cap returns the maximum byte capacity of the Buffer.
func (b Buffer) Cap() uint {
	return uint(len(b.slice))
}

// Len returns the number of bytes currently in the Buffer.
func (b Buffer) Len() uint {
	if b.busy {
		i := uint(b.i)
		j := uint(b.j)
		if i >= j {
			j += b.Cap()
		}
		return (j - i)
	}
	return 0
}

// IsEmpty returns true iff the Buffer contains no bytes.
func (b Buffer) IsEmpty() bool {
	return !b.busy
}

// IsFull returns true iff the Buffer contains the maximum number of bytes.
func (b Buffer) IsFull() bool {
	return b.busy && (b.i == b.j)
}

// Init initializes the Buffer.  The Buffer will hold a maximum of 2**N bits,
// where N is the argument provided.  The argument must be a number between 0
// and 31 inclusive.
func (b *Buffer) Init(numBits byte) {
	assert.Assertf(numBits <= 31, "numBits %d must not exceed 31", numBits)

	size := uint32(1) << numBits
	mask := (size - 1)
	*b = Buffer{
		slice: make([]byte, size),
		mask:  mask,
		i:     0,
		j:     0,
		busy:  false,
		nbits: numBits,
	}
}

// Clear erases the contents of the Buffer.
func (b *Buffer) Clear() {
	b.i = 0
	b.j = 0
	b.busy = false
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
func (b *Buffer) PrepareBulkWrite(length uint) []byte {
	bCap := b.Cap()
	i := uint(b.i)
	j := uint(b.j)

	var available uint
	switch {
	case b.busy && i == j:
		return nil

	case b.busy && i < j:
		available = bCap - j

	case b.busy:
		available = i - j

	default:
		b.i = 0
		b.j = 0
		available = bCap
	}

	if length > available {
		length = available
	}

	k := j + length
	return b.slice[j:k]
}

// CommitBulkWrite completes the bulk write begun by the previous call to
// PrepareBulkWrite.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkWrite.
//
func (b *Buffer) CommitBulkWrite(length uint) {
	if length == 0 {
		return
	}

	bCap := b.Cap()
	i := uint(b.i)
	j := uint(b.j)

	var available uint
	switch {
	case b.busy && i == j:
		available = 0

	case b.busy && i < j:
		available = bCap - j

	case b.busy:
		available = i - j

	default:
		available = bCap
	}

	assert.Assertf(length <= available, "length %d > available %d", length, available)
	b.j = uint32(j+length) & b.mask
	b.busy = true
}

// WriteByte writes a single byte to the Buffer.  If the Buffer is full,
// ErrFull is returned.
func (b *Buffer) WriteByte(ch byte) error {
	if b.busy && b.i == b.j {
		return ErrFull
	}

	if !b.busy {
		b.i = 0
		b.j = 0
	}

	b.slice[b.j] = ch
	b.j = (b.j + 1) & b.mask
	b.busy = true
	return nil
}

// Write writes a slice of bytes to the Buffer.  If the Buffer is full, as many
// bytes as possible are written to the Buffer and ErrFull is returned.
func (b *Buffer) Write(p []byte) (int, error) {
	pLen := uint(len(p))
	if pLen == 0 {
		return 0, nil
	}

	if !b.busy {
		b.i = 0
		b.j = 0
	}

	bCap := b.Cap()
	jw := b.j
	i := uint(b.i)
	j := uint(b.j)
	if b.busy && i >= j {
		j += bCap
	}

	bUsed := (j - i)
	bFree := bCap - bUsed
	err := error(nil)
	if pLen > bFree {
		pLen = bFree
		p = p[:pLen]
		err = ErrFull
	}

	k := j + pLen
	kw := uint32(k) & b.mask

	if jw >= kw {
		x := (bCap - j)
		copy(b.slice[jw:bCap], p[:x])
		copy(b.slice[0:kw], p[x:])
	} else {
		copy(b.slice[jw:kw], p)
	}

	b.j = kw
	b.busy = true
	return int(pLen), err
}

// ReadFrom attempts to fill this Buffer by reading from the provided Reader.
// May return any error returned by the Reader, including io.EOF.  If a nil
// error is returned, then the buffer is now full.
func (b *Buffer) ReadFrom(r io.Reader) (int64, error) {
	var total int64
	var err error

	bCap := b.Cap()
	for err == nil {
		p := b.PrepareBulkWrite(bCap)
		if p == nil {
			break
		}

		var nn int
		nn, err = r.Read(p)
		assert.Assertf(nn >= 0, "Read() returned %d, which is < 0", nn)
		assert.Assertf(nn <= len(p), "Read() returned %d, which is > len(buffer) %d", nn, len(p))
		b.CommitBulkWrite(uint(nn))
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
func (b *Buffer) PrepareBulkRead(length uint) []byte {
	if !b.busy {
		return nil
	}

	bCap := b.Cap()
	i := uint(b.i)
	j := uint(b.j)
	if i >= j {
		j = bCap
	}

	available := (j - i)
	if length > available {
		length = available
	}

	k := i + length
	return b.slice[i:k]
}

// CommitBulkRead completes the bulk read begun by the previous call to
// PrepareBulkRead.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkRead.
//
func (b *Buffer) CommitBulkRead(length uint) {
	if length == 0 {
		return
	}

	bCap := b.Cap()
	i := uint(b.i)
	j := uint(b.j)

	var available uint
	if b.busy {
		if i >= j {
			j = bCap
		}
		available = (j - i)
	}
	assert.Assertf(length <= available, "length %d > available %d", length, available)

	k := i + length
	b.i = uint32(k) & b.mask
	b.busy = (b.i != b.j)
}

// ReadByte reads a single byte from the Buffer.  If the buffer is empty,
// ErrEmpty is returned.
func (b *Buffer) ReadByte() (byte, error) {
	if !b.busy {
		return 0, ErrEmpty
	}

	ch := b.slice[b.i]
	b.i = (b.i + 1) & b.mask
	b.busy = (b.i != b.j)
	return ch, nil
}

// Read reads a slice of bytes from the Buffer.  If the buffer is empty,
// ErrEmpty is returned.
func (b *Buffer) Read(p []byte) (int, error) {
	pLen := uint(len(p))
	if pLen == 0 {
		return 0, nil
	}

	if !b.busy {
		return 0, ErrEmpty
	}

	bCap := b.Cap()
	i := uint(b.i)
	j := uint(b.j)
	if i >= j {
		j += bCap
	}

	available := (j - i)
	if pLen > available {
		pLen = available
		p = p[:pLen]
	}

	k := i + pLen
	kw := uint32(k) & b.mask

	if b.i >= kw {
		x := (bCap - i)
		copy(p[:x], b.slice[i:bCap])
		copy(p[x:], b.slice[0:kw])
	} else {
		copy(p, b.slice[i:kw])
	}

	b.i = uint32(kw)
	b.busy = (b.i != b.j)
	return int(pLen), nil
}

// WriteTo attempts to drain this Buffer by writing to the provided Writer.
// May return any error returned by the Writer.  If a nil error is returned,
// then the Buffer is now empty.
func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	var total int64
	var err error

	bCap := b.Cap()
	for err == nil {
		p := b.PrepareBulkRead(bCap)
		if p == nil {
			break
		}

		var nn int
		nn, err = w.Write(p)
		assert.Assertf(nn >= 0, "Write() returned %d, which is < 0", nn)
		assert.Assertf(nn <= len(p), "Write() returned %d, which is > len(buffer) %d", nn, len(p))
		b.CommitBulkRead(uint(nn))
		total += int64(nn)
	}
	return total, err
}

func (b Buffer) DebugString() string {
	var buf strings.Builder
	buf.WriteString("Buffer(i=")
	buf.WriteString(strconv.FormatUint(uint64(b.i), 10))
	buf.WriteString(",j=")
	buf.WriteString(strconv.FormatUint(uint64(b.j), 10))
	buf.WriteString(",busy=")
	buf.WriteString(strconv.FormatBool(b.busy))
	buf.WriteString(",[ ")
	i := uint(b.i)
	j := uint(b.j)
	if b.busy && i >= j {
		j += b.Cap()
	}
	for k := i; k < j; k++ {
		kw := uint32(k) & b.mask
		ch := b.slice[kw]
		fmt.Fprintf(&buf, "%02x ", ch)
	}
	buf.WriteString("])")
	return buf.String()
}

func (b Buffer) GoString() string {
	return fmt.Sprintf("Buffer(i=%d,j=%d,cap=%d,busy=%t)", b.i, b.j, b.Cap(), b.busy)
}

func (b Buffer) String() string {
	return fmt.Sprintf("(buffer with %d bytes)", b.Len())
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
