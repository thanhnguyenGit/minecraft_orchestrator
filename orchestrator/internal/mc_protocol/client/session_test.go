package client

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

func TestSessionCompletesOfflineEncryptedLoginWhenServerRequestsAuthentication(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close() })

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	verifyToken := []byte{9, 8, 7, 6}
	serverErrors := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		reader := bufio.NewReader(serverConn)
		codec := wire.NewCodec()
		for range 2 {
			if _, err := codec.ReadPacket(reader); err != nil {
				serverErrors <- err
				return
			}
		}
		if err := codec.WritePacket(serverConn, encryptionRequestPacket(t, true, publicKey, verifyToken)); err != nil {
			serverErrors <- err
			return
		}
		response, err := codec.ReadPacket(reader)
		if err != nil {
			serverErrors <- err
			return
		}
		if response.ID != 0x01 {
			serverErrors <- fmt.Errorf("encryption response ID = %#x", response.ID)
			return
		}
		sharedSecret, encryptedToken, err := readEncryptionResponse(response.Body)
		if err != nil {
			serverErrors <- err
			return
		}
		secret, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, sharedSecret)
		if err != nil {
			serverErrors <- err
			return
		}
		token, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, encryptedToken)
		if err != nil {
			serverErrors <- err
			return
		}
		if len(secret) != 16 || !bytes.Equal(token, verifyToken) {
			serverErrors <- fmt.Errorf("shared secret/token validation failed")
			return
		}

		encryptedReader, err := wire.NewEncryptedReader(serverConn, secret)
		if err != nil {
			serverErrors <- err
			return
		}
		encryptedWriter, err := wire.NewEncryptedWriter(serverConn, secret)
		if err != nil {
			serverErrors <- err
			return
		}
		reader = bufio.NewReader(encryptedReader)
		if err := codec.WritePacket(encryptedWriter, wire.Packet{ID: 0x02}); err != nil {
			serverErrors <- err
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x03 {
			serverErrors <- fmt.Errorf("login acknowledgement = %#v, %w", packet, err)
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x00 {
			serverErrors <- fmt.Errorf("client information = %#v, %w", packet, err)
			return
		}
		if err := codec.WritePacket(encryptedWriter, wire.Packet{ID: 0x03}); err != nil {
			serverErrors <- err
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x03 {
			serverErrors <- fmt.Errorf("finish configuration acknowledgement = %#v, %w", packet, err)
			return
		}
		serverErrors <- nil
	}()

	session, err := newSessionWithDial(Config{Host: "127.0.0.1", Port: 25565, Username: "encrypted_bot"}, func(context.Context, string, string) (net.Conn, error) {
		return clientConn, nil
	})
	if err != nil {
		t.Fatalf("newSessionWithDial() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := session.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := <-session.Ready(); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	if err := <-serverErrors; err != nil {
		t.Fatalf("server error = %v", err)
	}
	if err := session.Wait(); !errors.Is(err, io.EOF) {
		t.Fatalf("Wait() error = %v, want EOF", err)
	}
}

func TestSessionAcceptsTypedPlayDataMessage(t *testing.T) {
	session := new(Session)
	if err := session.handlePlayMessage(SetCenterChunk{X: 0, Z: -1}); err != nil {
		t.Fatalf("handlePlayMessage(SetCenterChunk) error = %v", err)
	}
}

func encryptionRequestPacket(t *testing.T, shouldAuthenticate bool, publicKey, verifyToken []byte) wire.Packet {
	t.Helper()
	var body bytes.Buffer
	if err := wire.WriteString(&body, ""); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteByteArray(&body, publicKey); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteByteArray(&body, verifyToken); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteBool(&body, shouldAuthenticate); err != nil {
		t.Fatal(err)
	}
	return wire.Packet{ID: 0x01, Body: body.Bytes()}
}

func readEncryptionResponse(body []byte) ([]byte, []byte, error) {
	reader := bytes.NewReader(body)
	sharedSecret, err := wire.ReadByteArray(reader)
	if err != nil {
		return nil, nil, err
	}
	verifyToken, err := wire.ReadByteArray(reader)
	if err != nil {
		return nil, nil, err
	}
	return sharedSecret, verifyToken, wire.RequireEmpty(reader)
}
