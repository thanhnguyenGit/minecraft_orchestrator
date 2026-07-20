package wire

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
)

// CFB8 is the byte-feedback AES stream cipher used by Minecraft after login
// encryption begins. Each direction needs its own CFB8 instance.
type CFB8 struct {
	block    cipher.Block
	feedback [aes.BlockSize]byte
	decrypt  bool
}

// EncryptedReader decrypts a Minecraft protocol byte stream.
type EncryptedReader struct {
	reader io.Reader
	stream *CFB8
}

// EncryptedWriter encrypts a Minecraft protocol byte stream.
type EncryptedWriter struct {
	writer io.Writer
	stream *CFB8
}

// NewCFB8Encrypter creates a Minecraft AES-128-CFB8 encrypting stream.
func NewCFB8Encrypter(secret []byte) (*CFB8, error) {
	return newCFB8(secret, false)
}

// NewCFB8Decrypter creates a Minecraft AES-128-CFB8 decrypting stream.
func NewCFB8Decrypter(secret []byte) (*CFB8, error) {
	return newCFB8(secret, true)
}

// NewEncryptedReader wraps reader with Minecraft AES-128-CFB8 decryption.
func NewEncryptedReader(reader io.Reader, secret []byte) (*EncryptedReader, error) {
	stream, err := NewCFB8Decrypter(secret)
	if err != nil {
		return nil, err
	}
	return &EncryptedReader{reader: reader, stream: stream}, nil
}

// NewEncryptedWriter wraps writer with Minecraft AES-128-CFB8 encryption.
func NewEncryptedWriter(writer io.Writer, secret []byte) (*EncryptedWriter, error) {
	stream, err := NewCFB8Encrypter(secret)
	if err != nil {
		return nil, err
	}
	return &EncryptedWriter{writer: writer, stream: stream}, nil
}

func (r *EncryptedReader) Read(data []byte) (int, error) {
	n, err := r.reader.Read(data)
	if n > 0 {
		r.stream.XORKeyStream(data[:n], data[:n])
	}
	return n, err
}

func (w *EncryptedWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	encrypted := make([]byte, len(data))
	trial := *w.stream
	trial.XORKeyStream(encrypted, data)
	n, err := w.writer.Write(encrypted)
	if n < 0 || n > len(data) {
		return 0, fmt.Errorf("invalid encrypted write length: %d", n)
	}
	if n > 0 {
		committed := make([]byte, n)
		w.stream.XORKeyStream(committed, data[:n])
	}
	return n, err
}

func newCFB8(secret []byte, decrypt bool) (*CFB8, error) {
	if len(secret) != aes.BlockSize {
		return nil, fmt.Errorf("Minecraft shared secret must be %d bytes: %d", aes.BlockSize, len(secret))
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	stream := &CFB8{block: block, decrypt: decrypt}
	copy(stream.feedback[:], secret)
	return stream, nil
}

// XORKeyStream encrypts or decrypts src into dst. It supports in-place use.
func (s *CFB8) XORKeyStream(dst, src []byte) {
	if len(dst) < len(src) {
		panic("output smaller than input")
	}
	var block [aes.BlockSize]byte
	for i, source := range src {
		s.block.Encrypt(block[:], s.feedback[:])
		output := source ^ block[0]
		dst[i] = output

		copy(s.feedback[:], s.feedback[1:])
		if s.decrypt {
			s.feedback[aes.BlockSize-1] = source
		} else {
			s.feedback[aes.BlockSize-1] = output
		}
	}
}
