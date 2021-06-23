package buffer

import (
	"fmt"
	"math"
	"math/bits"
	"strings"

	"github.com/chronos-tachyon/assert"
)

const hashLen = 4
const hashLenSubOne = hashLen - 1

// Hybrid implements a combination Window/Buffer that uses the Window to
// remember bytes that were recently removed from the Buffer, and that hashes
// all data that enters the Window so that LZ77-style prefix matching can be
// made efficient.
type Hybrid struct {
	window   Window
	buffer   Buffer
	hashHead []uint32
	hashPrev []uint32
	hashNext []uint32
	hashRev  []uint32
	hashMask uint32
	maxMatch uint16
	hbits    byte
}

// HybridOptions holds options for initializing an instance of Hybrid.
type HybridOptions struct {
	WindowNumBits  byte
	BufferNumBits  byte
	HashNumBits    byte
	MaxMatchLength uint16
}

// NewHybrid is a convenience function that allocates a Hybrid and calls Init on it.
func NewHybrid(o HybridOptions) *Hybrid {
	hybrid := new(Hybrid)
	hybrid.Init(o)
	return hybrid
}

// Init initializes a Hybrid.
func (hybrid *Hybrid) Init(o HybridOptions) {
	assert.Assertf(o.WindowNumBits <= 31, "WindowNumBits %d must not exceed 31", o.WindowNumBits)
	assert.Assertf(o.BufferNumBits <= 31, "BufferNumBits %d must not exceed 31", o.BufferNumBits)
	assert.Assertf(o.HashNumBits <= 31, "HashNumBits %d must not exceed 31", o.HashNumBits)
	assert.Assertf(o.MaxMatchLength >= hashLen, "MaxMatchLength %d must be %d or greater", o.MaxMatchLength, hashLen)

	hybrid.window.Init(o.WindowNumBits)
	hybrid.buffer.Init(o.BufferNumBits)
	hybrid.hashHead = make([]uint32, 1<<o.HashNumBits)
	hybrid.hashPrev = make([]uint32, 1<<o.WindowNumBits)
	hybrid.hashNext = make([]uint32, 1<<o.WindowNumBits)
	hybrid.hashRev = make([]uint32, 1<<o.WindowNumBits)
	hybrid.hashMask = (uint32(1) << o.HashNumBits) - 1
	hybrid.maxMatch = o.MaxMatchLength
	hybrid.hbits = o.HashNumBits
	hybrid.Clear()
}

// Clear clears all data in the entire Hybrid.
func (hybrid *Hybrid) Clear() {
	hybrid.buffer.Clear()
	hybrid.WindowClear()
}

// WindowClear clears just the data in the Hybrid's Window.
func (hybrid *Hybrid) WindowClear() {
	hybrid.window.Clear()
	for index := range hybrid.hashHead {
		hybrid.hashHead[index] = math.MaxUint32
	}
	for index := range hybrid.hashPrev {
		hybrid.hashPrev[index] = math.MaxUint32
		hybrid.hashNext[index] = math.MaxUint32
		hybrid.hashRev[index] = math.MaxUint32
	}
}

// Window returns a copy of the Hybrid's Window.
func (hybrid Hybrid) Window() Window {
	return hybrid.window
}

// Buffer returns a copy of the Hybrid's Buffer.
func (hybrid Hybrid) Buffer() Buffer {
	return hybrid.buffer
}

// WindowNumBits returns the size of the Window in bits.
func (hybrid Hybrid) WindowNumBits() byte {
	return hybrid.window.NumBits()
}

// BufferNumBits returns the size of the Buffer in bits.
func (hybrid Hybrid) BufferNumBits() byte {
	return hybrid.buffer.NumBits()
}

// HashNumBits returns the size of the hash function output in bits.
func (hybrid Hybrid) HashNumBits() byte {
	return hybrid.hbits
}

// IsEmpty returns true iff the Hybrid's Buffer is empty.
func (hybrid Hybrid) IsEmpty() bool {
	return hybrid.buffer.IsEmpty()
}

// IsFull returns true iff the Hybrid's Buffer is full.
func (hybrid Hybrid) IsFull() bool {
	return hybrid.buffer.IsFull()
}

// DebugString returns a detailed dump of the Hybrid's internal state.
func (hybrid Hybrid) DebugString() string {
	var buf strings.Builder
	buf.WriteString("Hybrid(\n\t")

	buf.WriteString(hybrid.buffer.DebugString())
	buf.WriteString("\n\t")

	buf.WriteString(hybrid.window.DebugString())
	buf.WriteString("\n")

	i := uint(hybrid.window.i)
	j := uint(hybrid.window.j)
	if hybrid.window.busy && i >= j {
		j += hybrid.window.Cap()
	}
	buf.WriteString("\thashes = [")
	for i < j {
		iw := uint32(i) & hybrid.window.mask
		hash := hybrid.hashRev[iw]
		if hash != math.MaxUint32 {
			isHead := (hybrid.hashHead[hash] == iw)
			fmt.Fprintf(&buf, " [%d]:{Hash:%#x,IsHead:%t", iw, hash, isHead)
			if prev := hybrid.hashPrev[iw]; prev != math.MaxUint32 {
				fmt.Fprintf(&buf, ",Prev:%d", prev)
			}
			if next := hybrid.hashNext[iw]; next != math.MaxUint32 {
				fmt.Fprintf(&buf, ",Next:%d", next)
			}
			buf.WriteString("}")
		}

		i++
	}
	buf.WriteString(" ]\n")

	buf.WriteString(")\n")
	return buf.String()
}

// GoString returns a brief dump of the Hybrid's internal state.
func (hybrid Hybrid) GoString() string {
	return fmt.Sprintf("Hybrid(%#v,%#v)", hybrid.buffer, hybrid.window)
}

// String returns a plain-text description of the Hybrid.
func (hybrid Hybrid) String() string {
	return fmt.Sprintf("%s with %s", hybrid.buffer, hybrid.window)
}

// WriteByte writes a single byte to the Hybrid's Buffer.
func (hybrid *Hybrid) WriteByte(ch byte) error {
	bLenOld := hybrid.buffer.Len()
	err := hybrid.buffer.WriteByte(ch)
	if err == nil && bLenOld < hashLenSubOne {
		hybrid.windowUpdate()
	}
	return err
}

// Write writes a slice of bytes to the Hybrid's Buffer.
func (hybrid *Hybrid) Write(p []byte) (int, error) {
	bLenOld := hybrid.buffer.Len()
	nn, err := hybrid.buffer.Write(p)
	if err == nil && bLenOld < hashLenSubOne {
		hybrid.windowUpdate()
	}
	return nn, err
}

// SetWindow replaces the Hybrid's Window with the given data.
func (hybrid *Hybrid) SetWindow(p []byte) {
	var tmp [hashLenSubOne]byte
	q := hybrid.bufferTail(&tmp)
	hybrid.WindowClear()
	hybrid.windowInsert(p, q)
}

// Advance moves a slice of bytes from the Hybrid's Buffer to its Window.  The
// nature of the slice depends on the Hybrid's prefix match settings, the
// contents of the Hybrid's Window, and the contents of the Hybrid's Buffer.
func (hybrid *Hybrid) Advance() (buf []byte, bestDistance uint, bestLength uint, bestFound bool) {
	if !hybrid.buffer.busy {
		return
	}

	bCap := hybrid.buffer.Cap()
	bMask := hybrid.buffer.mask
	bSlice := hybrid.buffer.slice
	aw := hybrid.buffer.a
	bw := hybrid.buffer.b
	a := uint(aw)
	b := uint(bw)
	if hybrid.buffer.busy && aw >= bw {
		b += bCap
	}
	bUsed := (b - a)

	if bUsed < hashLen || !hybrid.window.busy {
		var tmp [hashLenSubOne]byte
		buf = hybrid.bufferRemove(1)
		hybrid.windowInsert(buf, hybrid.bufferTail(&tmp))
		return
	}

	p := make([]byte, 0, hybrid.maxMatch)
	for uint(len(p)) < hashLen {
		p = append(p, bSlice[aw])
		a++
		aw = (aw + 1) & bMask
	}

	hash := hash4(p, hybrid.hashMask)
	kw := hybrid.hashHead[hash]
	if kw == math.MaxUint32 {
		buf = hybrid.bufferRemove(1)
		hybrid.windowInsert(buf, p[1:])
		return
	}

	for a < b && uint(len(p)) < uint(hybrid.maxMatch) {
		p = append(p, bSlice[aw])
		a++
		aw = (aw + 1) & bMask
	}
	pLen := uint(len(p))

	wCap := hybrid.window.Cap()
	wMask := hybrid.window.mask
	wSlice := hybrid.window.slice
	iw := hybrid.window.i
	jw := hybrid.window.j
	j := uint(jw)
	if iw >= jw {
		j += wCap
	}
	k := uint(kw)
	if iw > kw {
		k += wCap
	}

	at := func(k uint) (byte, bool) {
		if k >= j {
			return p[k-j], true
		}
		kw := uint32(k) & wMask
		return wSlice[kw], true
	}

	for index := uint(0); index < pLen; index++ {
		ch, ok := at(k + index)
		if !ok || ch != p[index] {
			break
		}
		length := index + 1
		if length >= hashLen {
			bestDistance = (j - k)
			bestLength = length
			bestFound = true
		}
	}

	for bestLength < pLen {
		kw = hybrid.hashPrev[kw]
		if kw == math.MaxUint32 {
			break
		}

		k = uint(kw)
		if hybrid.window.busy && iw >= kw {
			k += wCap
		}

		if bestFound {
			ch, ok := at(k + bestLength)
			if !ok || ch != p[bestLength] {
				continue
			}
		}

		for index := uint(0); index < pLen; index++ {
			ch, ok := at(k + index)
			if !ok || ch != p[index] {
				break
			}
			length := index + 1
			if length >= hashLen && (!bestFound || length > bestLength) {
				bestDistance = (j - k)
				bestLength = length
				bestFound = true
			}
		}
	}

	if !bestFound {
		buf = hybrid.bufferRemove(1)
		hybrid.windowInsert(buf, p[1:])
		return
	}

	var tmp [hashLenSubOne]byte
	buf = hybrid.bufferRemove(bestLength)
	hybrid.windowInsert(buf, hybrid.bufferTail(&tmp))
	return
}

func (hybrid *Hybrid) bufferRemove(length uint) []byte {
	bLen := hybrid.buffer.Len()
	assert.Assertf(length <= bLen, "transfer length %d > buffer length %d", length, bLen)

	p := make([]byte, length)
	nn, err := hybrid.buffer.Read(p)
	if err != nil {
		panic(err)
	}
	assert.Assertf(uint(nn) == length, "Buffer.Read returned %d bytes, expected %d bytes", nn, length)
	return p
}

func (hybrid *Hybrid) bufferTail(tmp *[hashLenSubOne]byte) []byte {
	q := (*tmp)[:0]
	if hybrid.buffer.busy {
		bMask := hybrid.buffer.mask
		bSlice := hybrid.buffer.slice
		aw := hybrid.buffer.a
		bw := hybrid.buffer.b
		n := uint(0)
		for n < hashLenSubOne {
			(*tmp)[n] = bSlice[aw]
			n++
			q = (*tmp)[:n]
			aw = (aw + 1) & bMask
			if aw == bw {
				break
			}
		}
	}
	return q
}

func (hybrid *Hybrid) windowInsert(p []byte, q []byte) {
	pLen := uint(len(p))
	qLen := uint(len(q))
	if pLen == 0 {
		return
	}
	if qLen > hashLenSubOne {
		qLen = hashLenSubOne
		q = q[:qLen]
	}

	wCap := hybrid.window.Cap()
	wMask := hybrid.window.mask
	if pLen > wCap {
		x := pLen - wCap
		p = p[x:]
		pLen = wCap
	}
	if pLen == wCap {
		hybrid.WindowClear()
	}

	pqLen := pLen + qLen
	var xLen uint
	if pqLen >= hashLen {
		xLen = pqLen - hashLenSubOne
	}

	iw := hybrid.window.i
	jw := hybrid.window.j
	i := uint(iw)
	j := uint(jw)
	if hybrid.window.busy && iw >= jw {
		j += wCap
	}
	wUsed := (j - i)
	wFree := (wCap - wUsed)

	if pLen > wFree {
		numRemove := pLen - wFree
		for numRemove > 0 {
			hash := hybrid.hashRev[iw]
			xw := hybrid.hashHead[hash]
			yw := hybrid.hashNext[iw]
			hybrid.hashPrev[iw] = math.MaxUint32
			hybrid.hashNext[iw] = math.MaxUint32
			hybrid.hashRev[iw] = math.MaxUint32
			if xw == iw {
				hybrid.hashHead[hash] = math.MaxUint32
			}
			if yw != math.MaxUint32 {
				hybrid.hashPrev[yw] = math.MaxUint32
			}

			numRemove--
			i++
			iw = (iw + 1) & wMask
		}
		hybrid.window.i = iw
	}

	kw := jw
	index := uint(0)
	for index < pLen {
		hybrid.window.slice[kw] = p[index]
		index++
		kw = (kw + 1) & wMask
	}
	hybrid.window.j = kw
	hybrid.window.busy = true

	if xLen > 0 {
		kw = jw

		at := func(index uint) byte {
			if index < pLen {
				return p[index]
			}
			return q[index-pLen]
		}

		var tmp [4]byte
		tmp[0] = at(0)
		tmp[1] = at(1)
		tmp[2] = at(2)
		tmp[3] = at(3)

		hash := hash4(tmp[:], hybrid.hashMask)
		hybrid.setHash(kw, hash)

		index = 1
		kw = (kw + 1) & wMask
		for index < xLen {
			tmp[0] = tmp[1]
			tmp[1] = tmp[2]
			tmp[2] = tmp[3]
			tmp[3] = at(index + 3)
			hash = hash4(tmp[:], hybrid.hashMask)
			hybrid.setHash(kw, hash)

			index++
			kw = (kw + 1) & wMask
		}
	}
}

func (hybrid *Hybrid) windowUpdate() {
	if !hybrid.window.busy {
		return
	}

	wCap := hybrid.window.Cap()
	wMask := hybrid.window.mask
	wSlice := hybrid.window.slice
	iw := hybrid.window.i
	jw := hybrid.window.j
	i := uint(iw)
	j := uint(jw)
	if hybrid.window.busy && iw >= jw {
		j += wCap
	}
	wUsed := (j - i)

	kw3 := uint32(j-3) & wMask
	kw2 := uint32(j-2) & wMask
	kw1 := uint32(j-1) & wMask

	bCap := hybrid.buffer.Cap()
	bMask := hybrid.buffer.mask
	bSlice := hybrid.buffer.slice
	aw := hybrid.buffer.a
	bw := hybrid.buffer.b
	a := uint(aw)
	b := uint(bw)
	if hybrid.buffer.busy && aw >= bw {
		b += bCap
	}
	bUsed := (b - a)

	aw0 := uint32(a+0) & bMask
	aw1 := uint32(a+1) & bMask
	aw2 := uint32(a+2) & bMask

	var tmp [hashLen]byte

	if wUsed >= 3 && bUsed >= 1 && hybrid.hashRev[kw3] == math.MaxUint32 {
		tmp[0] = wSlice[kw3]
		tmp[1] = wSlice[kw2]
		tmp[2] = wSlice[kw1]
		tmp[3] = bSlice[aw0]
		hybrid.setHash(kw3, hash4(tmp[:], hybrid.hashMask))
	}

	if wUsed >= 2 && bUsed >= 2 && hybrid.hashRev[kw2] == math.MaxUint32 {
		tmp[0] = wSlice[kw2]
		tmp[1] = wSlice[kw1]
		tmp[2] = bSlice[aw0]
		tmp[3] = bSlice[aw1]
		hybrid.setHash(kw2, hash4(tmp[:], hybrid.hashMask))
	}

	if wUsed >= 1 && bUsed >= 3 && hybrid.hashRev[kw1] == math.MaxUint32 {
		tmp[0] = wSlice[kw1]
		tmp[1] = bSlice[aw0]
		tmp[2] = bSlice[aw1]
		tmp[3] = bSlice[aw2]
		hybrid.setHash(kw1, hash4(tmp[:], hybrid.hashMask))
	}
}

func (hybrid *Hybrid) setHash(kw uint32, hash uint32) {
	xw := hybrid.hashHead[hash]
	hybrid.hashHead[hash] = kw
	hybrid.hashPrev[kw] = xw
	hybrid.hashNext[kw] = math.MaxUint32
	hybrid.hashRev[kw] = hash
	if xw != math.MaxUint32 {
		hybrid.hashNext[xw] = kw
	}
}

// hash4 returns a hash of the first 4 bytes of p.
//
// It is based on CityHash32.  Reference:
//
//    https://github.com/google/cityhash/blob/master/src/city.cc
//
func hash4(p []byte, hashMask uint32) uint32 {
	const c1 = 0xcc9e2d51
	var b, c uint32
	b = uint32(p[0])
	c = uint32(9) ^ b
	b = uint32(int32(b*c1) + int32(int8(p[1])))
	c ^= b
	b = uint32(int32(b*c1) + int32(int8(p[2])))
	c ^= b
	b = uint32(int32(b*c1) + int32(int8(p[3])))
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
	return h*5 + 0x1b873593
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
