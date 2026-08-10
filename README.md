# Minecraft Orchestrator

> Autonomous Minecraft bots that make survival decisions from their live surroundings.

Minecraft Orchestrator is an experimental survival-behaviour runtime for
Minecraft bots. Bots explore, seek useful visible resources, craft basic tools,
manage hunger, and respond to threats by fighting or fleeing.

## What it provides

- Bots that choose their next survival priority from the world around them
- Visible-resource gathering and basic crafting progression
- Hunger, combat, fleeing, and exploration behaviour
- A configurable runtime for connecting bots to a Minecraft server

## Why it exists

Rather than following one fixed task sequence, a bot continuously reassesses
what it can see and changes its behaviour as its needs and surroundings change.
The project explores how a Minecraft bot can make simple, reactive survival
decisions on its own.

In the longer term, the project aims to connect language models to bots and
explore a simulated Minecraft civilization: autonomous inhabitants with their
own goals, relationships, and decisions. Rather than following fixed scripts,
they could gather, build, trade, cooperate, compete, and shape the world
alongside real players. The aim is to make sparse survival servers feel more
alive through an emergent population whose behaviour is not wholly predictable.

## Current state

This is an early MVP under active development. Bots can connect to a Minecraft
server, wander, gather visible resources, craft basic tools, eat, fight, and
flee.

## Quick start

You need a reachable Minecraft Java server (1.21.11), Go 1.26, Node.js, and npm.

From the repository root:

```bash
cp .env.example .env
# Edit .env and set MINECRAFT_HOST and MINECRAFT_PORT.

cd bots
npm ci

cd ../orchestrator
go mod download
go run ./cmd/orchestrator
```

The orchestrator starts its bot host automatically. Stop it with `Ctrl-C`.

For authentication, protocol-version, and logging settings, see
[`.env.example`](.env.example).
