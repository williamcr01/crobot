package plugins

import "testing"

func TestPackUnpackPtrLen(t *testing.T) {
	ptr := uint32(0x12345678)
	length := uint32(0x90abcdef)
	gotPtr, gotLen := unpackPtrLen(packPtrLen(ptr, length))
	if gotPtr != ptr || gotLen != length {
		t.Fatalf("unpackPtrLen(packPtrLen()) = (%#x, %#x), want (%#x, %#x)", gotPtr, gotLen, ptr, length)
	}
}
