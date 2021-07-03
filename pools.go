package buffer

import (
	"strings"
	"sync"

	"github.com/chronos-tachyon/assert"
)

var gPoolStringsBuilder = sync.Pool{
	New: func() interface{} {
		sb := new(strings.Builder)
		sb.Grow(256)
		return sb
	},
}

func takeStringsBuilder() *strings.Builder {
	return gPoolStringsBuilder.Get().(*strings.Builder)
}

func giveStringsBuilder(sb *strings.Builder) {
	assert.NotNil(&sb)
	sb.Reset()
	gPoolStringsBuilder.Put(sb)
}
