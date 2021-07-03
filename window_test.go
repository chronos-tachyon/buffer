package buffer

import (
	"testing"
)

func BenchmarkWindow_WriteByte_1(b *testing.B) {
	var window Window
	window.Init(1)
	for n := 0; n < b.N; n++ {
		_ = window.WriteByte('a')
	}
}

func BenchmarkWindow_WriteByte_8(b *testing.B) {
	var window Window
	window.Init(8)
	for n := 0; n < b.N; n++ {
		_ = window.WriteByte('a')
	}
}

func BenchmarkWindow_WriteByte_15(b *testing.B) {
	var window Window
	window.Init(15)
	for n := 0; n < b.N; n++ {
		_ = window.WriteByte('a')
	}
}
