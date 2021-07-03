package buffer

import (
	"strings"
	"testing"
)

func TestHybrid(t *testing.T) {
	var hybrid Hybrid
	hybrid.Init(HybridOptions{
		WindowNumBits:     3,
		BufferNumBits:     4,
		HashNumBits:       8,
		MaxMatchLength:    8,
		HasMaxMatchLength: true,
	})

	nn, err := hybrid.Write([]byte("0123456789abcdef"))
	if err != nil {
		t.Fatalf("Write failed unexpectedly: %v", err)
	}
	if nn != 16 {
		t.Fatalf("Write returned wrong length: expect 16, got %d", nn)
	}

	hybrid.SetWindow([]byte("cdef0123"))

	expectDebug := strings.Join([]string{
		"Hybrid(\n",
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
		"\ti = 8\n",
		"\tj = 24\n",
		"\tstart = 0\n",
		"\tmatchStart = 0\n",
		"\tlength = 16\n",
		"\tbytes = [ 63 64 65 66 30 31 32 33 | 30 31 32 33 34 35 36 37 38 39 61 62 63 64 65 66 ]\n",
		"\thashList = [ 0x003d:[7] 0x0055:[2] 0x0071:[0] 0x0087:[6] 0x00bd:[5] 0x00c4:[3] 0x00d1:[4] 0x00e0:[1] ]\n",
		")\n",
	}, "")
	actualDebug := hybrid.DebugString()
	if actualDebug != expectDebug {
		t.Errorf("DebugString returned unexpected result.\n\tExpect: %s\n\tActual: %s", expectDebug, actualDebug)
	}

	buf, bestDistance, bestLength, bestFound := hybrid.Advance()
	str := string(buf)
	if str != "0123" || bestDistance != 4 || bestLength != 4 || bestFound != true {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"0123", 4, 4, true,
			str, bestDistance, bestLength, bestFound,
		)
	}

	expectDebug = strings.Join([]string{
		"Hybrid(\n",
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
		"\ti = 12\n",
		"\tj = 24\n",
		"\tstart = 4\n",
		"\tmatchStart = 4\n",
		"\tlength = 12\n",
		"\tbytes = [ 30 31 32 33 30 31 32 33 | 34 35 36 37 38 39 61 62 63 64 65 66 ]\n",
		"\thashList = [ 0x0005:[10] 0x003d:[7] 0x0087:[6] 0x009a:[11] 0x009d:[9] 0x00bd:[5] 0x00d1:[4 8] ]\n",
		")\n",
	}, "")
	actualDebug = hybrid.DebugString()
	if actualDebug != expectDebug {
		t.Errorf("DebugString returned unexpected result.\n\tExpect: %s\n\tActual: %s", expectDebug, actualDebug)
	}

	hybrid.Clear()
	nn, err = hybrid.Write([]byte("0123012301230123"))
	if err != nil {
		t.Fatalf("Write failed unexpectedly: %v", err)
	}
	if nn != 16 {
		t.Fatalf("Write returned wrong length: expect 16, got %d", nn)
	}

	expectDebug = strings.Join([]string{
		"Hybrid(\n",
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
		"\ti = 8\n",
		"\tj = 24\n",
		"\tstart = 0\n",
		"\tmatchStart = 0\n",
		"\tlength = 16\n",
		"\tbytes = [ 00 00 00 00 00 00 00 00 | 30 31 32 33 30 31 32 33 30 31 32 33 30 31 32 33 ]\n",
		"\thashList = [ 0x0037:[7] 0x00d0:[0 1 2 3 4] 0x00d8:[6] 0x00fb:[5] ]\n",
		")\n",
	}, "")
	actualDebug = hybrid.DebugString()
	if actualDebug != expectDebug {
		t.Errorf("DebugString returned unexpected result.\n\tExpect: %s\n\tActual: %s", expectDebug, actualDebug)
	}

	buf, bestDistance, bestLength, bestFound = hybrid.Advance()
	str = string(buf)
	if str != "0" || bestDistance != 0 || bestLength != 0 || bestFound != false {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"0", 0, 0, false,
			str, bestDistance, bestLength, bestFound,
		)
	}

	buf, bestDistance, bestLength, bestFound = hybrid.Advance()
	str = string(buf)
	if str != "1" || bestDistance != 0 || bestLength != 0 || bestFound != false {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"1", 0, 0, false,
			str, bestDistance, bestLength, bestFound,
		)
	}

	buf, bestDistance, bestLength, bestFound = hybrid.Advance()
	str = string(buf)
	if str != "2" || bestDistance != 0 || bestLength != 0 || bestFound != false {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"2", 0, 0, false,
			str, bestDistance, bestLength, bestFound,
		)
	}

	buf, bestDistance, bestLength, bestFound = hybrid.Advance()
	str = string(buf)
	if str != "3" || bestDistance != 0 || bestLength != 0 || bestFound != false {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"3", 0, 0, false,
			str, bestDistance, bestLength, bestFound,
		)
	}

	expectDebug = strings.Join([]string{
		"Hybrid(\n",
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
		"\ti = 12\n",
		"\tj = 24\n",
		"\tstart = 4\n",
		"\tmatchStart = 4\n",
		"\tlength = 12\n",
		"\tbytes = [ 00 00 00 00 30 31 32 33 | 30 31 32 33 30 31 32 33 30 31 32 33 ]\n",
		"\thashList = [ 0x0037:[7] 0x003d:[11] 0x0087:[10] 0x00bd:[9] 0x00d0:[4] 0x00d1:[8] 0x00d8:[6] 0x00fb:[5] ]\n",
		")\n",
	}, "")
	actualDebug = hybrid.DebugString()
	if actualDebug != expectDebug {
		t.Errorf("DebugString returned unexpected result.\n\tExpect: %s\n\tActual: %s", expectDebug, actualDebug)
	}

	buf, bestDistance, bestLength, bestFound = hybrid.Advance()
	str = string(buf)
	if str != "01230123" || bestDistance != 4 || bestLength != 8 || bestFound != true {
		t.Errorf(
			"Advance returned unexpected data.\n\tExpect: %q, %d, %d, %t\n\tActual: %q, %d, %d, %t",
			"01230123", 4, 8, true,
			str, bestDistance, bestLength, bestFound,
		)
	}

	expectDebug = strings.Join([]string{
		"Hybrid(\n",
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
		"\ti = 20\n",
		"\tj = 24\n",
		"\tstart = 12\n",
		"\tmatchStart = 12\n",
		"\tlength = 4\n",
		"\tbytes = [ 30 31 32 33 30 31 32 33 | 30 31 32 33 ]\n",
		"\thashList = [ 0x003d:[15 19] 0x0087:[14 18] 0x00bd:[13 17] 0x00d1:[12 16] ]\n",
		")\n",
	}, "")
	actualDebug = hybrid.DebugString()
	if actualDebug != expectDebug {
		t.Errorf("DebugString returned unexpected result.\n\tExpect: %s\n\tActual: %s", expectDebug, actualDebug)
	}
}

func BenchmarkHybrid_WriteByte_8_8(b *testing.B) {
	var hybrid Hybrid
	hybrid.Init(HybridOptions{
		BufferNumBits: 8,
		WindowNumBits: 8,
		HashNumBits:   24,
	})
	for n := 0; n < b.N; n++ {
		err := hybrid.WriteByte('a')
		if err == ErrFull {
			tmp := hybrid.PrepareBulkRead(1 << 8)
			hybrid.CommitBulkRead(uint(len(tmp)))
		}
	}
}

func BenchmarkHybrid_WriteByte_8_16(b *testing.B) {
	var hybrid Hybrid
	hybrid.Init(HybridOptions{
		BufferNumBits: 16,
		WindowNumBits: 8,
		HashNumBits:   24,
	})
	for n := 0; n < b.N; n++ {
		err := hybrid.WriteByte('a')
		if err == ErrFull {
			tmp := hybrid.PrepareBulkRead(1 << 16)
			hybrid.CommitBulkRead(uint(len(tmp)))
		}
	}
}

func BenchmarkHybrid_WriteByte_15_16(b *testing.B) {
	var hybrid Hybrid
	hybrid.Init(HybridOptions{
		BufferNumBits: 16,
		WindowNumBits: 15,
		HashNumBits:   24,
	})
	for n := 0; n < b.N; n++ {
		err := hybrid.WriteByte('a')
		if err == ErrFull {
			tmp := hybrid.PrepareBulkRead(1 << 16)
			hybrid.CommitBulkRead(uint(len(tmp)))
		}
	}
}

func BenchmarkHybrid_Advance_A(b *testing.B) {
	var hybrid Hybrid
	hybrid.Init(HybridOptions{
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
		tmp := hybrid.PrepareBulkWrite(1 << 16)
		for index := range tmp {
			tmp[index] = 'a'
		}
		hybrid.CommitBulkWrite(uint(len(tmp)))
		for {
			buf, _, _, _ := hybrid.Advance()
			if buf == nil {
				break
			}
		}
	}
}

func BenchmarkHybrid_Advance_B(b *testing.B) {
	var hybrid Hybrid
	hybrid.Init(HybridOptions{
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
		tmp := hybrid.PrepareBulkWrite(1 << 16)
		for index := range tmp {
			tmp[index] = 'a'
		}
		hybrid.CommitBulkWrite(uint(len(tmp)))
		for {
			buf, _, _, _ := hybrid.Advance()
			if buf == nil {
				break
			}
		}
	}
}

func BenchmarkHybrid_Advance_C(b *testing.B) {
	var hybrid Hybrid
	hybrid.Init(HybridOptions{
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
		tmp := hybrid.PrepareBulkWrite(1 << 16)
		for index := range tmp {
			tmp[index] = 'a'
		}
		hybrid.CommitBulkWrite(uint(len(tmp)))
		for {
			buf, _, _, _ := hybrid.Advance()
			if buf == nil {
				break
			}
		}
	}
}
