package buffer

import (
	"fmt"

	"github.com/chronos-tachyon/enumhelper"
)

// Error is the type for the error constants returned by this package.
type Error byte

const (
	// ErrEmpty is returned when reading from an empty Buffer.
	ErrEmpty Error = iota

	// ErrFull is returned when writing to a full Buffer.
	ErrFull

	// ErrBadDistance is returned when Window.LookupByte is called with a
	// distance that isn't contained within the Window.
	ErrBadDistance
)

var errorData = [...]enumhelper.EnumData{
	{GoName: "ErrEmpty"},
	{GoName: "ErrFull"},
	{GoName: "ErrBadDistance"},
}

var errorText = [...]string{
	"buffer is empty",
	"buffer is full",
	"given distance lies outside of sliding window",
}

// GoString returns the name of the Go constant.
func (err Error) GoString() string {
	return enumhelper.DereferenceEnumData("Error", errorData[:], uint(err)).GoName
}

// Error returns the error message for this error.
func (err Error) Error() string {
	return errorText[err]
}

var _ fmt.GoStringer = Error(0)
var _ error = Error(0)
