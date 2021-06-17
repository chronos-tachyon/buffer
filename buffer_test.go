package buffer

import (
	"testing"
)

func TestBuffer_Byte(t *testing.T) {
	var b Buffer
	b.Init(1)

	_, err := b.ReadByte()
	if err != ErrEmpty {
		t.Errorf("ReadByte returned wrong error:\n\texpect: [%v]\n\tactual: [%v]", ErrEmpty, err)
	}

	err = b.WriteByte('a')
	if err != nil {
		t.Errorf("WriteByte unexpectedly returned non-nil error: %v", err)
	}

	err = b.WriteByte('b')
	if err != nil {
		t.Errorf("WriteByte unexpectedly returned non-nil error: %v", err)
	}

	err = b.WriteByte('c')
	if err != ErrFull {
		t.Errorf("WriteByte returned wrong error:\n\texpect: [%v]\n\tactual: [%v]", ErrFull, err)
	}

	ch, err := b.ReadByte()
	if err != nil {
		t.Errorf("ReadByte unexpectedly returned non-nil error: %v", err)
	}
	if ch != 'a' {
		t.Errorf("ReadByte unexpectedly returned ch=%q, expected %q", rune(ch), rune('a'))
	}

	err = b.WriteByte('c')
	if err != nil {
		t.Errorf("WriteByte unexpectedly returned non-nil error: %v", err)
	}

	ch, err = b.ReadByte()
	if err != nil {
		t.Errorf("ReadByte unexpectedly returned non-nil error: %v", err)
	}
	if ch != 'b' {
		t.Errorf("ReadByte unexpectedly returned ch=%q, expected %q", ch, 'b')
	}

	ch, err = b.ReadByte()
	if err != nil {
		t.Errorf("ReadByte unexpectedly returned non-nil error: %v", err)
	}
	if ch != 'c' {
		t.Errorf("ReadByte unexpectedly returned ch=%q, expected %q", ch, 'c')
	}

	_, err = b.ReadByte()
	if err != ErrEmpty {
		t.Errorf("ReadByte returned wrong error:\n\texpect: [%v]\n\tactual: [%v]", ErrEmpty, err)
	}
}

func TestBuffer_Bytes(t *testing.T) {
	var b Buffer
	b.Init(1)

	err := b.WriteByte('0')
	if err != nil {
		t.Errorf("WriteByte unexpectedly returned non-nil error: %v", err)
	}

	nn, err := b.Write([]byte{'a'})
	if err != nil {
		t.Errorf("Write unexpectedly returned non-nil error: %v", err)
	}
	if nn != 1 {
		t.Errorf("Write unexpectedly returned nn=%d, expected %d", nn, 1)
	}

	ch, err := b.ReadByte()
	if err != nil {
		t.Errorf("ReadByte unexpectedly returned non-nil error: %v", err)
	}
	if ch != '0' {
		t.Errorf("ReadByte unexpectedly returned ch=%q, expected %q", ch, '0')
	}

	nn, err = b.Write([]byte{'b'})
	if err != nil {
		t.Errorf("Write unexpectedly returned non-nil error: %v", err)
	}
	if nn != 1 {
		t.Errorf("Write unexpectedly returned nn=%d, expected %d", nn, 1)
	}

	var tmp [4]byte
	nn, err = b.Read(tmp[:])
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
