package buffer

import (
	"fmt"
	"hash"
	"io"

	"github.com/chronos-tachyon/assert"
	"github.com/chronos-tachyon/bzero"
)

// Window implements a sliding window.  The Window has space for 2**N bytes for
// user-specified N.
type Window struct {
	slice []byte
	end   uint32
	size  uint32
	nbits byte
}

// NewWindow is a convenience function that allocates a Window and calls Init on it.
func NewWindow(numBits uint) *Window {
	window := new(Window)
	window.Init(numBits)
	return window
}

// NumBits returns the number of bits used to initialize this Window.
func (window Window) NumBits() uint {
	return uint(window.nbits)
}

// Size returns the maximum byte capacity of the Window.
func (window Window) Size() uint {
	return uint(window.size)
}

// IsZero returns true iff the Window contains only 0 bytes.
func (window Window) IsZero() bool {
	slice := window.slice
	j := window.end
	i := j - window.size
	for i < j {
		if slice[i] != 0 {
			return false
		}
		i++
	}
	return true
}

// Init initializes the Window.  The Window will hold a maximum of 2**N bits,
// where N is the argument provided.  The argument must be a number between 0
// and 31 inclusive.
func (window *Window) Init(numBits uint) {
	assert.Assertf(numBits <= 31, "numBits %d must not exceed 31", numBits)

	size := (uint32(1) << numBits)
	*window = Window{
		slice: make([]byte, size*2),
		end:   size,
		size:  size,
		nbits: byte(numBits),
	}
}

// Clear erases the contents of the Window.
func (window *Window) Clear() {
	bzero.Uint8(window.slice)
	window.end = window.size
}

// PrepareBulkWrite obtains a slice into which the caller can write bytes.  The
// bytes do not become a part of the Window's contents until CommitBulkWrite is
// called.  If CommitBulkWrite is not subsequently called, the write is
// considered abandoned.
//
// The returned slice may contain fewer bytes than requested, if the provided
// length is greater than the size of the Window.  The caller must check the
// slice's length before using it.
//
// The returned slice is only valid until the next call to any mutating method
// on this Window; mutating methods are those which take a pointer receiver.
//
func (window *Window) PrepareBulkWrite(length uint) []byte {
	size := window.size
	if length > uint(size) {
		length = uint(size)
	}

	window.shift(uint32(length))
	j := window.end
	k := j + uint32(length)
	return window.slice[j:k]
}

// CommitBulkWrite completes the bulk write begun by the previous call to
// PrepareBulkWrite.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkWrite.
//
func (window *Window) CommitBulkWrite(length uint) {
	size := window.size
	assert.Assertf(length <= uint(size), "length %d > window size %d", length, uint(size))
	j := window.end
	k := j + uint32(length)
	window.end = k
}

// WriteByte writes a single byte to the Window.  The oldest byte in the Window
// is dropped to make room.
func (window *Window) WriteByte(ch byte) error {
	window.shift(1)
	window.slice[window.end] = ch
	window.end++
	return nil
}

// Write writes a slice of bytes to the Window.  The oldest len(data) bytes in
// the Window are dropped to make room.  If len(data) exceeds Window.Size(),
// then only the last Window.Size() bytes of the slice will be recorded.
func (window *Window) Write(data []byte) (int, error) {
	result := len(data)
	length := uint(result)
	size := window.size
	if length > uint(size) {
		x := length - uint(size)
		data = data[x:]
		length = uint(size)
	}

	window.shift(uint32(length))
	j := window.end
	k := j + uint32(length)
	copy(window.slice[j:k], data)
	window.end = k
	return result, nil
}

// BytesView returns a slice into the Window's contents.
//
// The returned slice is only valid until the next call to any mutating method
// on this Window; mutating methods are those which take a pointer receiver.
//
func (window Window) BytesView() []byte {
	size := window.size
	j := window.end
	i := j - size
	return window.slice[i:j]
}

// Bytes allocates and returns a copy of the Window's contents.
func (window Window) Bytes() []byte {
	size := window.size
	j := window.end
	i := j - size
	out := make([]byte, size)
	copy(out, window.slice[i:j])
	return out
}

// Hash non-destructively writes the contents of the Window into the provided
// Hash object(s).
func (window Window) Hash(hashes ...hash.Hash) {
	view := window.BytesView()
	for _, h := range hashes {
		h.Write(view)
	}
}

// Hash32 is a convenience method that constructs a Hash32, calls Window.Hash
// with it, and calls Sum32 on it.
func (window Window) Hash32(fn func() hash.Hash32) uint32 {
	h := fn()
	h.Write(window.BytesView())
	return h.Sum32()
}

// LookupByte returns a byte which was written previously.  The argument is the
// offset into the window, with 1 representing the most recently written byte
// and Window.Size() representing the oldest byte still within the Window.
func (window Window) LookupByte(distance uint) (byte, error) {
	size := window.size
	if distance == 0 || distance > uint(size) {
		return 0, ErrBadDistance
	}

	j := window.end
	k := j - uint32(distance)
	return window.slice[k], nil
}

// LookupSlice returns a slice which was written previously.  The distance
// argument measures the offset into the Window, with 1 representing the most
// recently written byte and Window.Size() representing the oldest byte still
// within the Window.  The length argument is the maximum length of the slice
// to be returned; it may be shorter if it would otherwise extend past the most
// recently written byte.
func (window Window) LookupSlice(distance uint, length uint) ([]byte, error) {
	size := window.size
	if distance == 0 || distance > uint(size) {
		return nil, ErrBadDistance
	}

	if length > distance {
		length = distance
	}

	j := window.end
	k := j - uint32(distance)
	l := k + uint32(length)
	return window.slice[k:l], nil
}

// DebugString returns a detailed dump of the Window's internal state.
func (window Window) DebugString() string {
	buf := takeStringsBuilder()
	defer giveStringsBuilder(buf)

	nbits := window.nbits
	size := window.size
	j := window.end
	i := j - size
	slice := window.slice

	buf.WriteString("Window(")
	fmt.Fprintf(buf, "nbits=%d, ", nbits)
	fmt.Fprintf(buf, "size=%d, ", size)
	fmt.Fprintf(buf, "i=%d, ", i)
	fmt.Fprintf(buf, "j=%d, ", j)
	buf.WriteString("[")
	for i < j {
		ch := slice[i]
		i++
		fmt.Fprintf(buf, " %02x", ch)
	}
	buf.WriteString(" ])")
	return buf.String()
}

// GoString returns a brief dump of the Window's internal state.
func (window Window) GoString() string {
	return fmt.Sprintf("Window(size=%d,end=%d)", window.size, window.end)
}

// String returns a plain-text description of the Window.
func (window Window) String() string {
	return fmt.Sprintf("(sliding window with %d bytes)", window.Size())
}

func (window *Window) shift(n uint32) {
	size := window.size
	slice := window.slice
	j := window.end
	k := j + n
	if k <= uint32(len(slice)) {
		return
	}

	i := j - size
	copy(slice[0:size], slice[i:j])
	bzero.Uint8(slice[size:])
	window.end = size
}

var (
	_ io.Writer      = (*Window)(nil)
	_ io.ByteWriter  = (*Window)(nil)
	_ fmt.GoStringer = Window{}
	_ fmt.Stringer   = Window{}
)
