# Minecraft Orchestrator

Skeleton for a Go orchestrator that talks to a Node.js Mineflayer worker through Redis Streams with protobuf payloads.

## Layout

- `orchestrator/cmd/orchestrator`: Go CLI that publishes bot commands.
- `orchestrator/internal/bus`: Redis stream names and publisher.
- `orchestrator/internal/commands`: protobuf command builders.
- `proto/orchestrator/v1`: shared protobuf contract.
- `bots/src`: TypeScript Mineflayer worker that consumes commands and emits events.

## Local Setup

```bash
docker compose up -d redis
cd bots
npm install
cd ..
buf generate
```

The current Mineflayer target is Minecraft Java `1.21.11`. A LAN server returning `26.2` / protocol `776` is intentionally out of scope until Mineflayer upstream supports it.

## Configuration

Copy `.env.example` to `.env` and update the current LAN port:

```bash
cp .env.example .env
```

Supported keys:

```bash
REDIS_URL=redis://localhost:6379/0
BOT_ID=king_crimson
MINECRAFT_HOST=192.168.31.170
MINECRAFT_PORT=64735
MINECRAFT_USERNAME=king_crimson_bot
MINECRAFT_AUTH=offline
MINECRAFT_VERSION=1.21.11
```

CLI flags override shell environment values, shell environment values override `.env`, and `.env` overrides built-in defaults.

## Run Local

Use the Python runner to start Redis, start one Mineflayer worker, and send the initial connect command:

```bash
python3 scripts/run_local.py
```

Press `Ctrl+C` to send disconnect, stop the worker, and stop Redis.

Useful options:

```bash
python3 scripts/run_local.py \
  --bot-id king_crimson \
  --host 192.168.31.170 \
  --port 64735 \
  --username king_crimson_bot \
  --auth offline \
  --version 1.21.11
```

## Run One Bot Worker

```bash
cd bots
npm run worker
```

The worker listens on `mc:bot:king_crimson:commands` and publishes events to `mc:events`.

## Send Commands

From the Go module directory:

```bash
cd orchestrator

go run ./cmd/orchestrator connect

go run ./cmd/orchestrator chat --message "hello from go"
go run ./cmd/orchestrator status
go run ./cmd/orchestrator disconnect
```

## Verify

```bash
cd orchestrator
go test ./cmd/... ./internal/...

cd bots
npm test
npm run build
```

In this sandbox, Go tests may need cache paths under `/private/tmp`:

```bash
cd orchestrator
GOCACHE=/private/tmp/mc-go-build GOPATH=/private/tmp/mc-go go test ./cmd/... ./internal/...
```
