#!/usr/bin/env python3
"""Bridge Bilibili / Douyin danmaku into CyberVerse offline-live sessions."""

from __future__ import annotations

import argparse
import asyncio
import copy
import json
import sys
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "live-danmaku"))

from engine_adapter import Danmaku  # noqa: E402


class CyberverseEngine:
    def __init__(self, session_id: str, api_base: str) -> None:
        self.session_id = session_id
        self.api_base = api_base.rstrip("/")
        self.stats: Counter = Counter()

    def submit(self, message: Danmaku, source: str = "bilibili") -> str:
        payload = {
            "username": message.username or "观众",
            "text": message.text,
            "source": source,
            "message_id": message.message_id,
        }
        url = f"{self.api_base}/api/v1/sessions/{self.session_id}/danmaku/inject"
        try:
            import urllib.request

            req = urllib.request.Request(
                url,
                data=json.dumps(payload).encode("utf-8"),
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            with urllib.request.urlopen(req, timeout=8) as resp:
                if resp.status >= 300:
                    return "error"
        except Exception:
            self.record("listener_error", error="inject_failed")
            return "error"
        return "received"

    def record(self, state: str, **data) -> None:
        if state == "listener_error":
            print(f"[danmaku-worker] {state}: {data}", file=sys.stderr, flush=True)


async def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True, type=Path)
    args = parser.parse_args()
    raw = json.loads(args.config.read_text(encoding="utf-8"))
    session_id = str(raw.get("session_id", "")).strip()
    api_base = str(raw.get("api_base", "http://127.0.0.1:6006")).strip()
    if not session_id:
        print("session_id required", file=sys.stderr)
        return 1

    config = {
        "live": {
            "platform": raw.get("live", {}).get("platform", ""),
            "douyin_room": raw.get("live", {}).get("douyin_room", ""),
            "douyin_relay": raw.get("live", {}).get("douyin_relay", "http://127.0.0.1:8757"),
        },
        "bilibili": {
            "mode": raw.get("bilibili", {}).get("mode", "web"),
            "room_id": int(raw.get("bilibili", {}).get("room_id") or 0),
        },
    }
    platform = config["live"]["platform"]
    engine = CyberverseEngine(session_id, api_base)
    listener = None
    try:
        if platform == "bilibili":
            from bilibili import Listener as BilibiliListener  # noqa: E402

            listener = BilibiliListener(copy.deepcopy(config), engine)
        elif platform == "douyin":
            from douyin import Listener as DouyinListener  # noqa: E402

            listener = DouyinListener(copy.deepcopy(config), engine)
        else:
            print(f"unsupported platform: {platform}", file=sys.stderr)
            return 1
        await listener.start()
        while True:
            await asyncio.sleep(3600)
    except asyncio.CancelledError:
        pass
    finally:
        if listener is not None:
            await listener.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main()))
