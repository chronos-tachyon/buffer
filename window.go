package buffer

import (
	"fmt"
	"hash"
	"io"
	"strconv"
	"strings"

	"github.com/chronos-tachyon/assert"
)

// Window implements a sliding window backed by a ring buffer.  The ring buffer
// has space for 2**N bytes for user-specified N.
type Window struct {
	slice []byte
	mask  uint32
	i     uint32
	j     uint32
	busy  bool
	nbits byte
}

// NewWindow is a convenience function that allocates a Window and calls Init on it.
func NewWindow(numBits byte) *Window {
	window := new(Window)
	window.Init(numBits)
	return window
}

// NumBits returns the number of bits used to initialize this Window.
func (window Window) NumBits() byte {
	return window.nbits
}

// Cap returns the maximum byte capacity of the Window.
func (window Window) Cap() uint {
	return uint(len(window.slice))
}

// Len returns the number of bytes currently in the Window.
func (window Window) Len() uint {
	if window.busy {
		i := uint(window.i)
		j := uint(window.j)
		if i >= j {
			j += window.Cap()
		}
		return (j - i)
	}
	return 0
}

// IsEmpty returns true iff the Window contains no bytes.
func (window Window) IsEmpty() bool {
	return !window.busy
}

// IsFull returns true iff the Window contains the maximum number of bytes.
func (window Window) IsFull() bool {
	return window.busy && (window.i == window.j)
}

// Init initializes the Window.  The Window will hold a maximum of 2**N bits,
// where N is the argument provided.  The argument must be a number between 0
// and 31 inclusive.
func (window *Window) Init(numBits byte) {
	assert.Assertf(numBits <= 31, "numBits %d must not exceed 31", numBits)

	size := uint32(1) << numBits
	mask := (size - 1)
	*window = Window{
		slice: make([]byte, size),
		mask:  mask,
		i:     0,
		j:     0,
		busy:  false,
		nbits: numBits,
	}
}

// Clear erases the contents of the Window.
func (window *Window) Clear() {
	window.i = 0
	window.j = 0
	window.busy = false
}

// WriteByte writes a single byte to the Window.  If the Window is full, the
// oldest byte in the inferred stream is dropped.
func (window *Window) WriteByte(ch byte) error {
	window.slice[window.j] = ch
	same := window.busy && (window.i == window.j)
	window.busy = true
	window.j = (window.j + 1) & window.mask
	if same {
		window.i = window.j
	}
	return nil
}

// Write writes a slice of bytes to the Window.  If the Window is full or if
// the slice exceeds the capacity of the Window, the oldest bytes in the
// inferred stream are dropped until the slice fits.
func (window *Window) Write(p []byte) (int, error) {
	for _, ch := range p {
		_ = window.WriteByte(ch)
	}
	return len(p), nil
}

// Bytes allocates and returns a copy of the Window's contents.
func (window Window) Bytes() []byte {
	wCap := window.Cap()
	iw := uint(window.i)
	jw := uint(window.j)
	i := iw
	j := jw
	split := false
	if window.busy && iw >= jw {
		j += wCap
		split = true
	}
	out := make([]byte, j-i)
	if split {
		x := (wCap - iw)
		copy(out[:x], window.slice[iw:wCap])
		copy(out[x:], window.slice[0:jw])
	} else {
		copy(out, window.slice[iw:jw])
	}
	return out
}

// Slices returns zero or more []byte slices which provide a view of the
// Window's contents.  The slices are ordered from oldest to newest, the slices
// are only valid until the next mutating method call, and the contents of the
// slices should not be modified.
func (window Window) Slices() [][]byte {
	var out [][]byte
	if window.busy {
		wCap := window.Cap()
		i := uint(window.i)
		j := uint(window.j)
		out = make([][]byte, 0, 2)
		if i >= j {
			out = append(out, window.slice[i:wCap])
			out = append(out, window.slice[0:j])
		} else {
			out = append(out, window.slice[i:j])
		}
	}
	return out
}

// Hash non-destructively writes the contents of the Window into the provided
// Hash object(s).
func (window Window) Hash(hashes ...hash.Hash) {
	if window.busy {
		i := window.i
		j := window.j
		if i < j {
			for _, h := range hashes {
				h.Write(window.slice[i:j])
			}
		} else {
			for _, h := range hashes {
				h.Write(window.slice[i:])
				h.Write(window.slice[:j])
			}
		}
	}
}

// Hash32 is a convenience method that constructs a Hash32, calls Window.Hash
// with it, and calls Sum32 on it.
func (window Window) Hash32(fn func() hash.Hash32) uint32 {
	h := fn()
	window.Hash(h)
	return h.Sum32()
}

// LookupByte returns a byte which was written previously.  The argument is the
// offset into the window, with 1 representing the most recently written byte
// and Window.Cap() representing the oldest byte still within the Window.
func (window Window) LookupByte(distance uint) (byte, error) {
	if distance == 0 || !window.busy {
		return 0, ErrBadDistance
	}

	wCap := window.Cap()
	i := uint(window.i)
	j := uint(window.j)
	if i >= j {
		j += wCap
	}

	available := (j - i)
	if distance > available {
		return 0, ErrBadDistance
	}

	k := j - distance
	kw := uint32(k) & window.mask
	return window.slice[kw], nil
}

// FindLongestPrefix searches the Window for the longest prefix of the given
// byte slice that exists within the Window's history.
//
// This method could use some additional optimization.
//
func (window Window) FindLongestPrefix(p []byte) (distance uint, length uint, ok bool) {
	pLen := uint(len(p))

	if pLen == 0 || !window.busy {
		return
	}

	wCap := window.Cap()
	iw := window.i
	jw := window.j
	i := uint(iw)
	j := uint(jw)

	loopGuts := func(kw uint32, k uint) {
		if window.slice[kw] != p[0] {
			return
		}

		currentDistance := (j - k)
		currentLength := uint(1)
		l := k + 1
		lw := uint32(l) & window.mask
		for l < j && currentLength < pLen {
			if window.slice[lw] != p[currentLength] {
				break
			}
			currentLength++
			l++
			lw = uint32(l) & window.mask
		}

		if !ok || currentLength > length || currentDistance < distance {
			distance = currentDistance
			length = currentLength
			ok = true
		}
	}

	if i >= j {
		j += wCap
		for kw := iw; kw < uint32(wCap); kw++ {
			loopGuts(kw, uint(kw))
		}
		for kw := uint32(0); kw < jw; kw++ {
			loopGuts(kw, uint(kw)+wCap)
		}
	} else {
		for kw := iw; kw < jw; kw++ {
			loopGuts(kw, uint(kw))
		}
	}
	return
}

// DebugString returns a detailed dump of the Window's internal state.
func (window Window) DebugString() string {
	var buf strings.Builder
	buf.WriteString("Window(i=")
	buf.WriteString(strconv.FormatUint(uint64(window.i), 10))
	buf.WriteString(",j=")
	buf.WriteString(strconv.FormatUint(uint64(window.j), 10))
	buf.WriteString(",busy=")
	buf.WriteString(strconv.FormatBool(window.busy))
	buf.WriteString(",[ ")
	i := uint(window.i)
	j := uint(window.j)
	if window.busy && i >= j {
		j += window.Cap()
	}
	for k := i; k < j; k++ {
		kw := uint32(k) & window.mask
		ch := window.slice[kw]
		fmt.Fprintf(&buf, "%02x ", ch)
	}
	buf.WriteString("])")
	return buf.String()
}

// GoString returns a brief dump of the Window's internal state.
func (window Window) GoString() string {
	return fmt.Sprintf("Window(i=%d,j=%d,cap=%d,busy=%t)", window.i, window.j, window.Cap(), window.busy)
}

// String returns a plain-text description of the Window.
func (window Window) String() string {
	return fmt.Sprintf("(sliding window with %d bytes)", window.Len())
}

var (
	_ io.Writer      = (*Window)(nil)
	_ io.ByteWriter  = (*Window)(nil)
	_ fmt.GoStringer = Window{}
	_ fmt.Stringer   = Window{}
)
