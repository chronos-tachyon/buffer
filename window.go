package buffer

import (
	"hash"
	"io"

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
}

// NewWindow is a convenience function that allocates a Window and calls Init on it.
func NewWindow(numBits byte) *Window {
	w := new(Window)
	w.Init(numBits)
	return w
}

// Cap returns the maximum byte capacity of the Window.
func (w Window) Cap() uint {
	return uint(len(w.slice))
}

// Len returns the number of bytes currently in the Window.
func (w Window) Len() uint {
	if w.busy && w.i == w.j {
		return w.Cap()
	}
	if w.busy {
		i := uint(w.i)
		j := uint(w.j)
		if i > j {
			j += w.Cap()
		}
		return (j - i)
	}
	return 0
}

// IsEmpty returns true iff the Window contains no bytes.
func (w Window) IsEmpty() bool {
	return !w.busy
}

// IsFull returns true iff the Window contains the maximum number of bytes.
func (w Window) IsFull() bool {
	return w.busy && (w.i == w.j)
}

// Init initializes the Window.  The Window will hold a maximum of 2**N bits,
// where N is the argument provided.  The argument must be a number between 0
// and 31 inclusive.
func (w *Window) Init(numBits byte) {
	assert.Assertf(numBits <= 31, "numBits %d must not exceed 31", numBits)

	size := uint32(1) << numBits
	mask := (size - 1)
	*w = Window{
		slice: make([]byte, size),
		mask:  mask,
		i:     0,
		j:     0,
		busy:  false,
	}
}

// Clear erases the contents of the Window.
func (w *Window) Clear() {
	w.i = 0
	w.j = 0
	w.busy = false
}

// WriteByte writes a single byte to the Window.  If the Window is full, the
// oldest byte in the inferred stream is dropped.
func (w *Window) WriteByte(ch byte) error {
	w.slice[w.j] = ch
	same := w.busy && (w.i == w.j)
	w.busy = true
	w.j = (w.j + 1) & w.mask
	if same {
		w.i = w.j
	}
	return nil
}

// Write writes a slice of bytes to the Window.  If the Window is full or if
// the slice exceeds the capacity of the Window, the oldest bytes in the
// inferred stream are dropped until the slice fits.
func (w *Window) Write(p []byte) (int, error) {
	for _, ch := range p {
		_ = w.WriteByte(ch)
	}
	return len(p), nil
}

// Hash non-destructively writes the contents of the Window into the provided
// Hash object(s).
func (w Window) Hash(hashes ...hash.Hash) {
	if w.busy {
		i := w.i
		j := w.j
		if i < j {
			for _, h := range hashes {
				h.Write(w.slice[i:j])
			}
		} else {
			for _, h := range hashes {
				h.Write(w.slice[i:])
				h.Write(w.slice[:j])
			}
		}
	}
}

// Hash32 is a convenience method that constructs a Hash32, calls Window.Hash
// with it, and calls Sum32 on it.
func (w Window) Hash32(fn func() hash.Hash32) uint32 {
	h := fn()
	w.Hash(h)
	return h.Sum32()
}

// LookupByte returns a byte which was written previously.  The argument is the
// offset into the window, with 1 representing the most recently written byte
// and Window.Cap() representing the oldest byte still within the Window.
func (w Window) LookupByte(distance uint) (byte, error) {
	if distance == 0 || !w.busy {
		return 0, ErrBadDistance
	}

	wCap := w.Cap()
	i := uint(w.i)
	j := uint(w.j)
	if i >= j {
		j += wCap
	}

	available := (j - i)
	if distance > available {
		return 0, ErrBadDistance
	}

	k := j - distance
	kw := uint32(k) & w.mask
	return w.slice[kw], nil
}

// FindLongestPrefix searches the Window for the longest prefix of the given
// byte slice that exists within the Window's history.
//
// This method could use some additional optimization.
//
func (w Window) FindLongestPrefix(p []byte) (distance uint, length uint, ok bool) {
	pLen := uint(len(p))

	if pLen == 0 || !w.busy {
		return
	}

	wCap := w.Cap()
	iw := w.i
	jw := w.j
	i := uint(iw)
	j := uint(jw)

	loopGuts := func(kw uint32, k uint) {
		if w.slice[kw] != p[0] {
			return
		}

		currentDistance := (j - k)
		currentLength := uint(1)
		l := k + 1
		lw := uint32(l) & w.mask
		for l < j && currentLength < pLen {
			if w.slice[lw] != p[currentLength] {
				break
			}
			currentLength++
			l++
			lw = uint32(l) & w.mask
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

var (
	_ io.Writer     = (*Window)(nil)
	_ io.ByteWriter = (*Window)(nil)
)
