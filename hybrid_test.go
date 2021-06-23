package buffer

import (
	"strings"
	"testing"
)

func TestHybrid(t *testing.T) {
	var hybrid Hybrid
	hybrid.Init(HybridOptions{
		WindowNumBits:  3,
		BufferNumBits:  4,
		HashNumBits:    8,
		MaxMatchLength: 8,
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
		"\tBuffer(a=0,b=0,busy=true,[ 30 31 32 33 34 35 36 37 38 39 61 62 63 64 65 66 ])\n",
		"\tWindow(i=0,j=0,busy=true,[ 63 64 65 66 30 31 32 33 ])\n",
		"\thashes = [ ",
		"[0]:{Hash:0x71,IsHead:true} [1]:{Hash:0xe0,IsHead:true} ",
		"[2]:{Hash:0x55,IsHead:true} [3]:{Hash:0xc4,IsHead:true} ",
		"[4]:{Hash:0xd1,IsHead:true} [5]:{Hash:0xbd,IsHead:true} ",
		"[6]:{Hash:0x87,IsHead:true} [7]:{Hash:0x3d,IsHead:true} ]\n",
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
		"\tBuffer(a=4,b=0,busy=true,[ 34 35 36 37 38 39 61 62 63 64 65 66 ])\n",
		"\tWindow(i=4,j=4,busy=true,[ 30 31 32 33 30 31 32 33 ])\n",
		"\thashes = [ ",
		"[4]:{Hash:0xd1,IsHead:false,Next:0} [5]:{Hash:0xbd,IsHead:true} ",
		"[6]:{Hash:0x87,IsHead:true} [7]:{Hash:0x3d,IsHead:true} ",
		"[0]:{Hash:0xd1,IsHead:true,Prev:4} [1]:{Hash:0x9d,IsHead:true} ",
		"[2]:{Hash:0x5,IsHead:true} [3]:{Hash:0x9a,IsHead:true} ]\n",
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
		"\tBuffer(a=0,b=0,busy=true,[ 30 31 32 33 30 31 32 33 30 31 32 33 30 31 32 33 ])\n",
		"\tWindow(i=0,j=0,busy=false,[ ])\n",
		"\thashes = [ ]\n",
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
		"\tBuffer(a=4,b=0,busy=true,[ 30 31 32 33 30 31 32 33 30 31 32 33 ])\n",
		"\tWindow(i=0,j=4,busy=true,[ 30 31 32 33 ])\n",
		"\thashes = [ ",
		"[0]:{Hash:0xd1,IsHead:true} [1]:{Hash:0xbd,IsHead:true} ",
		"[2]:{Hash:0x87,IsHead:true} [3]:{Hash:0x3d,IsHead:true} ]\n",
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
}
