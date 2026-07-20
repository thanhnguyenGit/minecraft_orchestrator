package wire

import (
	"bytes"
	"testing"
)

func TestCFB8MatchesMinecraftEncryptionVector(t *testing.T) {
	secret := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	plain := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31}
	wantCiphertext := []byte{0x0a, 0x22, 0xf7, 0x96, 0xe1, 0xb9, 0x3e, 0x90, 0x32, 0xcf, 0xf8, 0x04, 0x83, 0x8a, 0xdf, 0xc3, 0xa5, 0xe4, 0xb3, 0xff, 0xdd, 0x47, 0x10, 0x85, 0x75, 0x53, 0x3e, 0x67, 0x2e, 0xf5, 0xd8, 0xef}

	encrypter, err := NewCFB8Encrypter(secret)
	if err != nil {
		t.Fatalf("NewCFB8Encrypter() error = %v", err)
	}
	ciphertext := make([]byte, len(plain))
	encrypter.XORKeyStream(ciphertext, plain)
	if !bytes.Equal(ciphertext, wantCiphertext) {
		t.Fatalf("ciphertext = %x, want %x", ciphertext, wantCiphertext)
	}

	decrypter, err := NewCFB8Decrypter(secret)
	if err != nil {
		t.Fatalf("NewCFB8Decrypter() error = %v", err)
	}
	decoded := make([]byte, len(ciphertext))
	decrypter.XORKeyStream(decoded, ciphertext)
	if !bytes.Equal(decoded, plain) {
		t.Fatalf("decoded = %x, want %x", decoded, plain)
	}
}
