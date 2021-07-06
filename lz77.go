package buffer

import (
	"fmt"
	"math/bits"
	"sort"

	"github.com/chronos-tachyon/assert"
	"github.com/chronos-tachyon/bzero"
)

const hashLen = 4

// LZ77 implements a combination Window/Buffer that uses the Window to
// remember bytes that were recently removed from the Buffer, and that hashes
// all data that enters the Window so that LZ77-style prefix matching can be
// made efficient.
type LZ77 struct {
	slice    []byte
	hashMap  map[uint32]*[]uint32
	h        uint32
	i        uint32
	j        uint32
	bsize    uint32
	wsize    uint32
	hashMask uint32
	minLen   uint32
	maxLen   uint32
	maxDist  uint32
	bbits    byte
	wbits    byte
	hbits    byte
}

// LZ77Options holds options for initializing an instance of LZ77.
type LZ77Options struct {
	BufferNumBits       uint
	WindowNumBits       uint
	HashNumBits         uint
	MinMatchLength      uint
	MaxMatchLength      uint
	MaxMatchDistance    uint
	HasMinMatchLength   bool
	HasMaxMatchLength   bool
	HasMaxMatchDistance bool
}

// NewLZ77 is a convenience function that allocates a LZ77 and calls Init on it.
func NewLZ77(o LZ77Options) *LZ77 {
	lz77 := new(LZ77)
	lz77.Init(o)
	return lz77
}

// Options returns a LZ77Options struct which can be used to construct a new
// LZ77 with the same settings.
func (lz77 LZ77) Options() LZ77Options {
	return LZ77Options{
		BufferNumBits:       uint(lz77.bbits),
		WindowNumBits:       uint(lz77.wbits),
		HashNumBits:         uint(lz77.hbits),
		MinMatchLength:      uint(lz77.minLen),
		MaxMatchLength:      uint(lz77.maxLen),
		MaxMatchDistance:    uint(lz77.maxDist),
		HasMinMatchLength:   true,
		HasMaxMatchLength:   true,
		HasMaxMatchDistance: true,
	}
}

// BufferNumBits returns the size of the buffer in bits.
func (lz77 LZ77) BufferNumBits() uint {
	return uint(lz77.bbits)
}

// WindowNumBits returns the size of the sliding window in bits.
func (lz77 LZ77) WindowNumBits() uint {
	return uint(lz77.wbits)
}

// HashNumBits returns the size of the hash function output in bits.
func (lz77 LZ77) HashNumBits() uint {
	return uint(lz77.hbits)
}

// WindowSize returns the size of the sliding window, in bytes.
func (lz77 LZ77) WindowSize() uint {
	return uint(lz77.wsize)
}

// WindowLen returns the number of bytes currently in the LZ77's Window.
func (lz77 LZ77) WindowLen() uint {
	return uint(lz77.i - lz77.h)
}

// IsWindowEmpty returns true iff the Window is empty.
func (lz77 LZ77) IsWindowEmpty() bool {
	return lz77.h == lz77.i
}

// IsWindowFull returns true iff the Window is full.
func (lz77 LZ77) IsWindowFull() bool {
	return (lz77.i - lz77.h) >= lz77.wsize
}

// BufferSize returns the size of the buffer, in bytes.
func (lz77 LZ77) BufferSize() uint {
	return uint(lz77.bsize)
}

// Len returns the number of bytes currently in the LZ77's Buffer.
func (lz77 LZ77) Len() uint {
	return uint(lz77.j - lz77.i)
}

// IsEmpty returns true iff the buffer is empty.
func (lz77 LZ77) IsEmpty() bool {
	return lz77.i == lz77.j
}

// IsFull returns true iff the buffer is full.
func (lz77 LZ77) IsFull() bool {
	return (lz77.j - lz77.i) >= lz77.bsize
}

// Init initializes a LZ77.
func (lz77 *LZ77) Init(o LZ77Options) {
	bbits := o.BufferNumBits
	wbits := o.WindowNumBits
	hbits := o.HashNumBits

	assert.Assertf(bbits >= 2, "BufferNumBits %d must be at least 2", bbits)
	assert.Assertf(bbits <= 30, "BufferNumBits %d must not exceed 30", bbits)
	assert.Assertf(wbits <= 30, "WindowNumBits %d must not exceed 30", wbits)
	assert.Assertf(hbits <= 32, "HashNumBits %d must not exceed 32", hbits)

	bsize := (uint32(1) << bbits)
	wsize := (uint32(1) << wbits)

	maxLen := bsize
	if o.HasMaxMatchLength {
		if o.MaxMatchLength > uint(bsize) {
			o.MaxMatchLength = uint(bsize)
		}
		maxLen = uint32(o.MaxMatchLength)
	}

	minLen := uint32(hashLen)
	if o.HasMinMatchLength {
		if o.MinMatchLength > uint(bsize) {
			assert.Raisef("MinMatchLength %d > buffer capacity %d", o.MinMatchLength, bsize)
		}
		minLen = uint32(o.MinMatchLength)
	}

	maxDist := wsize
	if o.HasMaxMatchDistance {
		if o.MaxMatchDistance > uint(wsize) {
			o.MaxMatchDistance = uint(wsize)
		}
		maxDist = uint32(o.MaxMatchDistance)
	}

	if maxLen == 0 || maxDist == 0 {
		minLen = 0
		maxLen = 0
		maxDist = 0
		hbits = 0
	}

	if minLen == 0 && maxLen != 0 {
		minLen = 1
	}

	if minLen < hashLen {
		hbits = 0
	}

	assert.Assertf(minLen <= maxLen, "MinMatchLength %d > MaxMatchLength %d", minLen, maxLen)

	hashMask := ^uint32(0)
	if hbits < 32 {
		hashMask = (uint32(1) << hbits) - 1
	}

	*lz77 = LZ77{
		slice:    make([]byte, wsize+bsize*2),
		hashMap:  nil,
		h:        wsize,
		i:        wsize,
		j:        wsize,
		bsize:    bsize,
		wsize:    wsize,
		hashMask: hashMask,
		minLen:   minLen,
		maxLen:   maxLen,
		maxDist:  maxDist,
		bbits:    byte(bbits),
		wbits:    byte(wbits),
		hbits:    byte(hbits),
	}

	if hbits != 0 {
		lz77.hashMap = make(map[uint32]*[]uint32, maxDist)
		lz77.windowUpdateRegion(0)
	}
}

// Clear clears all data, emptying both the buffer and the sliding window.
func (lz77 *LZ77) Clear() {
	wsize := lz77.wsize
	lz77.h = wsize
	lz77.i = wsize
	lz77.j = wsize
	bzero.Uint8(lz77.slice)
	for _, ptr := range lz77.hashMap {
		*ptr = (*ptr)[:0]
	}
}

// WindowClear clears the sliding window.
func (lz77 *LZ77) WindowClear() {
	i := lz77.i
	lz77.h = i
	bzero.Uint8(lz77.slice[:i])
	for _, ptr := range lz77.hashMap {
		*ptr = (*ptr)[:0]
	}
}

// SetWindow replaces the sliding window with the given data.
func (lz77 *LZ77) SetWindow(data []byte) {
	length := uint(len(data))
	if maxDist := uint(lz77.maxDist); length > maxDist {
		x := length - maxDist
		data = data[x:]
		length = maxDist
	}

	i := lz77.i
	h := (i - uint32(length))

	lz77.h = h
	bzero.Uint8(lz77.slice[:h])
	copy(lz77.slice[h:i], data)
	for _, ptr := range lz77.hashMap {
		*ptr = (*ptr)[:0]
	}
	lz77.windowUpdateRegion(h)
}

// DebugString returns a detailed dump of the LZ77's internal state.
func (lz77 LZ77) DebugString() string {
	buf := takeStringsBuilder()
	defer giveStringsBuilder(buf)

	buf.WriteString("LZ77(\n")

	slice := lz77.slice
	h := lz77.h
	i := lz77.i
	j := lz77.j
	n := uint32(len(slice))

	used := (j - i)

	fmt.Fprintf(buf, "\tcapacity = %d\n", n)
	fmt.Fprintf(buf, "\tbbits = %d\n", lz77.bbits)
	fmt.Fprintf(buf, "\twbits = %d\n", lz77.wbits)
	fmt.Fprintf(buf, "\thbits = %d\n", lz77.hbits)
	fmt.Fprintf(buf, "\tminLen = %d\n", lz77.minLen)
	fmt.Fprintf(buf, "\tmaxLen = %d\n", lz77.maxLen)
	fmt.Fprintf(buf, "\tmaxDist = %d\n", lz77.maxDist)
	fmt.Fprintf(buf, "\thashMask = %#08x\n", lz77.hashMask)
	fmt.Fprintf(buf, "\tbCap = %d\n", lz77.bsize)
	fmt.Fprintf(buf, "\twCap = %d\n", lz77.wsize)
	fmt.Fprintf(buf, "\th = %d\n", h)
	fmt.Fprintf(buf, "\ti = %d\n", i)
	fmt.Fprintf(buf, "\tj = %d\n", j)
	fmt.Fprintf(buf, "\tlength = %d\n", used)

	buf.WriteString("\tbytes = [")
	for index := h; index < j; index++ {
		prefix := ""
		if index == i {
			prefix = " |"
		}
		ch := lz77.slice[index]
		fmt.Fprintf(buf, "%s %02x", prefix, ch)
	}
	if i == j {
		buf.WriteString(" |")
	}
	buf.WriteString(" ]\n")

	if lz77.hashMap != nil {
		buf.WriteString("\thashList = [")

		hashes := make([]uint32, 0, len(lz77.hashMap))
		for hash := range lz77.hashMap {
			hashes = append(hashes, hash)
		}
		sort.Sort(byUint32(hashes))

		for _, hash := range hashes {
			ptr := lz77.hashMap[hash]
			matches := *ptr
			matchesLen := uint(len(matches))
			x := uint(0)
			for x < matchesLen {
				if matches[x] >= h {
					break
				}
				x++
			}
			matches = matches[x:]
			matchesLen -= x
			if matchesLen > 0 {
				fmt.Fprintf(buf, " %#04x:%v", hash, matches)
			}
		}

		buf.WriteString(" ]\n")
	}

	buf.WriteString(")\n")
	return buf.String()
}

// GoString returns a brief dump of the LZ77's internal state.
func (lz77 LZ77) GoString() string {
	buf := takeStringsBuilder()
	defer giveStringsBuilder(buf)

	buf.WriteString("LZ77(")
	fmt.Fprintf(buf, "bbits=%d, ", lz77.bbits)
	fmt.Fprintf(buf, "wbits=%d, ", lz77.wbits)
	fmt.Fprintf(buf, "hbits=%d, ", lz77.hbits)
	fmt.Fprintf(buf, "minLen=%d, ", lz77.minLen)
	fmt.Fprintf(buf, "maxLen=%d, ", lz77.maxLen)
	fmt.Fprintf(buf, "maxDist=%d, ", lz77.maxDist)
	fmt.Fprintf(buf, "bsize=%d, ", lz77.bsize)
	fmt.Fprintf(buf, "wsize=%d, ", lz77.wsize)
	fmt.Fprintf(buf, "h=%d, ", lz77.h)
	fmt.Fprintf(buf, "i=%d, ", lz77.i)
	fmt.Fprintf(buf, "j=%d, ", lz77.j)
	buf.WriteString(")")

	return buf.String()
}

// String returns a plain-text description of the LZ77.
func (lz77 LZ77) String() string {
	return fmt.Sprintf("(window-buffer with %d bytes in the buffer)", lz77.Len())
}

// PrepareBulkWrite obtains a slice into which the caller can write bytes.  See
// Buffer.PrepareBulkWrite for more details.
//
func (lz77 *LZ77) PrepareBulkWrite(length uint) []byte {
	bsize := lz77.bsize
	i := lz77.i
	j := lz77.j
	x := (j - i)
	y := bsize - x

	if length > uint(y) {
		length = uint(y)
	}

	lz77.shift(uint32(length))
	j = lz77.j
	jPrime := j + uint32(length)
	return lz77.slice[j:jPrime]
}

// CommitBulkWrite completes the bulk write begun by the previous call to
// PrepareBulkWrite.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkWrite.
//
func (lz77 *LZ77) CommitBulkWrite(length uint) {
	bsize := lz77.bsize
	i := lz77.i
	j := lz77.j
	x := (j - i)
	y := bsize - x

	assert.Assertf(length <= uint(y), "length %d > available space %d", length, uint(y))

	lz77.j = j + uint32(length)
	lz77.windowUpdateRegion(j - hashLen)
}

// WriteByte writes a single byte to the LZ77's Buffer.
func (lz77 *LZ77) WriteByte(ch byte) error {
	bsize := lz77.bsize
	i := lz77.i
	j := lz77.j
	x := (j - i)
	y := bsize - x

	if y == 0 {
		return ErrFull
	}

	lz77.shift(1)
	j = lz77.j
	lz77.slice[j] = ch
	lz77.j = j + 1
	lz77.windowUpdateRegion(j - hashLen)
	return nil
}

// Write writes a slice of bytes to the LZ77's Buffer.
func (lz77 *LZ77) Write(data []byte) (int, error) {
	bsize := lz77.bsize
	i := lz77.i
	j := lz77.j
	x := (j - i)
	y := bsize - x

	length := uint(len(data))
	var err error
	if length > uint(y) {
		length = uint(y)
		data = data[:length]
		err = ErrFull
	}

	lz77.shift(uint32(length))
	j = lz77.j
	jPrime := j + uint32(length)
	copy(lz77.slice[j:jPrime], data)
	lz77.j = jPrime
	lz77.windowUpdateRegion(j - hashLen)
	return int(length), err
}

// PrepareBulkRead obtains a slice from which the caller can read bytes.  See
// Buffer.PrepareBulkRead for more details.
//
func (lz77 *LZ77) PrepareBulkRead(length uint) []byte {
	bsize := lz77.bsize
	if length > uint(bsize) {
		length = uint(bsize)
	}

	i := lz77.i
	j := lz77.j
	iPrime := i + uint32(length)
	if iPrime > j {
		iPrime = j
	}

	return lz77.slice[i:iPrime]
}

// CommitBulkRead completes the bulk read begun by the previous call to
// PrepareBulkRead.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkRead.
//
func (lz77 *LZ77) CommitBulkRead(length uint) {
	bsize := lz77.bsize
	if length > uint(bsize) {
		length = uint(bsize)
	}

	i := lz77.i
	j := lz77.j
	iPrime := i + uint32(length)
	assert.Assertf(iPrime <= j, "length %d exceeds %d bytes of available data", length, j-i)

	hPrime := lz77.h
	if x := (iPrime - lz77.maxDist); hPrime < x {
		hPrime = x
	}

	lz77.h = hPrime
	lz77.i = iPrime
	lz77.windowUpdateRegion(i)
}

// ReadByte reads a single byte, or returns ErrEmpty if the buffer is empty.
func (lz77 *LZ77) ReadByte() (byte, error) {
	i := lz77.i
	j := lz77.j
	iPrime := i + 1
	if iPrime > j {
		return 0, ErrEmpty
	}

	hPrime := lz77.h
	if x := (iPrime - lz77.maxDist); hPrime < x {
		hPrime = x
	}

	ch := lz77.slice[i]
	lz77.h = hPrime
	lz77.i = iPrime
	lz77.windowUpdateRegion(i)
	return ch, nil
}

// Read reads a slice of bytes from the LZ77's Buffer.  If the buffer is
// empty, ErrEmpty is returned.
func (lz77 *LZ77) Read(data []byte) (int, error) {
	length := uint(len(data))
	if length == 0 {
		return 0, nil
	}

	bsize := lz77.bsize
	if length > uint(bsize) {
		length = uint(bsize)
		data = data[:length]
	}

	i := lz77.i
	j := lz77.j
	iPrime := i + uint32(length)
	if iPrime > j {
		iPrime = j
		length = uint(iPrime - i)
		data = data[:length]
		if length == 0 {
			return 0, ErrEmpty
		}
	}

	hPrime := lz77.h
	if x := (iPrime - lz77.maxDist); hPrime < x {
		hPrime = x
	}

	lz77.h = hPrime
	lz77.i = iPrime
	copy(data, lz77.slice[i:iPrime])
	lz77.windowUpdateRegion(i)
	return int(length), nil
}

// Advance moves a slice of bytes from the LZ77's Buffer to its Window.  The
// nature of the slice depends on the LZ77's prefix match settings, the
// contents of the LZ77's Window, and the contents of the LZ77's Buffer.
func (lz77 *LZ77) Advance() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	hbits := lz77.hbits
	minLen := lz77.minLen
	maxLen := lz77.maxLen
	maxDist := lz77.maxDist
	bsize := lz77.bsize
	wsize := lz77.wsize
	h := lz77.h
	i := lz77.i
	j := lz77.j
	n := uint32(len(lz77.slice))

	assert.Assertf(h <= i, "h %d > i %d", h, i)
	assert.Assertf(i <= j, "i %d > j %d", i, j)
	assert.Assertf(j <= n, "j %d > n %d", j, n)

	if maxLen == 0 {
		assert.Assertf(minLen == 0, "minLen %d != 0", minLen)
		assert.Assertf(maxDist == 0, "maxDist %d != 0", maxDist)
		assert.Assertf(hbits == 0, "hbits %d != 0", hbits)
	} else {
		assert.Assert(minLen > 0, "minLen == 0")
		assert.Assert(maxDist > 0, "maxDist == 0")
		assert.Assertf(minLen <= maxLen, "minLen %d > maxLen %d", minLen, maxLen)
		assert.Assertf(maxLen <= bsize, "maxLen %d > bsize %d", maxLen, bsize)
		assert.Assertf(maxDist <= wsize, "maxDist %d > wsize %d", maxDist, wsize)
	}

	if hbits == 0 {
		assert.Assert(lz77.hashMap == nil, "hashMap is unexpectedly non-nil")
	} else {
		assert.Assertf(minLen >= hashLen, "minLen %d > hashLen %d", minLen, hashLen)
		assert.NotNil(&lz77.hashMap)
	}

	switch {
	case lz77.maxLen == 0:
		return lz77.advanceByte()
	case lz77.hbits == 0:
		return lz77.advanceNoHash()
	default:
		return lz77.advanceStandard()
	}
}

// WindowBytesView returns a slice into the Hybrid's Window's contents.
func (lz77 LZ77) WindowBytesView() []byte {
	return lz77.slice[lz77.h:lz77.i]
}

// WindowBytes allocates and returns a copy of the Hybrid's Window's contents.
func (lz77 LZ77) WindowBytes() []byte {
	shared := lz77.WindowBytesView()
	result := make([]byte, len(shared))
	copy(result, shared)
	return shared
}

// BufferBytesView returns a slice into the Hybrid's Buffer's contents.
func (lz77 LZ77) BufferBytesView() []byte {
	return lz77.slice[lz77.i:lz77.j]
}

// BufferBytes allocates and returns a copy of the Hybrid's Buffer's contents.
func (lz77 LZ77) BufferBytes() []byte {
	shared := lz77.BufferBytesView()
	result := make([]byte, len(shared))
	copy(result, shared)
	return shared
}

func (lz77 *LZ77) advanceByte() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	i := lz77.i
	j := lz77.j
	iPrime := i + 1
	if iPrime > j {
		return
	}

	hPrime := lz77.h
	if x := (iPrime - lz77.maxDist); hPrime < x {
		hPrime = x
	}

	buf = lz77.slice[i:iPrime]
	lz77.h = hPrime
	lz77.i = iPrime
	lz77.windowUpdateRegion(i)
	return
}

func (lz77 *LZ77) advanceNoHash() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	slice := lz77.slice
	minLen := lz77.minLen
	maxLen := lz77.maxLen
	h := lz77.h
	i := lz77.i
	j := lz77.j

	iPrime := i + 1
	if iPrime > j {
		return
	}

	if used := (j - i); maxLen > used {
		maxLen = used
	}

	var bestFound bool
	var bestDistance, bestLength uint32

	if minLen <= maxLen {
		x := i
		for x > h {
			x--
			if lz77.advanceCheckMatch(x, &bestFound, &bestDistance, &bestLength) {
				break
			}
		}
	}

	if bestFound {
		matchFound = true
		matchDistance = uint(bestDistance)
		matchLength = uint(bestLength)
		iPrime = i + bestLength
	}

	hPrime := h
	if x := (iPrime - lz77.maxDist); hPrime < x {
		hPrime = x
	}

	buf = slice[i:iPrime]
	lz77.h = hPrime
	lz77.i = iPrime
	lz77.windowUpdateRegion(i)
	return
}

func (lz77 *LZ77) advanceStandard() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	slice := lz77.slice
	minLen := lz77.minLen
	maxLen := lz77.maxLen
	h := lz77.h
	i := lz77.i
	j := lz77.j

	iPrime := i + 1
	if iPrime > j {
		return
	}

	if used := (j - i); maxLen > used {
		maxLen = used
	}

	var bestFound bool
	var bestDistance, bestLength uint32

	if minLen <= maxLen {
		hash := hash4(slice[i:i+hashLen], lz77.hashMask)
		ptr := lz77.hashMap[hash]
		if ptr != nil {
			matches := *ptr
			matchIndex := uint(len(matches))
			for matchIndex > 0 {
				matchIndex--
				x := matches[matchIndex]
				if lz77.advanceCheckMatch(x, &bestFound, &bestDistance, &bestLength) {
					break
				}
			}
		}
	}

	if bestFound {
		matchFound = true
		matchDistance = uint(bestDistance)
		matchLength = uint(bestLength)
		iPrime = i + bestLength
	}

	hPrime := h
	if x := (iPrime - lz77.maxDist); hPrime < x {
		hPrime = x
	}

	buf = slice[i:iPrime]
	lz77.h = hPrime
	lz77.i = iPrime
	lz77.windowUpdateRegion(i)
	return
}

func (lz77 *LZ77) advanceCheckMatch(x uint32, bestFoundPtr *bool, bestDistancePtr *uint32, bestLengthPtr *uint32) bool {
	bestFound := *bestFoundPtr
	bestDistance := *bestDistancePtr
	bestLength := *bestLengthPtr

	slice := lz77.slice
	minLen := lz77.minLen
	maxLen := lz77.maxLen
	h := lz77.h
	i := lz77.i

	if x < h {
		return true
	}

	if bestFound && slice[x+bestLength] != slice[i+bestLength] {
		return false
	}

	for index := uint32(0); index < maxLen; index++ {
		if slice[x+index] != slice[i+index] {
			break
		}
		lenSoFar := index + 1
		if lenSoFar >= minLen && (!bestFound || lenSoFar > bestLength) {
			bestDistance = (i - x)
			bestLength = lenSoFar
			bestFound = true
		}
	}

	*bestFoundPtr = bestFound
	*bestDistancePtr = bestDistance
	*bestLengthPtr = bestLength

	if bestFound && bestLength >= maxLen {
		return true
	}

	return false
}

func (lz77 *LZ77) windowUpdateRegion(index uint32) {
	if lz77.hashMap == nil {
		return
	}

	slice := lz77.slice
	h := lz77.h
	i := lz77.i
	j := lz77.j

	if index < h {
		index = h
	}

	end := j - hashLen
	if end > i {
		end = i
	}

	for index < end {
		hash := hash4(slice[index:index+hashLen], lz77.hashMask)

		var matches []uint32
		ptr := lz77.hashMap[hash]
		if ptr != nil {
			matches = *ptr
		}
		matchesLen := uint(len(matches))

		x := uint(0)
		for x < matchesLen {
			if matches[x] >= h {
				break
			}
			x++
		}
		matches = matches[x:]
		matchesLen -= x

		doAppend := true
		if matchesLen != 0 {
			y := matchesLen - 1
			if matches[y] >= index {
				doAppend = false
			}
		}

		if doAppend {
			if matches == nil {
				matches = make([]uint32, 0, 256)
			}
			matches = append(matches, uint32(index))
		}

		if ptr == nil {
			ptr = new([]uint32)
			*ptr = matches
			lz77.hashMap[hash] = ptr
		} else {
			*ptr = matches
		}

		index++
	}
}

func (lz77 *LZ77) shift(n uint32) {
	wsize := lz77.wsize
	slice := lz77.slice
	h := lz77.h
	i := lz77.i
	j := lz77.j
	k := j + n
	if k <= uint32(len(slice)) {
		return
	}

	windowLen := (i - h)
	bufferLen := (j - i)

	iPrime := wsize
	hPrime := (iPrime - windowLen)
	jPrime := (iPrime + bufferLen)

	copy(slice[hPrime:jPrime], slice[h:j])
	bzero.Uint8(slice[:hPrime])
	bzero.Uint8(slice[jPrime:])

	lz77.h = hPrime
	lz77.i = iPrime
	lz77.j = jPrime

	for _, ptr := range lz77.hashMap {
		matches := *ptr
		matchesLen := uint(len(matches))
		var a, b uint
		for a < matchesLen {
			index := matches[a]
			a++
			if index >= h {
				matches[b] = index - h + hPrime
				b++
			}
		}
		*ptr = matches[:b]
	}
}

// Equal returns true iff the given LZ77Options is semantically equal to this one.
func (opts LZ77Options) Equal(other LZ77Options) bool {
	ok := true
	ok = ok && (opts.BufferNumBits == other.BufferNumBits)
	ok = ok && (opts.WindowNumBits == other.WindowNumBits)
	ok = ok && (opts.HashNumBits == other.HashNumBits)
	ok = ok && (opts.HasMinMatchLength == other.HasMinMatchLength)
	ok = ok && (opts.HasMaxMatchLength == other.HasMaxMatchLength)
	ok = ok && (opts.HasMaxMatchDistance == other.HasMaxMatchDistance)
	ok = ok && opts.equalPartTwo(other)
	return ok
}

func (opts LZ77Options) equalPartTwo(other LZ77Options) bool {
	ok := true
	if opts.HasMinMatchLength && other.HasMinMatchLength {
		ok = ok && (opts.MinMatchLength == other.MinMatchLength)
	}
	if opts.HasMaxMatchLength && other.HasMaxMatchLength {
		ok = ok && (opts.MaxMatchLength == other.MaxMatchLength)
	}
	if opts.HasMaxMatchDistance && other.HasMaxMatchDistance {
		ok = ok && (opts.MaxMatchDistance == other.MaxMatchDistance)
	}
	return ok
}

// hash4 returns a hash of the first 4 bytes of slice.
//
// It is based on CityHash32.  Reference:
//
//    https://github.com/google/cityhash/blob/master/src/city.cc
//
func hash4(slice []byte, hashMask uint32) uint32 {
	const c1 = 0xcc9e2d51
	var b, c uint32
	b = uint32(slice[0])
	c = uint32(9) ^ b
	b = uint32(int32(b*c1) + int32(int8(slice[1])))
	c ^= b
	b = uint32(int32(b*c1) + int32(int8(slice[2])))
	c ^= b
	b = uint32(int32(b*c1) + int32(int8(slice[3])))
	c ^= b
	return fmix(mur(b, mur(4, c))) & hashMask
}

func mur(a, h uint32) uint32 {
	const c1 = 0xcc9e2d51
	const c2 = 0x1b873593
	a *= c1
	a = rotate32(a, 17)
	a *= c2
	h ^= a
	h = rotate32(h, 19)
	return h*5 + c2
}

func rotate32(x uint32, shift int) uint32 {
	return bits.RotateLeft32(x, -shift)
}

func fmix(h uint32) uint32 {
	h ^= (h >> 16)
	h *= 0x85ebca6b
	h ^= (h >> 13)
	h *= 0xc2b2ae35
	h ^= (h >> 16)
	return h
}

type byUint32 []uint32

func (list byUint32) Len() int {
	return len(list)
}

func (list byUint32) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list byUint32) Less(i, j int) bool {
	return list[i] < list[j]
}

var _ sort.Interface = byUint32(nil)
