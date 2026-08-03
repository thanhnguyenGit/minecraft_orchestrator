package hosttransport

import (
	"bytes"
	"testing"
)

func TestDecoderEmitsCompleteFramesAcrossReads(t *testing.T) {
	first := Encode([]byte{1, 2, 3})
	second := Encode([]byte{4, 5})
	decoder := NewDecoder(1024)

	if frames, err := decoder.Push(first[:5]); err != nil || len(frames) != 0 {
		t.Fatalf("first partial Push() = %#v, %v; want no frames", frames, err)
	}
	frames, err := decoder.Push(append(first[5:], second[:2]...))
	if err != nil || len(frames) != 1 || !bytes.Equal(frames[0], []byte{1, 2, 3}) {
		t.Fatalf("second Push() = %#v, %v; want first frame", frames, err)
	}
	frames, err = decoder.Push(second[2:])
	if err != nil || len(frames) != 1 || !bytes.Equal(frames[0], []byte{4, 5}) {
		t.Fatalf("third Push() = %#v, %v; want second frame", frames, err)
	}
}

func TestDecoderRejectsOversizedFrame(t *testing.T) {
	decoder := NewDecoder(3)
	_, err := decoder.Push([]byte{0, 0, 0, 4})
	if err == nil {
		t.Fatal("Push() error = nil, want oversized frame error")
	}
}
