package buffer

import (
	"testing"
)

func TestBuffer(t *testing.T) {
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
		t.Errorf("ReadByte unexpectedly returned ch=%c, expected %c", ch, 'b')
	}

	ch, err = b.ReadByte()
	if err != nil {
		t.Errorf("ReadByte unexpectedly returned non-nil error: %v", err)
	}
	if ch != 'c' {
		t.Errorf("ReadByte unexpectedly returned ch=%c, expected %c", ch, 'c')
	}

	_, err = b.ReadByte()
	if err != ErrEmpty {
		t.Errorf("ReadByte returned wrong error:\n\texpect: [%v]\n\tactual: [%v]", ErrEmpty, err)
	}
}
