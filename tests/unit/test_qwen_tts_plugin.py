import asyncio
import json
import sys
from types import SimpleNamespace
from unittest.mock import patch

import numpy as np
import pytest

from inference.core.types import PluginConfig, TTSRequestConfig
from inference.plugins.tts.qwen_tts_plugin import QwenTTSPlugin


class FakeCosyVoiceWebSocket:
    def __init__(self) -> None:
        self.sent: list[dict] = []
        self.messages: asyncio.Queue[str | bytes] = asyncio.Queue()
        self.closed = False

    async def send(self, payload: str) -> None:
        event = json.loads(payload)
        self.sent.append(event)
        action = event.get("header", {}).get("action")
        if action == "run-task":
            await self.messages.put(json.dumps({"header": {"event": "task-started"}}))
        if action == "finish-task":
            pcm = np.array([0, 32767], dtype=np.int16).tobytes()
            await self.messages.put(pcm)
            await self.messages.put(
                json.dumps({"header": {"event": "task-finished"}})
            )

    async def recv(self) -> str | bytes:
        return await self.messages.get()

    async def close(self) -> None:
        self.closed = True


class FakeWebSockets:
    def __init__(self) -> None:
        self.ws = FakeCosyVoiceWebSocket()
        self.connect_url = ""
        self.connect_headers: dict[str, str] = {}

    async def connect(self, url: str, **kwargs):
        self.connect_url = url
        self.connect_headers = kwargs.get("additional_headers") or kwargs.get(
            "extra_headers",
            {},
        )
        return self.ws


@pytest.mark.asyncio
async def test_cosyvoice_model_uses_task_protocol():
    plugin = QwenTTSPlugin()
    await plugin.initialize(
        PluginConfig(
            plugin_name="tts.qwen",
            params={
                "api_key": "dashscope-key",
                "model": "qwen3-tts-flash-realtime",
                "voice": "configured-voice",
                "sample_rate": 16000,
                "target_sample_rate": 16000,
                "rechunk_samples": 2,
                "cosyvoice_ws_url": "wss://cosy.example.com/api-ws/v1/inference",
            },
        )
    )
    fake_websockets = FakeWebSockets()

    async def text_stream():
        yield "  hello  "

    with patch.dict(
        sys.modules,
        {"websockets": SimpleNamespace(connect=fake_websockets.connect)},
    ):
        chunks = [
            chunk
            async for chunk in plugin.synthesize_stream(
                text_stream(),
                TTSRequestConfig(model="cosyvoice-v3.5-flash", voice="request-voice"),
            )
        ]

    assert fake_websockets.connect_url == "wss://cosy.example.com/api-ws/v1/inference"
    assert fake_websockets.connect_headers == {"Authorization": "Bearer dashscope-key"}
    assert fake_websockets.ws.closed is True
    assert [event["header"]["action"] for event in fake_websockets.ws.sent] == [
        "run-task",
        "continue-task",
        "finish-task",
    ]
    run_payload = fake_websockets.ws.sent[0]["payload"]
    assert run_payload["model"] == "cosyvoice-v3.5-flash"


def test_qwen3_tts_uses_realtime_protocol_not_cosyvoice():
    assert QwenTTSPlugin._uses_cosyvoice_protocol("qwen3-tts-flash-realtime") is False
    assert QwenTTSPlugin._uses_cosyvoice_protocol("cosyvoice-v3-flash") is True
    assert QwenTTSPlugin._uses_cosyvoice_protocol("qwen3.5-omni-flash-realtime") is True
