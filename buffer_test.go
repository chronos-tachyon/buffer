package buffer

import (
	"testing"
)

func TestBuffer_Byte(t *testing.T) {
	var buffer Buffer
	buffer.Init(1)

	_, err := buffer.ReadByte()
	if err != ErrEmpty {
		t.Errorf("ReadByte returned wrong error:\n\texpect: [%v]\n\tactual: [%v]", ErrEmpty, err)
	}

	err = buffer.WriteByte('a')
	if err != nil {
		t.Errorf("WriteByte unexpectedly returned non-nil error: %v", err)
	}

	err = buffer.WriteByte('b')
	if err != nil {
		t.Errorf("WriteByte unexpectedly returned non-nil error: %v", err)
	}

	err = buffer.WriteByte('c')
	if err != ErrFull {
		t.Errorf("WriteByte returned wrong error:\n\texpect: [%v]\n\tactual: [%v]", ErrFull, err)
	}

	ch, err := buffer.ReadByte()
	if err != nil {
		t.Errorf("ReadByte unexpectedly returned non-nil error: %v", err)
	}
	if ch != 'a' {
		t.Errorf("ReadByte unexpectedly returned ch=%q, expected %q", rune(ch), rune('a'))
	}

	err = buffer.WriteByte('c')
	if err != nil {
		t.Errorf("WriteByte unexpectedly returned non-nil error: %v", err)
	}

	ch, err = buffer.ReadByte()
	if err != nil {
		t.Errorf("ReadByte unexpectedly returned non-nil error: %v", err)
	}
	if ch != 'b' {
		t.Errorf("ReadByte unexpectedly returned ch=%q, expected %q", ch, 'b')
	}

	ch, err = buffer.ReadByte()
	if err != nil {
		t.Errorf("ReadByte unexpectedly returned non-nil error: %v", err)
	}
	if ch != 'c' {
		t.Errorf("ReadByte unexpectedly returned ch=%q, expected %q", ch, 'c')
	}

	_, err = buffer.ReadByte()
	if err != ErrEmpty {
		t.Errorf("ReadByte returned wrong error:\n\texpect: [%v]\n\tactual: [%v]", ErrEmpty, err)
	}
}

func TestBuffer_Bytes(t *testing.T) {
	var buffer Buffer
	buffer.Init(1)

	err := buffer.WriteByte('0')
	if err != nil {
		t.Errorf("WriteByte unexpectedly returned non-nil error: %v", err)
	}

	nn, err := buffer.Write([]byte{'a'})
	if err != nil {
		t.Errorf("Write unexpectedly returned non-nil error: %v", err)
	}
	if nn != 1 {
		t.Errorf("Write unexpectedly returned nn=%d, expected %d", nn, 1)
	}

	ch, err := buffer.ReadByte()
	if err != nil {
		t.Errorf("ReadByte unexpectedly returned non-nil error: %v", err)
	}
	if ch != '0' {
		t.Errorf("ReadByte unexpectedly returned ch=%q, expected %q", ch, '0')
	}

	nn, err = buffer.Write([]byte{'b'})
	if err != nil {
		t.Errorf("Write unexpectedly returned non-nil error: %v", err)
	}
	if nn != 1 {
		t.Errorf("Write unexpectedly returned nn=%d, expected %d", nn, 1)
	}

	var tmp [4]byte
	nn, err = buffer.Read(tmp[:])
	if err != nil {
		t.Errorf("Read unexpectedly returned non-nil error: %v", err)
	}
	if nn != 2 {
		t.Errorf("Read unexpectedly returned nn=%d, expected %d", nn, 2)
	}
	if expect := [4]byte{'a', 'b', 0, 0}; tmp != expect {
		t.Errorf("Read unexpectedly filled buffer with wrong data:\n\texpect: %#v\n\tactual: %#v", expect, tmp)
	}
}

func BenchmarkBuffer_WriteByte_2(b *testing.B) {
	var buffer Buffer
	buffer.Init(2)
	for n := 0; n < b.N; n++ {
		err := buffer.WriteByte('a')
		if err == ErrFull {
			tmp := buffer.PrepareBulkRead(1 << 2)
			buffer.CommitBulkRead(uint(len(tmp)))
		}
	}
}

func BenchmarkBuffer_WriteByte_8(b *testing.B) {
	var buffer Buffer
	buffer.Init(8)
	for n := 0; n < b.N; n++ {
		err := buffer.WriteByte('a')
		if err == ErrFull {
			tmp := buffer.PrepareBulkRead(1 << 8)
			buffer.CommitBulkRead(uint(len(tmp)))
		}
	}
}

func BenchmarkBuffer_WriteByte_15(b *testing.B) {
	var buffer Buffer
	buffer.Init(15)
	for n := 0; n < b.N; n++ {
		err := buffer.WriteByte('a')
		if err == ErrFull {
			tmp := buffer.PrepareBulkRead(1 << 15)
			buffer.CommitBulkRead(uint(len(tmp)))
		}
	}
}

func BenchmarkBuffer_WriteByte_16(b *testing.B) {
	var buffer Buffer
	buffer.Init(16)
	for n := 0; n < b.N; n++ {
		err := buffer.WriteByte('a')
		if err == ErrFull {
			tmp := buffer.PrepareBulkRead(1 << 16)
			buffer.CommitBulkRead(uint(len(tmp)))
		}
	}
}

func BenchmarkBuffer_WriteByte_24(b *testing.B) {
	var buffer Buffer
	buffer.Init(24)
	for n := 0; n < b.N; n++ {
		err := buffer.WriteByte('a')
		if err == ErrFull {
			tmp := buffer.PrepareBulkRead(1 << 24)
			buffer.CommitBulkRead(uint(len(tmp)))
		}
	}
}
