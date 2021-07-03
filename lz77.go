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

// BufferSize returns the size of the buffer, in bytes.
func (lz77 LZ77) BufferSize() uint {
	return uint(lz77.bsize)
}

// WindowSize returns the size of the sliding window, in bytes.
func (lz77 LZ77) WindowSize() uint {
	return uint(lz77.wsize)
}

// IsEmpty returns true iff the buffer is empty.
func (lz77 LZ77) IsEmpty() bool {
	return lz77.i == lz77.j
}

// IsFull returns true iff the buffer is full.
func (lz77 LZ77) IsFull() bool {
	return (lz77.j - lz77.i) >= lz77.bsize
}

// Len returns the number of bytes currently in the LZ77's Buffer.
func (lz77 LZ77) Len() uint {
	return uint(lz77.j - lz77.i)
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

// Clear clears all data, emptying the buffer and zeroing out the sliding window.
func (lz77 *LZ77) Clear() {
	wsize := lz77.wsize
	lz77.i = wsize
	lz77.j = wsize
	bzero.Uint8(lz77.slice[wsize:])
	lz77.WindowClear()
}

// WindowClear zeroes out the sliding window.
func (lz77 *LZ77) WindowClear() {
	wsize := lz77.wsize
	i := lz77.i
	start := (i - wsize)

	bzero.Uint8(lz77.slice[start:i])
	for _, ptr := range lz77.hashMap {
		*ptr = []uint32(nil)
	}
	lz77.windowUpdateRegion(start)
}

// SetWindow replaces the sliding window with the given data.
func (lz77 *LZ77) SetWindow(data []byte) {
	wsize := lz77.wsize

	length := uint(len(data))
	if length > uint(wsize) {
		x := length - uint(wsize)
		data = data[x:]
		length = uint(wsize)
	}

	i := lz77.i
	start := (i - wsize)
	offset := (i - uint32(length))

	slice := lz77.slice
	bzero.Uint8(slice[start:offset])
	copy(slice[offset:i], data)
	for _, ptr := range lz77.hashMap {
		*ptr = []uint32(nil)
	}
	lz77.windowUpdateRegion(start)
}

// DebugString returns a detailed dump of the LZ77's internal state.
func (lz77 LZ77) DebugString() string {
	buf := takeStringsBuilder()
	defer giveStringsBuilder(buf)

	buf.WriteString("LZ77(\n")

	bsize := lz77.bsize
	wsize := lz77.wsize
	minLen := lz77.minLen
	maxLen := lz77.maxLen
	maxDist := lz77.maxDist

	slice := lz77.slice
	i := lz77.i
	j := lz77.j
	n := uint32(len(slice))

	start := (i - wsize)
	matchStart := (i - maxDist)
	used := (j - i)

	fmt.Fprintf(buf, "\tcapacity = %d\n", n)
	fmt.Fprintf(buf, "\tbbits = %d\n", lz77.bbits)
	fmt.Fprintf(buf, "\twbits = %d\n", lz77.wbits)
	fmt.Fprintf(buf, "\thbits = %d\n", lz77.hbits)
	fmt.Fprintf(buf, "\tminLen = %d\n", minLen)
	fmt.Fprintf(buf, "\tmaxLen = %d\n", maxLen)
	fmt.Fprintf(buf, "\tmaxDist = %d\n", maxDist)
	fmt.Fprintf(buf, "\thashMask = %#08x\n", lz77.hashMask)
	fmt.Fprintf(buf, "\tbCap = %d\n", bsize)
	fmt.Fprintf(buf, "\twCap = %d\n", wsize)
	fmt.Fprintf(buf, "\ti = %d\n", i)
	fmt.Fprintf(buf, "\tj = %d\n", j)
	fmt.Fprintf(buf, "\tstart = %d\n", start)
	fmt.Fprintf(buf, "\tmatchStart = %d\n", matchStart)
	fmt.Fprintf(buf, "\tlength = %d\n", used)

	buf.WriteString("\tbytes = [")
	for index := matchStart; index < j; index++ {
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
				if matches[x] >= matchStart {
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
	fmt.Fprintf(buf, "i=%d, ", lz77.i)
	fmt.Fprintf(buf, "j=%d, ", lz77.j)
	fmt.Fprintf(buf, "start=%d, ", lz77.i-lz77.wsize)
	fmt.Fprintf(buf, "matchStart=%d", lz77.i-lz77.maxDist)
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
	k := j + uint32(length)
	return lz77.slice[j:k]
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
	k := j + uint32(length)
	copy(lz77.slice[j:k], data)
	lz77.j = k
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
	k := i + uint32(length)
	if k > j {
		k = j
	}

	return lz77.slice[i:k]
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
	k := i + uint32(length)

	assert.Assertf(k <= j, "length %d exceeds %d bytes of available data", length, j-i)

	lz77.i = k
	lz77.windowUpdateRegion(i)
}

// ReadByte reads a single byte, or returns ErrEmpty if the buffer is empty.
func (lz77 *LZ77) ReadByte() (byte, error) {
	i := lz77.i
	j := lz77.j
	k := i + 1

	if k > j {
		return 0, ErrEmpty
	}

	ch := lz77.slice[i]
	lz77.i = k
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
	k := i + uint32(length)
	if k > j {
		k = j
		length = uint(k - i)
		data = data[:length]
		if length == 0 {
			return 0, ErrEmpty
		}
	}

	copy(data, lz77.slice[i:k])
	lz77.i = k
	lz77.windowUpdateRegion(i)
	return int(length), nil
}

// Advance moves a slice of bytes from the LZ77's Buffer to its Window.  The
// nature of the slice depends on the LZ77's prefix match settings, the
// contents of the LZ77's Window, and the contents of the LZ77's Buffer.
func (lz77 *LZ77) Advance() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	i := lz77.i
	j := lz77.j
	n := uint32(len(lz77.slice))
	bsize := lz77.bsize
	wsize := lz77.wsize
	minLen := lz77.minLen
	maxLen := lz77.maxLen
	maxDist := lz77.maxDist
	hbits := lz77.hbits

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

func (lz77 *LZ77) advanceByte() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	i := lz77.i
	j := lz77.j
	k := i + 1
	if k > j {
		return
	}

	buf = lz77.slice[i:k]
	lz77.i = k
	lz77.windowUpdateRegion(i)
	return
}

func (lz77 *LZ77) advanceNoHash() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	slice := lz77.slice
	i := lz77.i
	j := lz77.j
	wsize := lz77.wsize
	minLen := lz77.minLen
	maxLen := lz77.maxLen

	k := i + 1
	if k > j {
		return
	}

	used := (j - i)
	if maxLen > used {
		maxLen = used
	}
	if maxLen < minLen {
		buf = slice[i:k]
		lz77.i = k
		lz77.windowUpdateRegion(i)
		return
	}

	var bestFound bool
	var bestDistance, bestLength uint32

	start := i - wsize
	x := i
	for x > start {
		x--

		if bestFound && slice[x+bestLength] != slice[i+bestLength] {
			continue
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

		if bestFound && bestLength >= maxLen {
			break
		}
	}

	if bestFound {
		matchFound = true
		matchDistance = uint(bestDistance)
		matchLength = uint(bestLength)
		k = i + bestLength
	}

	buf = slice[i:k]
	lz77.i = k
	lz77.windowUpdateRegion(i)
	return
}

func (lz77 *LZ77) advanceStandard() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	slice := lz77.slice
	i := lz77.i
	j := lz77.j
	minLen := lz77.minLen
	maxLen := lz77.maxLen
	maxDist := lz77.maxDist

	k := i + 1
	if k > j {
		return
	}

	used := (j - i)
	if maxLen > used {
		maxLen = used
	}

	var bestFound bool
	var bestDistance, bestLength uint32

	if maxLen >= minLen {
		matchStart := i - maxDist
		hash := hash4(slice[i:i+hashLen], lz77.hashMask)
		ptr := lz77.hashMap[hash]
		if ptr != nil {
			matches := *ptr
			matchIndex := uint(len(matches))
			for matchIndex > 0 {
				matchIndex--

				x := matches[matchIndex]
				if x < matchStart {
					break
				}

				if bestFound && slice[x+bestLength] != slice[i+bestLength] {
					continue
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

				if bestFound && bestLength >= maxLen {
					break
				}
			}
		}
	}

	if bestFound {
		matchFound = true
		matchDistance = uint(bestDistance)
		matchLength = uint(bestLength)
		k = i + bestLength
	}

	buf = slice[i:k]
	lz77.i = uint32(k)
	lz77.windowUpdateRegion(i)
	return
}

func (lz77 *LZ77) windowUpdateRegion(index uint32) {
	if lz77.hashMap == nil {
		return
	}

	slice := lz77.slice
	i := lz77.i
	j := lz77.j

	matchStart := i - lz77.maxDist
	if index < matchStart {
		index = matchStart
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
			if matches[x] >= matchStart {
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
	i := lz77.i
	j := lz77.j
	k := j + n
	if k <= uint32(len(slice)) {
		return
	}

	start := (i - wsize)
	x := (j - start)
	copy(slice[0:x], slice[start:j])
	bzero.Uint8(slice[x:])
	lz77.i = wsize
	lz77.j = x

	for _, ptr := range lz77.hashMap {
		matches := *ptr
		matchesLen := uint(len(matches))
		var a, b uint
		for a < matchesLen {
			index := matches[a]
			a++
			if index >= start {
				matches[b] = index - start
				b++
			}
		}
		*ptr = matches[:b]
	}
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

// Equal returns true iff the given LZ77Options is semantically equal to this one.
func (opts LZ77Options) Equal(other LZ77Options) bool {
	ok := true
	ok = ok && (opts.BufferNumBits == other.BufferNumBits)
	ok = ok && (opts.WindowNumBits == other.WindowNumBits)
	ok = ok && (opts.HashNumBits == other.HashNumBits)
	ok = ok && (opts.HasMinMatchLength == other.HasMinMatchLength)
	ok = ok && (opts.HasMaxMatchLength == other.HasMaxMatchLength)
	ok = ok && (opts.HasMaxMatchDistance == other.HasMaxMatchDistance)
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
