package buffer

import (
	"fmt"
	"math/bits"
	"sort"

	"github.com/chronos-tachyon/assert"
	"github.com/chronos-tachyon/bzero"
)

const hashLen = 4

// Hybrid implements a combination Window/Buffer that uses the Window to
// remember bytes that were recently removed from the Buffer, and that hashes
// all data that enters the Window so that LZ77-style prefix matching can be
// made efficient.
type Hybrid struct {
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

// HybridOptions holds options for initializing an instance of Hybrid.
type HybridOptions struct {
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

// NewHybrid is a convenience function that allocates a Hybrid and calls Init on it.
func NewHybrid(o HybridOptions) *Hybrid {
	hybrid := new(Hybrid)
	hybrid.Init(o)
	return hybrid
}

// BufferNumBits returns the size of the Buffer in bits.
func (hybrid Hybrid) BufferNumBits() uint {
	return uint(hybrid.bbits)
}

// WindowNumBits returns the size of the Window in bits.
func (hybrid Hybrid) WindowNumBits() uint {
	return uint(hybrid.wbits)
}

// HashNumBits returns the size of the hash function output in bits.
func (hybrid Hybrid) HashNumBits() uint {
	return uint(hybrid.hbits)
}

// IsEmpty returns true iff the Hybrid's Buffer is empty.
func (hybrid Hybrid) IsEmpty() bool {
	return hybrid.i == hybrid.j
}

// IsFull returns true iff the Hybrid's Buffer is full.
func (hybrid Hybrid) IsFull() bool {
	return (hybrid.j - hybrid.i) >= hybrid.bsize
}

// Len returns the number of bytes currently in the Hybrid's Buffer.
func (hybrid Hybrid) Len() uint {
	return uint(hybrid.j - hybrid.i)
}

// Init initializes a Hybrid.
func (hybrid *Hybrid) Init(o HybridOptions) {
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

	*hybrid = Hybrid{
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
		hybrid.hashMap = make(map[uint32]*[]uint32, maxDist)
		hybrid.windowUpdateRegion(0)
	}
}

// Clear clears all data in the entire Hybrid.
func (hybrid *Hybrid) Clear() {
	wsize := hybrid.wsize
	hybrid.i = wsize
	hybrid.j = wsize
	bzero.Uint8(hybrid.slice[wsize:])
	hybrid.WindowClear()
}

// WindowClear clears just the data in the Hybrid's Window.
func (hybrid *Hybrid) WindowClear() {
	wsize := hybrid.wsize
	i := hybrid.i
	start := (i - wsize)

	bzero.Uint8(hybrid.slice[start:i])
	for _, ptr := range hybrid.hashMap {
		*ptr = []uint32(nil)
	}
	hybrid.windowUpdateRegion(start)
}

// SetWindow replaces the Hybrid's Window with the given data.
func (hybrid *Hybrid) SetWindow(data []byte) {
	wsize := hybrid.wsize

	length := uint(len(data))
	if length > uint(wsize) {
		x := length - uint(wsize)
		data = data[x:]
		length = uint(wsize)
	}

	i := hybrid.i
	start := (i - wsize)
	offset := (i - uint32(length))

	slice := hybrid.slice
	bzero.Uint8(slice[start:offset])
	copy(slice[offset:i], data)
	for _, ptr := range hybrid.hashMap {
		*ptr = []uint32(nil)
	}
	hybrid.windowUpdateRegion(start)
}

// DebugString returns a detailed dump of the Hybrid's internal state.
func (hybrid Hybrid) DebugString() string {
	buf := takeStringsBuilder()
	defer giveStringsBuilder(buf)

	buf.WriteString("Hybrid(\n")

	bsize := hybrid.bsize
	wsize := hybrid.wsize
	minLen := hybrid.minLen
	maxLen := hybrid.maxLen
	maxDist := hybrid.maxDist

	slice := hybrid.slice
	i := hybrid.i
	j := hybrid.j
	n := uint32(len(slice))

	start := (i - wsize)
	matchStart := (i - maxDist)
	used := (j - i)

	fmt.Fprintf(buf, "\tcapacity = %d\n", n)
	fmt.Fprintf(buf, "\tbbits = %d\n", hybrid.bbits)
	fmt.Fprintf(buf, "\twbits = %d\n", hybrid.wbits)
	fmt.Fprintf(buf, "\thbits = %d\n", hybrid.hbits)
	fmt.Fprintf(buf, "\tminLen = %d\n", minLen)
	fmt.Fprintf(buf, "\tmaxLen = %d\n", maxLen)
	fmt.Fprintf(buf, "\tmaxDist = %d\n", maxDist)
	fmt.Fprintf(buf, "\thashMask = %#08x\n", hybrid.hashMask)
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
		ch := hybrid.slice[index]
		fmt.Fprintf(buf, "%s %02x", prefix, ch)
	}
	if i == j {
		buf.WriteString(" |")
	}
	buf.WriteString(" ]\n")

	if hybrid.hashMap != nil {
		buf.WriteString("\thashList = [")

		hashes := make([]uint32, 0, len(hybrid.hashMap))
		for hash := range hybrid.hashMap {
			hashes = append(hashes, hash)
		}
		sort.Sort(byUint32(hashes))

		for _, hash := range hashes {
			ptr := hybrid.hashMap[hash]
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

// GoString returns a brief dump of the Hybrid's internal state.
func (hybrid Hybrid) GoString() string {
	buf := takeStringsBuilder()
	defer giveStringsBuilder(buf)

	buf.WriteString("Hybrid(")
	fmt.Fprintf(buf, "bbits=%d, ", hybrid.bbits)
	fmt.Fprintf(buf, "wbits=%d, ", hybrid.wbits)
	fmt.Fprintf(buf, "hbits=%d, ", hybrid.hbits)
	fmt.Fprintf(buf, "minLen=%d, ", hybrid.minLen)
	fmt.Fprintf(buf, "maxLen=%d, ", hybrid.maxLen)
	fmt.Fprintf(buf, "maxDist=%d, ", hybrid.maxDist)
	fmt.Fprintf(buf, "bsize=%d, ", hybrid.bsize)
	fmt.Fprintf(buf, "wsize=%d, ", hybrid.wsize)
	fmt.Fprintf(buf, "i=%d, ", hybrid.i)
	fmt.Fprintf(buf, "j=%d, ", hybrid.j)
	fmt.Fprintf(buf, "start=%d, ", hybrid.i-hybrid.wsize)
	fmt.Fprintf(buf, "matchStart=%d", hybrid.i-hybrid.maxDist)
	buf.WriteString(")")

	return buf.String()
}

// String returns a plain-text description of the Hybrid.
func (hybrid Hybrid) String() string {
	return fmt.Sprintf("(window-buffer with %d bytes in the buffer)", hybrid.Len())
}

// PrepareBulkWrite obtains a slice into which the caller can write bytes.  See
// Buffer.PrepareBulkWrite for more details.
//
func (hybrid *Hybrid) PrepareBulkWrite(length uint) []byte {
	bsize := hybrid.bsize
	i := hybrid.i
	j := hybrid.j
	x := (j - i)
	y := bsize - x

	if length > uint(y) {
		length = uint(y)
	}

	hybrid.shift(uint32(length))
	j = hybrid.j
	k := j + uint32(length)
	return hybrid.slice[j:k]
}

// CommitBulkWrite completes the bulk write begun by the previous call to
// PrepareBulkWrite.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkWrite.
//
func (hybrid *Hybrid) CommitBulkWrite(length uint) {
	bsize := hybrid.bsize
	i := hybrid.i
	j := hybrid.j
	x := (j - i)
	y := bsize - x

	assert.Assertf(length <= uint(y), "length %d > available space %d", length, uint(y))

	hybrid.j = j + uint32(length)
	hybrid.windowUpdateRegion(j - hashLen)
}

// WriteByte writes a single byte to the Hybrid's Buffer.
func (hybrid *Hybrid) WriteByte(ch byte) error {
	bsize := hybrid.bsize
	i := hybrid.i
	j := hybrid.j
	x := (j - i)
	y := bsize - x

	if y == 0 {
		return ErrFull
	}

	hybrid.shift(1)
	j = hybrid.j
	hybrid.slice[j] = ch
	hybrid.j = j + 1
	hybrid.windowUpdateRegion(j - hashLen)
	return nil
}

// Write writes a slice of bytes to the Hybrid's Buffer.
func (hybrid *Hybrid) Write(data []byte) (int, error) {
	bsize := hybrid.bsize
	i := hybrid.i
	j := hybrid.j
	x := (j - i)
	y := bsize - x

	length := uint(len(data))
	var err error
	if length > uint(y) {
		length = uint(y)
		data = data[:length]
		err = ErrFull
	}

	hybrid.shift(uint32(length))
	j = hybrid.j
	k := j + uint32(length)
	copy(hybrid.slice[j:k], data)
	hybrid.j = k
	hybrid.windowUpdateRegion(j - hashLen)
	return int(length), err
}

// PrepareBulkRead obtains a slice from which the caller can read bytes.  See
// Buffer.PrepareBulkRead for more details.
//
func (hybrid *Hybrid) PrepareBulkRead(length uint) []byte {
	bsize := hybrid.bsize
	if length > uint(bsize) {
		length = uint(bsize)
	}

	i := hybrid.i
	j := hybrid.j
	k := i + uint32(length)
	if k > j {
		k = j
	}

	return hybrid.slice[i:k]
}

// CommitBulkRead completes the bulk read begun by the previous call to
// PrepareBulkRead.  The argument must be between 0 and the length of the
// slice returned by PrepareBulkRead.
//
func (hybrid *Hybrid) CommitBulkRead(length uint) {
	bsize := hybrid.bsize
	if length > uint(bsize) {
		length = uint(bsize)
	}

	i := hybrid.i
	j := hybrid.j
	k := i + uint32(length)

	assert.Assertf(k <= j, "length %d exceeds %d bytes of available data", length, j-i)

	hybrid.i = k
	hybrid.windowUpdateRegion(i)
}

// ReadByte reads a single byte, or returns ErrEmpty if the buffer is empty.
func (hybrid *Hybrid) ReadByte() (byte, error) {
	i := hybrid.i
	j := hybrid.j
	k := i + 1

	if k > j {
		return 0, ErrEmpty
	}

	ch := hybrid.slice[i]
	hybrid.i = k
	hybrid.windowUpdateRegion(i)
	return ch, nil
}

// Read reads a slice of bytes from the Hybrid's Buffer.  If the buffer is
// empty, ErrEmpty is returned.
func (hybrid *Hybrid) Read(data []byte) (int, error) {
	length := uint(len(data))
	if length == 0 {
		return 0, nil
	}

	bsize := hybrid.bsize
	if length > uint(bsize) {
		length = uint(bsize)
		data = data[:length]
	}

	i := hybrid.i
	j := hybrid.j
	k := i + uint32(length)
	if k > j {
		k = j
		length = uint(k - i)
		data = data[:length]
		if length == 0 {
			return 0, ErrEmpty
		}
	}

	copy(data, hybrid.slice[i:k])
	hybrid.i = k
	hybrid.windowUpdateRegion(i)
	return int(length), nil
}

// Advance moves a slice of bytes from the Hybrid's Buffer to its Window.  The
// nature of the slice depends on the Hybrid's prefix match settings, the
// contents of the Hybrid's Window, and the contents of the Hybrid's Buffer.
func (hybrid *Hybrid) Advance() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	i := hybrid.i
	j := hybrid.j
	n := uint32(len(hybrid.slice))
	bsize := hybrid.bsize
	wsize := hybrid.wsize
	minLen := hybrid.minLen
	maxLen := hybrid.maxLen
	maxDist := hybrid.maxDist
	hbits := hybrid.hbits

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
		assert.Assert(hybrid.hashMap == nil, "hashMap is unexpectedly non-nil")
	} else {
		assert.Assertf(minLen >= hashLen, "minLen %d > hashLen %d", minLen, hashLen)
		assert.NotNil(&hybrid.hashMap)
	}

	switch {
	case hybrid.maxLen == 0:
		return hybrid.advanceByte()
	case hybrid.hbits == 0:
		return hybrid.advanceNoHash()
	default:
		return hybrid.advanceStandard()
	}
}

func (hybrid *Hybrid) advanceByte() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	i := hybrid.i
	j := hybrid.j
	k := i + 1
	if k > j {
		return
	}

	buf = hybrid.slice[i:k]
	hybrid.i = k
	hybrid.windowUpdateRegion(i)
	return
}

func (hybrid *Hybrid) advanceNoHash() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	slice := hybrid.slice
	i := hybrid.i
	j := hybrid.j
	wsize := hybrid.wsize
	minLen := hybrid.minLen
	maxLen := hybrid.maxLen

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
		hybrid.i = k
		hybrid.windowUpdateRegion(i)
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
	hybrid.i = k
	hybrid.windowUpdateRegion(i)
	return
}

func (hybrid *Hybrid) advanceStandard() (buf []byte, matchDistance uint, matchLength uint, matchFound bool) {
	slice := hybrid.slice
	i := hybrid.i
	j := hybrid.j
	minLen := hybrid.minLen
	maxLen := hybrid.maxLen
	maxDist := hybrid.maxDist

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
		hash := hash4(slice[i:i+hashLen], hybrid.hashMask)
		ptr := hybrid.hashMap[hash]
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
	hybrid.i = uint32(k)
	hybrid.windowUpdateRegion(i)
	return
}

func (hybrid *Hybrid) windowUpdateRegion(index uint32) {
	if hybrid.hashMap == nil {
		return
	}

	slice := hybrid.slice
	i := hybrid.i
	j := hybrid.j

	matchStart := i - hybrid.maxDist
	if index < matchStart {
		index = matchStart
	}

	end := j - hashLen
	if end > i {
		end = i
	}

	for index < end {
		hash := hash4(slice[index:index+hashLen], hybrid.hashMask)

		var matches []uint32
		ptr := hybrid.hashMap[hash]
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
			hybrid.hashMap[hash] = ptr
		} else {
			*ptr = matches
		}

		index++
	}
}

func (hybrid *Hybrid) shift(n uint32) {
	wsize := hybrid.wsize
	slice := hybrid.slice
	i := hybrid.i
	j := hybrid.j
	k := j + n
	if k <= uint32(len(slice)) {
		return
	}

	start := (i - wsize)
	x := (j - start)
	copy(slice[0:x], slice[start:j])
	bzero.Uint8(slice[x:])
	hybrid.i = wsize
	hybrid.j = x

	for _, ptr := range hybrid.hashMap {
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

// Options returns a HybridOptions struct which can be used to construct a new
// Hybrid with the same settings.
func (hybrid Hybrid) Options() HybridOptions {
	return HybridOptions{
		BufferNumBits:       uint(hybrid.bbits),
		WindowNumBits:       uint(hybrid.wbits),
		HashNumBits:         uint(hybrid.hbits),
		MinMatchLength:      uint(hybrid.minLen),
		MaxMatchLength:      uint(hybrid.maxLen),
		MaxMatchDistance:    uint(hybrid.maxDist),
		HasMinMatchLength:   true,
		HasMaxMatchLength:   true,
		HasMaxMatchDistance: true,
	}
}

// Equal returns true iff the given HybridOptions is semantically equal to this one.
func (opts HybridOptions) Equal(other HybridOptions) bool {
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
