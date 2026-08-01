# Minecraft Orchestrator

The Go process is an ECS metadata runtime for multiple Minecraft bots. A single
Node process hosts Mineflayer controllers, which own Minecraft connections,
chunks, collision, passive physics, knockback, and respawning.

```text
Mineflayer host
  -> framed protobuf observations
  -> Go HostSupervisor
  -> network.Inbox
  -> fixed ECS tick
  -> NetworkApply
  -> ECS bot metadata
```

The Mineflayer host never receives an ECS World pointer. `NetworkApply` is the
sole boundary that mutates ECS state from network observations.

## Configuration

Copy `.env.example` to `.env` and set the Minecraft server address:

```bash
MINECRAFT_HOST=192.168.31.170
MINECRAFT_PORT=64735
```

Optional settings are `MINEFLAYER_AUTH` (`offline` by default),
`MINEFLAYER_VERSION` (`1.21.11` by default), `MINEFLAYER_NODE_BINARY`, and
`MINEFLAYER_HOST_SCRIPT`. Set `MINEFLAYER_PHYSICS_DEBUG=true` only when
investigating Mineflayer passive-physics behavior.

## Run

```bash
cd orchestrator
go run ./cmd/orchestrator
```

Go starts the local Node host and configures the bot profiles automatically.

## Verify

```bash
cd orchestrator
go test ./...
go vet ./...

cd ../bots
npm test
npm run build
```
