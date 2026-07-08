#!/usr/bin/env python3
from __future__ import annotations

import argparse
import os
import signal
import subprocess
import sys
import threading
import time
from pathlib import Path
from typing import Mapping, Optional, Sequence


ROOT = Path(__file__).resolve().parents[1]
BOTS_DIR = ROOT / "bots"
ORCHESTRATOR_DIR = ROOT / "orchestrator"
DEFAULTS = {
    "REDIS_URL": "redis://localhost:6379/0",
    "BOT_ID": "king_crimson",
    "MINECRAFT_HOST": "192.168.31.170",
    "MINECRAFT_PORT": "64735",
    "MINECRAFT_USERNAME": "king_crimson_bot",
    "MINECRAFT_AUTH": "offline",
    "MINECRAFT_VERSION": "1.21.11",
}


class ManagedProcess:
    def __init__(
        self,
        name: str,
        args: Sequence[str],
        cwd: Path,
        env: Optional[Mapping[str, str]] = None,
    ) -> None:
        self.name = name
        self.args = list(args)
        self.cwd = cwd
        self.env = dict(env) if env is not None else None
        self.process: Optional[subprocess.Popen[str]] = None
        self._threads: list[threading.Thread] = []

    def start(self) -> None:
        self.process = subprocess.Popen(
            self.args,
            cwd=self.cwd,
            env=self.env,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,
        )
        assert self.process.stdout is not None
        assert self.process.stderr is not None
        self._threads = [
            threading.Thread(target=stream_lines, args=(self.name, self.process.stdout), daemon=True),
            threading.Thread(target=stream_lines, args=(self.name, self.process.stderr), daemon=True),
        ]
        for thread in self._threads:
            thread.start()

    def terminate(self, timeout_seconds: float = 10.0) -> None:
        if self.process is None or self.process.poll() is not None:
            return
        print(f"[runner] stopping {self.name}", flush=True)
        self.process.terminate()
        try:
            self.process.wait(timeout=timeout_seconds)
        except subprocess.TimeoutExpired:
            print(f"[runner] killing {self.name}", flush=True)
            self.process.kill()
            self.process.wait(timeout=timeout_seconds)

    def returncode(self) -> int | None:
        if self.process is None:
            return None
        return self.process.poll()


def stream_lines(prefix: str, stream) -> None:
    for line in iter(stream.readline, ""):
        print(f"[{prefix}] {line.rstrip()}", flush=True)


def run_command(
    prefix: str,
    args: Sequence[str],
    cwd: Path,
    env: Optional[Mapping[str, str]] = None,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    print(f"[runner] $ {' '.join(args)}", flush=True)
    process = subprocess.Popen(
        list(args),
        cwd=cwd,
        env=dict(env) if env is not None else None,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        bufsize=1,
    )
    assert process.stdout is not None
    for line in iter(process.stdout.readline, ""):
        print(f"[{prefix}] {line.rstrip()}", flush=True)
    returncode = process.wait()
    if check and returncode != 0:
        raise RuntimeError(f"{prefix} command failed with exit code {returncode}")
    return subprocess.CompletedProcess(args=list(args), returncode=returncode)


def parse_args() -> argparse.Namespace:
    load_dotenv(ROOT / ".env")
    parser = argparse.ArgumentParser(description="Run Redis, Mineflayer worker, and local Go connect command.")
    parser.add_argument("--bot-id", default=getenv("BOT_ID"))
    parser.add_argument("--host", default=getenv("MINECRAFT_HOST"))
    parser.add_argument("--port", type=int, default=int(getenv("MINECRAFT_PORT")))
    parser.add_argument("--username", default=getenv("MINECRAFT_USERNAME"))
    parser.add_argument("--auth", default=getenv("MINECRAFT_AUTH"))
    parser.add_argument("--version", default=getenv("MINECRAFT_VERSION"))
    parser.add_argument("--redis-url", default=getenv("REDIS_URL"))
    parser.add_argument("--no-connect", action="store_true", help="Start Redis and worker without sending connect.")
    parser.add_argument("--worker-start-delay", type=float, default=2.0)
    return parser.parse_args()


def getenv(key: str) -> str:
    return os.environ.get(key, DEFAULTS[key])


def load_dotenv(path: Path) -> None:
    if not path.exists():
        return

    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip("\"'")
        if key and key not in os.environ:
            os.environ[key] = value


def go_orchestrator_args(command: str, options: argparse.Namespace) -> list[str]:
    args = ["go", "run", "./cmd/orchestrator", command, "--bot-id", options.bot_id]
    if command == "connect":
        args.extend(
            [
                "--host",
                options.host,
                "--port",
                str(options.port),
                "--username",
                options.username,
                "--auth",
                options.auth,
                "--version",
                options.version,
            ]
        )
    return args


def main() -> int:
    options = parse_args()
    stop_requested = threading.Event()
    worker_env = os.environ.copy()
    worker_env.update({"BOT_ID": options.bot_id, "REDIS_URL": options.redis_url})
    go_env = os.environ.copy()
    go_env["REDIS_URL"] = options.redis_url

    worker = ManagedProcess("worker", ["npm", "run", "worker"], BOTS_DIR, worker_env)

    def request_stop(signum, _frame) -> None:
        print(f"[runner] received signal {signum}; shutting down", flush=True)
        stop_requested.set()

    signal.signal(signal.SIGINT, request_stop)
    signal.signal(signal.SIGTERM, request_stop)

    try:
        run_command("docker", ["docker", "compose", "up", "-d", "redis"], ROOT)
        worker.start()
        time.sleep(options.worker_start_delay)
        if worker.returncode() is not None:
            raise RuntimeError(f"worker exited early with exit code {worker.returncode()}")

        if not options.no_connect:
            run_command("go", go_orchestrator_args("connect", options), ORCHESTRATOR_DIR, go_env)

        print("[runner] running; press Ctrl+C to disconnect and stop services", flush=True)
        while not stop_requested.is_set():
            returncode = worker.returncode()
            if returncode is not None:
                print(f"[runner] worker exited with code {returncode}", flush=True)
                return returncode
            time.sleep(0.5)
        return 0
    except Exception as error:
        print(f"[runner] error: {error}", file=sys.stderr, flush=True)
        return 1
    finally:
        if worker.returncode() is None:
            try:
                run_command("go", go_orchestrator_args("disconnect", options), ORCHESTRATOR_DIR, go_env, check=False)
            except Exception as error:
                print(f"[runner] disconnect failed: {error}", file=sys.stderr, flush=True)
            worker.terminate()
        run_command("docker", ["docker", "compose", "stop", "redis"], ROOT, check=False)


if __name__ == "__main__":
    raise SystemExit(main())
