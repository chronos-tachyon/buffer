package buffer

import (
	"strings"
	"testing"
)

//nolint:gocyclo
func TestLZ77(t *testing.T) {
	var lz77 LZ77
	lz77.Init(LZ77Options{
		WindowNumBits:     3,
		BufferNumBits:     4,
		HashNumBits:       8,
		MaxMatchLength:    8,
		HasMaxMatchLength: true,
	})

	nn, err := lz77.Write([]byte("0123456789abcdef"))
	if err != nil {
		t.Fatalf("Write failed unexpectedly: %v", err)
	}
	if nn != 16 {
		t.Fatalf("Write returned wrong length: expect 16, got %d", nn)
	}

	lz77.SetWindow([]byte("cdef0123"))

	expectDebug := strings.Join([]string{
		"LZ77(\n",
		"\tcapacity = 40\n",
		"\tbbits = 4\n",
		"\twbits = 3\n",
		"\thbits = 8\n",
		"\tminLen = 4\n",
		"\tmaxLen = 8\n",
		"\tmaxDist = 8\n",
		"\thashMask = 0x000000ff\n",
		"\tbCap = 16\n",
		"\twCap = 8\n",
		"\th = 0\n",
		"\ti = 8\n",
		"\tj = 24\n",
		"\tlength = 16\n",
		"\tbytes = [ 63 64 65 66 30 31 32 33 | 30 31 32 33 34 35 36 37 38 39 61 62 63 64 65 66 ]\n",
		"\thashtable = [ 0x4e:[7 3] 0x64:[0] 0x7f:[6] 0xdb:[1] 0xe1:[5 2] 0xf0:[4] ]\n",
		")\n",
	}, "")
	actualDebug := lz77.DebugString()
	if actualDebug != expectDebug {
		t.Errorf("DebugString returned unexpected result.\n\tExpect: %s\n\tActual: %s", expectDebug, actualDebug)
	}

	buf, bestDistance, bestLength, bestFound := lz77.Advance()
	str := string(buf)
	if str != "0123" || bestDistance != 4 || bestLength != 4 || bestFound != true {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"0123", 4, 4, true,
			str, bestDistance, bestLength, bestFound,
		)
	}

	expectDebug = strings.Join([]string{
		"LZ77(\n",
		"\tcapacity = 40\n",
		"\tbbits = 4\n",
		"\twbits = 3\n",
		"\thbits = 8\n",
		"\tminLen = 4\n",
		"\tmaxLen = 8\n",
		"\tmaxDist = 8\n",
		"\thashMask = 0x000000ff\n",
		"\tbCap = 16\n",
		"\twCap = 8\n",
		"\th = 4\n",
		"\ti = 12\n",
		"\tj = 24\n",
		"\tlength = 12\n",
		"\tbytes = [ 30 31 32 33 30 31 32 33 | 34 35 36 37 38 39 61 62 63 64 65 66 ]\n",
		"\thashtable = [ 0x3d:[9] 0x4e:[7] 0x6a:[10] 0x7f:[6] 0xa3:[11] 0xe1:[5] 0xf0:[8 4] ]\n",
		")\n",
	}, "")
	actualDebug = lz77.DebugString()
	if actualDebug != expectDebug {
		t.Errorf("DebugString returned unexpected result.\n\tExpect: %s\n\tActual: %s", expectDebug, actualDebug)
	}

	lz77.Clear()
	nn, err = lz77.Write([]byte("0123012301230123"))
	if err != nil {
		t.Fatalf("Write failed unexpectedly: %v", err)
	}
	if nn != 16 {
		t.Fatalf("Write returned wrong length: expect 16, got %d", nn)
	}

	expectDebug = strings.Join([]string{
		"LZ77(\n",
		"\tcapacity = 40\n",
		"\tbbits = 4\n",
		"\twbits = 3\n",
		"\thbits = 8\n",
		"\tminLen = 4\n",
		"\tmaxLen = 8\n",
		"\tmaxDist = 8\n",
		"\thashMask = 0x000000ff\n",
		"\tbCap = 16\n",
		"\twCap = 8\n",
		"\th = 8\n",
		"\ti = 8\n",
		"\tj = 24\n",
		"\tlength = 16\n",
		"\tbytes = [ | 30 31 32 33 30 31 32 33 30 31 32 33 30 31 32 33 ]\n",
		"\thashtable = [ ]\n",
		")\n",
	}, "")
	actualDebug = lz77.DebugString()
	if actualDebug != expectDebug {
		t.Errorf("DebugString returned unexpected result.\n\tExpect: %s\n\tActual: %s", expectDebug, actualDebug)
	}

	buf, bestDistance, bestLength, bestFound = lz77.Advance()
	str = string(buf)
	if str != "0" || bestDistance != 0 || bestLength != 0 || bestFound != false {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"0", 0, 0, false,
			str, bestDistance, bestLength, bestFound,
		)
	}

	buf, bestDistance, bestLength, bestFound = lz77.Advance()
	str = string(buf)
	if str != "1" || bestDistance != 0 || bestLength != 0 || bestFound != false {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"1", 0, 0, false,
			str, bestDistance, bestLength, bestFound,
		)
	}

	buf, bestDistance, bestLength, bestFound = lz77.Advance()
	str = string(buf)
	if str != "2" || bestDistance != 0 || bestLength != 0 || bestFound != false {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"2", 0, 0, false,
			str, bestDistance, bestLength, bestFound,
		)
	}

	buf, bestDistance, bestLength, bestFound = lz77.Advance()
	str = string(buf)
	if str != "3" || bestDistance != 0 || bestLength != 0 || bestFound != false {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"3", 0, 0, false,
			str, bestDistance, bestLength, bestFound,
		)
	}

	expectDebug = strings.Join([]string{
		"LZ77(\n",
		"\tcapacity = 40\n",
		"\tbbits = 4\n",
		"\twbits = 3\n",
		"\thbits = 8\n",
		"\tminLen = 4\n",
		"\tmaxLen = 8\n",
		"\tmaxDist = 8\n",
		"\thashMask = 0x000000ff\n",
		"\tbCap = 16\n",
		"\twCap = 8\n",
		"\th = 8\n",
		"\ti = 12\n",
		"\tj = 24\n",
		"\tlength = 12\n",
		"\tbytes = [ 30 31 32 33 | 30 31 32 33 30 31 32 33 30 31 32 33 ]\n",
		"\thashtable = [ 0x4e:[11] 0x7f:[10] 0xe1:[9] 0xf0:[8] ]\n",
		")\n",
	}, "")
	actualDebug = lz77.DebugString()
	if actualDebug != expectDebug {
		t.Errorf("DebugString returned unexpected result.\n\tExpect: %s\n\tActual: %s", expectDebug, actualDebug)
	}

	buf, bestDistance, bestLength, bestFound = lz77.Advance()
	str = string(buf)
	if str != "01230123" || bestDistance != 4 || bestLength != 8 || bestFound != true {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"01230123", 4, 8, true,
			str, bestDistance, bestLength, bestFound,
		)
	}

	expectDebug = strings.Join([]string{
		"LZ77(\n",
		"\tcapacity = 40\n",
		"\tbbits = 4\n",
		"\twbits = 3\n",
		"\thbits = 8\n",
		"\tminLen = 4\n",
		"\tmaxLen = 8\n",
		"\tmaxDist = 8\n",
		"\thashMask = 0x000000ff\n",
		"\tbCap = 16\n",
		"\twCap = 8\n",
		"\th = 12\n",
		"\ti = 20\n",
		"\tj = 24\n",
		"\tlength = 4\n",
		"\tbytes = [ 30 31 32 33 30 31 32 33 | 30 31 32 33 ]\n",
		"\thashtable = [ 0x4e:[19 15] 0x7f:[18 14] 0xe1:[17 13] 0xf0:[16 12] ]\n",
		")\n",
	}, "")
	actualDebug = lz77.DebugString()
	if actualDebug != expectDebug {
		t.Errorf("DebugString returned unexpected result.\n\tExpect: %s\n\tActual: %s", expectDebug, actualDebug)
	}
}

func BenchmarkLZ77_WriteByte_8_8(b *testing.B) {
	var lz77 LZ77
	lz77.Init(LZ77Options{
		BufferNumBits: 8,
		WindowNumBits: 8,
		HashNumBits:   24,
	})
	for n := 0; n < b.N; n++ {
		err := lz77.WriteByte('a')
		if err == ErrFull {
			tmp := lz77.PrepareBulkRead(1 << 8)
			lz77.CommitBulkRead(uint(len(tmp)))
		}
	}
}

func BenchmarkLZ77_WriteByte_8_16(b *testing.B) {
	var lz77 LZ77
	lz77.Init(LZ77Options{
		BufferNumBits: 16,
		WindowNumBits: 8,
		HashNumBits:   24,
	})
	for n := 0; n < b.N; n++ {
		err := lz77.WriteByte('a')
		if err == ErrFull {
			tmp := lz77.PrepareBulkRead(1 << 16)
			lz77.CommitBulkRead(uint(len(tmp)))
		}
	}
}

func BenchmarkLZ77_WriteByte_15_16(b *testing.B) {
	var lz77 LZ77
	lz77.Init(LZ77Options{
		BufferNumBits: 16,
		WindowNumBits: 15,
		HashNumBits:   24,
	})
	for n := 0; n < b.N; n++ {
		err := lz77.WriteByte('a')
		if err == ErrFull {
			tmp := lz77.PrepareBulkRead(1 << 16)
			lz77.CommitBulkRead(uint(len(tmp)))
		}
	}
}

func BenchmarkLZ77_Advance_A(b *testing.B) {
	var lz77 LZ77
	lz77.Init(LZ77Options{
		BufferNumBits:       16,
		WindowNumBits:       8,
		HashNumBits:         24,
		MinMatchLength:      4,
		MaxMatchLength:      1 << 16,
		MaxMatchDistance:    1 << 8,
		HasMinMatchLength:   true,
		HasMaxMatchLength:   true,
		HasMaxMatchDistance: true,
	})
	for n := 0; n < b.N; n++ {
		tmp := lz77.PrepareBulkWrite(1 << 16)
		for index := range tmp {
			tmp[index] = 'a'
		}
		lz77.CommitBulkWrite(uint(len(tmp)))
		for {
			buf, _, _, _ := lz77.Advance()
			if buf == nil {
				break
			}
		}
	}
}

func BenchmarkLZ77_Advance_B(b *testing.B) {
	var lz77 LZ77
	lz77.Init(LZ77Options{
		BufferNumBits:       16,
		WindowNumBits:       15,
		HashNumBits:         24,
		MinMatchLength:      4,
		MaxMatchLength:      1 << 16,
		MaxMatchDistance:    1 << 15,
		HasMinMatchLength:   true,
		HasMaxMatchLength:   true,
		HasMaxMatchDistance: true,
	})
	for n := 0; n < b.N; n++ {
		tmp := lz77.PrepareBulkWrite(1 << 16)
		for index := range tmp {
			tmp[index] = 'a'
		}
		lz77.CommitBulkWrite(uint(len(tmp)))
		for {
			buf, _, _, _ := lz77.Advance()
			if buf == nil {
				break
			}
		}
	}
}

func BenchmarkLZ77_Advance_C(b *testing.B) {
	var lz77 LZ77
	lz77.Init(LZ77Options{
		BufferNumBits:       16,
		WindowNumBits:       15,
		HashNumBits:         24,
		MinMatchLength:      4,
		MaxMatchLength:      258,
		MaxMatchDistance:    1 << 15,
		HasMinMatchLength:   true,
		HasMaxMatchLength:   true,
		HasMaxMatchDistance: true,
	})
	for n := 0; n < b.N; n++ {
		tmp := lz77.PrepareBulkWrite(1 << 16)
		for index := range tmp {
			tmp[index] = 'a'
		}
		lz77.CommitBulkWrite(uint(len(tmp)))
		for {
			buf, _, _, _ := lz77.Advance()
			if buf == nil {
				break
			}
		}
	}
}
