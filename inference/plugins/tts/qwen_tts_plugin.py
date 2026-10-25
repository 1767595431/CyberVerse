import asyncio
import base64
import json
import logging
import time
import uuid
from math import gcd
from typing import Any, AsyncIterator

import numpy as np

from inference.core.types import AudioChunk, PluginConfig, TTSRequestConfig
from inference.plugins.qwen_endpoint import (
    dashscope_cosyvoice_ws_url,
    dashscope_realtime_ws_url,
)
from inference.plugins.tts.base import AudioRechunker, TTSPlugin

logger = logging.getLogger(__name__)


class QwenTTSPlugin(TTSPlugin):
    """DashScope Qwen realtime TTS plugin with CosyVoice model support."""

    name = "tts.qwen"

    def __init__(self) -> None:
        self.api_key = ""
        self.model = "qwen3-tts-flash-realtime"
        self.ws_url = ""
        self.cosyvoice_ws_url = ""
        self.cosyvoice_model = "cosyvoice-v3-flash"
        self.voice = "Momo"
        self.sample_rate = 24000
        self.target_sample_rate = 16000
        self.rechunk_samples = 17920

    async def initialize(self, config: PluginConfig) -> None:
        self.api_key = config.params.get("api_key", "")
        self.model = config.params.get("model", self.model)
        self.ws_url = dashscope_realtime_ws_url(self.model, "DASHSCOPE_TTS_WS_URL")
        self.cosyvoice_ws_url = config.params.get(
            "cosyvoice_ws_url",
            "",
        ) or dashscope_cosyvoice_ws_url()
        self.cosyvoice_model = str(
            config.params.get("cosyvoice_model", self.cosyvoice_model)
        ).strip()
        self.voice = config.params.get("voice", self.voice)
        self.sample_rate = int(config.params.get("sample_rate", self.sample_rate))
        self.target_sample_rate = int(
            config.params.get("target_sample_rate", self.target_sample_rate)
        )
        self.rechunk_samples = int(
            config.params.get("rechunk_samples", self.rechunk_samples)
        )

    async def synthesize_stream(
        self,
        text_stream: AsyncIterator[str],
        request_config: TTSRequestConfig | None = None,
    ) -> AsyncIterator[AudioChunk]:
        import websockets

        model = self._request_model(request_config)
        if self._uses_cosyvoice_protocol(model):
            async for audio in self._synthesize_cosyvoice_stream(
                websockets,
                text_stream,
                model,
                request_config,
            ):
                yield audio
            return

        voice = (request_config.voice if request_config else "") or self.voice
        session_id = (request_config.session_id if request_config else "") or ""
        rechunker = AudioRechunker(
            chunk_samples=self.rechunk_samples,
            sample_rate=self.target_sample_rate,
        )

        ws = await self._connect(websockets, self._qwen_ws_url(model))
        try:
            await self._configure_session(ws, voice)

            async for text in text_stream:
                text = text.strip()
                if not text:
                    continue

                await self._send_json(
                    ws,
                    {
                        "type": "input_text_buffer.append",
                        "event_id": self._event_id(session_id),
                        "text": text,
                    },
                )
                await self._send_json(
                    ws,
                    {
                        "type": "input_text_buffer.commit",
                        "event_id": self._event_id(session_id, "commit"),
                    },
                )

                async for audio in self._receive_qwen_response_audio(ws, rechunker):
                    yield audio

            final_chunk = rechunker.flush()
            if final_chunk:
                yield final_chunk
        finally:
            await ws.close()

    async def _synthesize_cosyvoice_stream(
        self,
        websockets: Any,
        text_stream: AsyncIterator[str],
        model: str,
        request_config: TTSRequestConfig | None = None,
    ) -> AsyncIterator[AudioChunk]:
        voice = (request_config.voice if request_config else "") or self.voice
        if not voice.strip():
            raise ValueError(
                "CosyVoice TTS voice is required; use a cloned or designed voice ID."
            )
        voice = voice.strip()
        task_id = str(uuid.uuid4())
        rechunker = AudioRechunker(
            chunk_samples=self.rechunk_samples,
            sample_rate=self.target_sample_rate,
        )

        ws = await self._connect(websockets, self.cosyvoice_ws_url)
        try:
            cosy_model = self._cosyvoice_inference_model(model)
            await self._send_cosyvoice_run_task(ws, task_id, cosy_model, voice)
            await self._wait_for_cosyvoice_task_started(ws)

            async def send_text() -> None:
                try:
                    async for text in text_stream:
                        text = text.strip()
                        if not text:
                            continue
                        await self._send_cosyvoice_continue_task(ws, task_id, text)
                finally:
                    await self._send_cosyvoice_finish_task(ws, task_id)

            sender_task = asyncio.create_task(send_text())
            try:
                async for audio in self._receive_cosyvoice_response_audio(
                    ws,
                    rechunker,
                ):
                    yield audio
            finally:
                if not sender_task.done():
                    sender_task.cancel()
                try:
                    await sender_task
                except asyncio.CancelledError:
                    pass

            final_chunk = rechunker.flush()
            if final_chunk:
                yield final_chunk
        finally:
            await ws.close()

    async def _connect(self, websockets: Any, ws_url: str | None = None):
        headers = {"Authorization": f"Bearer {self.api_key}"}
        url = ws_url or self.ws_url
        try:
            return await websockets.connect(
                url,
                additional_headers=headers,
            )
        except TypeError:
            return await websockets.connect(
                url,
                extra_headers=headers,
            )

    async def _configure_session(self, ws: Any, voice: str) -> None:
        await self._send_json(
            ws,
            {
                "type": "session.update",
                "event_id": self._event_id("", "session"),
                "session": {
                    "mode": "server_commit",
                    "voice": voice or "Momo",
                    "response_format": "pcm",
                    "sample_rate": self.sample_rate,
                    "channels": 1,
                    "bit_depth": 16,
                },
            },
        )

        while True:
            event = json.loads(await ws.recv())
            event_type = event.get("type", "")
            if event_type in {"session.created", "session.updated"}:
                return
            if event_type == "error":
                raise RuntimeError(f"Qwen TTS session error: {event}")

    async def _send_cosyvoice_run_task(
        self,
        ws: Any,
        task_id: str,
        model: str,
        voice: str,
    ) -> None:
        await self._send_json(
            ws,
            {
                "header": {
                    "action": "run-task",
                    "task_id": task_id,
                    "streaming": "duplex",
                },
                "payload": {
                    "task_group": "audio",
                    "task": "tts",
                    "function": "SpeechSynthesizer",
                    "model": model,
                    "parameters": {
                        "text_type": "PlainText",
                        "voice": voice,
                        "format": "pcm",
                        "sample_rate": self.sample_rate,
                        "volume": 50,
                        "rate": 1.0,
                        "pitch": 1.0,
                    },
                    "input": {},
                },
            },
        )

    async def _wait_for_cosyvoice_task_started(self, ws: Any) -> None:
        while True:
            msg = await ws.recv()
            if isinstance(msg, bytes):
                continue
            event = json.loads(msg)
            event_type = event.get("header", {}).get("event", "")
            if event_type == "task-started":
                return
            if event_type == "task-failed":
                err_msg = event.get("header", {}).get("error_message", "Unknown error")
                raise RuntimeError(f"CosyVoice task failed to start: {err_msg}")

    async def _send_cosyvoice_continue_task(
        self,
        ws: Any,
        task_id: str,
        text: str,
    ) -> None:
        await self._send_json(
            ws,
            {
                "header": {
                    "action": "continue-task",
                    "task_id": task_id,
                    "streaming": "duplex",
                },
                "payload": {
                    "input": {
                        "text": text,
                    },
                },
            },
        )

    async def _send_cosyvoice_finish_task(self, ws: Any, task_id: str) -> None:
        await self._send_json(
            ws,
            {
                "header": {
                    "action": "finish-task",
                    "task_id": task_id,
                    "streaming": "duplex",
                },
                "payload": {
                    "input": {},
                },
            },
        )

    async def _receive_qwen_response_audio(
        self,
        ws: Any,
        rechunker: AudioRechunker,
    ) -> AsyncIterator[AudioChunk]:
        seen_audio = False
        while True:
            event = json.loads(await ws.recv())
            event_type = event.get("type", "")

            if event_type == "response.audio.delta":
                delta = event.get("delta", "")
                if not delta:
                    continue
                seen_audio = True
                try:
                    pcm = base64.b64decode(delta)
                    audio = (
                        np.frombuffer(pcm, dtype=np.int16).astype(np.float32)
                        / 32768.0
                    )
                    if self.sample_rate != self.target_sample_rate:
                        audio = self._resample(
                            audio,
                            self.sample_rate,
                            self.target_sample_rate,
                        )
                    for chunk in rechunker.feed(audio):
                        yield chunk
                except Exception:
                    logger.exception("Failed to decode Qwen TTS audio delta")
                continue

            if event_type == "response.audio.done":
                return

            if event_type == "response.done":
                if seen_audio:
                    return
                raise RuntimeError("Qwen TTS response ended without audio")

            if event_type == "output.done":
                return

            if event_type == "error":
                raise RuntimeError(f"Qwen TTS response error: {event}")

    async def _receive_cosyvoice_response_audio(
        self,
        ws: Any,
        rechunker: AudioRechunker,
    ) -> AsyncIterator[AudioChunk]:
        while True:
            msg = await ws.recv()

            if isinstance(msg, bytes):
                try:
                    audio = (
                        np.frombuffer(msg, dtype=np.int16).astype(np.float32)
                        / 32768.0
                    )
                    if self.sample_rate != self.target_sample_rate:
                        audio = self._resample(
                            audio,
                            self.sample_rate,
                            self.target_sample_rate,
                        )
                    for chunk in rechunker.feed(audio):
                        yield chunk
                except Exception:
                    logger.exception("Failed to decode CosyVoice audio binary")
                continue

            event = json.loads(msg)
            event_type = event.get("header", {}).get("event", "")
            if event_type == "task-finished":
                return
            if event_type == "task-failed":
                err_msg = event.get("header", {}).get("error_message", "Unknown error")
                raise RuntimeError(f"CosyVoice task failed: {err_msg}")

    @staticmethod
    async def _send_json(ws: Any, payload: dict[str, Any]) -> None:
        await ws.send(json.dumps(payload, ensure_ascii=False))

    @staticmethod
    def _event_id(session_id: str, suffix: str = "evt") -> str:
        base = session_id or "qwen_tts"
        return f"{base}_{suffix}_{int(time.time() * 1000)}"

    def _request_model(self, request_config: TTSRequestConfig | None) -> str:
        return ((request_config.model if request_config else "") or self.model).strip()

    def _qwen_ws_url(self, model: str) -> str:
        if model == self.model:
            return self.ws_url
        return dashscope_realtime_ws_url(model, "DASHSCOPE_TTS_WS_URL")

    def _cosyvoice_inference_model(self, model: str) -> str:
        """Map product TTS model ids to CosyVoice WS model ids."""
        m = model.strip().lower()
        if m.startswith("cosyvoice-"):
            return model.strip()
        if self._uses_cosyvoice_protocol(model):
            return self.cosyvoice_model or "cosyvoice-v3-flash"
        return model.strip()

    @staticmethod
    def _uses_cosyvoice_protocol(model: str) -> bool:
        m = model.strip().lower()
        if m.startswith("cosyvoice-"):
            return True
        # qwen3-tts-* / qwen-tts-* use the Qwen-TTS Realtime WebSocket (Serena, Momo, etc.).
        if m.startswith("qwen3-tts-") or m.startswith("qwen-tts-"):
            return False
        # Non-TTS Qwen realtime models (e.g. omni) are not valid on the TTS Realtime API.
        if m.startswith("qwen") and "realtime" in m:
            return True
        return False

    @staticmethod
    def _resample(audio: np.ndarray, orig_sr: int, target_sr: int) -> np.ndarray:
        if orig_sr == target_sr:
            return audio.astype(np.float32)
        from scipy.signal import resample_poly

        divisor = gcd(orig_sr, target_sr)
        return resample_poly(
            audio,
            target_sr // divisor,
            orig_sr // divisor,
        ).astype(np.float32)

    async def shutdown(self) -> None:
        return None
