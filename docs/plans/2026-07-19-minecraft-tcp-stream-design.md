# Minecraft TCP Stream Design

**Goal:** Give the Go protocol module a safe TCP transport boundary that can send a Java 1.21.11 handshake and read or write uncompressed Minecraft packet frames.

## Scope

Only `orchestrator/internal/mc_protocol/server/bytes.go` and `orchestrator/internal/mc_protocol/server/stream.go` implement production behavior. Focused Go tests may be added beside them.

The change does not implement Login Start, compression, encryption, configuration, Play-state dispatch, or Redis integration. Those are protocol layers above this transport boundary.

## Architecture

`bytes.go` owns the byte-level primitives:

- Minecraft signed VarInt encoding and decoding, limited to five bytes.
- Framed packet serialization: `VarInt(packet length) | VarInt(packet ID) | packet body`.
- Exact frame reads using `io.ReadFull`, with a configurable/bounded maximum frame size.
- Handshake payload encoding: packet ID `0`, protocol version `774`, host string, big-endian unsigned 16-bit port, and next state `2`.

`stream.go` owns the TCP resource:

- It dials a validated host/port using a caller-provided context and dial timeout.
- It wraps the connection in `bufio.Reader` for incremental VarInt/frame reads.
- It exposes packet write/read methods and an explicit handshake method.
- It closes idempotently and does not place one permanent deadline on a long-lived bot connection.

## Data Flow

```text
Connect TCP
  -> SendHandshake
  -> WritePacket(Login Start; later layer)
  <- ReadPacket
  -> raw packet ID and body returned to the caller
```

TCP segment boundaries are deliberately invisible to callers. A packet can arrive in partial reads or together with later packets; the frame reader consumes precisely one declared packet at a time.

## Errors and Boundaries

- Invalid port, nil configuration, malformed/oversized VarInts, invalid frame lengths, short reads, and use after close return errors.
- A successful `Connect` only establishes TCP. Protocol acceptance is established by a future Login Success packet.
- Compression begins only after the Login `set_compression` packet and is intentionally deferred so this first transport change stays small and testable.

## Test Strategy

- VarInt round trips plus invalid six-byte VarInt rejection.
- Handshake bytes exactly match the 1.21.11/protocol-774 example for `127.0.0.1:25565`.
- Frame reads work when a `net.Pipe` peer delivers the bytes in separate writes.
- A local TCP listener verifies connect, packet write/read, and close behaviour without requiring a LAN Minecraft server.
