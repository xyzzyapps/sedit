//go:build windows

package main

import (
	"testing"
	"unsafe"
)

func TestOpenFileNameMatchesWin64(t *testing.T) {
	if unsafe.Sizeof(openFileName{}) != 152 {
		t.Fatalf("size = %d", unsafe.Sizeof(openFileName{}))
	}
}
