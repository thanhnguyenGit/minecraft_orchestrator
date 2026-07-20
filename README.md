# Minecraft Orchestrator

The Go orchestrator is a direct, headless Minecraft Java client. It connects to
one server over TCP, completes the offline login lifecycle, and prints inbound
protocol packets as readable console events.

## Go direct client

The active Go code is deliberately independent of Redis, protobuf, and the
TypeScript bot playground.

- `orchestrator/cmd/orchestrator`: executable that connects and prints events.
- `orchestrator/internal/config`: required connection settings from the
  environment.
- `orchestrator/internal/mc_protocol`: protocol framing, encryption, session
  lifecycle, and typed packet messages.
- `orchestrator/internal/engine`: simulation engine work in progress.

The current client is offline-only and fixed to Minecraft Java protocol `774`.
It does not use Microsoft authentication.

### Configuration

Copy `.env.example` to `.env` and set these required direct-client values:

```bash
MINECRAFT_HOST=192.168.31.170
MINECRAFT_PORT=64735
MINECRAFT_USERNAME=king_crimson_bot
```

The process fails before dialing if any of these values is missing or invalid.
The Go client does not read `MINECRAFT_AUTH`, `MINECRAFT_VERSION`, or
`REDIS_URL`.

### Run

```bash
cd orchestrator
go run ./cmd/orchestrator
```

It searches parent directories for `.env`, then prints incoming packets until
you stop it with `Ctrl-C`.

### Verify

```bash
cd orchestrator
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/orchestrator
```

In the sandbox, set `GOCACHE=/private/tmp/minecraft_orchestrator_go_cache` for
Go commands.

## Legacy Mineflayer/Redis playground

The TypeScript Mineflayer worker, Redis Streams contract, and protobuf schema
remain available as a separate testing playground. They are not used by the Go
direct client.

```bash
docker compose up -d redis
cd bots
npm install
npm run worker
```

`REDIS_URL`, `BOT_ID`, `MINECRAFT_AUTH`, and `MINECRAFT_VERSION` in
`.env.example` belong to this playground. The protobuf generator now produces
only the TypeScript bindings consumed by `bots`.
