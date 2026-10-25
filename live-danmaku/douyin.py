"""Douyin danmaku through a local, normalized relay (DyHub compatible)."""
# 参考副本：接入时 from engine_adapter import Danmaku
import asyncio
import json
import re
import time
from urllib.parse import parse_qs, quote, urlsplit, urlunsplit

import aiohttp

from engine_adapter import Danmaku

ROOM_RE = re.compile(r'^[A-Za-z0-9_-]{1,64}$')


def normalize_room(value):
    if not isinstance(value, str):
        raise ValueError('请填写抖音直播间房间号或链接')
    value = value.strip()
    if not value:
        raise ValueError('请填写抖音直播间房间号或链接')
    if '://' in value or value.lower().startswith(('live.douyin.com/', 'www.douyin.com/')):
        parsed = urlsplit(value if '://' in value else f'https://{value}')
        if parsed.hostname not in ('live.douyin.com', 'www.douyin.com'):
            raise ValueError('请填写抖音直播间链接')
        query = parse_qs(parsed.query)
        for key in ('live_web_rid', 'web_rid', 'room_id', 'roomid'):
            candidate = (query.get(key) or [''])[0].strip()
            if candidate:
                if not ROOM_RE.fullmatch(candidate):
                    raise ValueError('抖音直播间房间号格式不正确')
                return candidate
        segments = [part for part in parsed.path.split('/') if part]
        if parsed.hostname != 'live.douyin.com' or len(segments) != 1:
            raise ValueError('请粘贴包含 live_web_rid 的抖音直播间链接')
        value = segments[0]
    elif '/' in value:
        raise ValueError('抖音直播间房间号格式不正确')
    if not ROOM_RE.fullmatch(value):
        raise ValueError('抖音直播间房间号格式不正确，请填写房间号或直播间链接')
    return value


def relay_base(value):
    value = (value or '').strip().rstrip('/')
    parsed = urlsplit(value)
    if (parsed.scheme not in ('http', 'https')
            or parsed.hostname not in ('127.0.0.1', 'localhost', '::1')
            or parsed.username or parsed.password):
        raise ValueError('抖音弹幕中继仅支持本机地址')
    return value


class Listener:
    def __init__(self, config, engine):
        self.config, self.engine = config, engine
        self.base = ''
        self.room_id = ''
        self.session = None
        self.ws = None
        self.supervisor = None
        self.connected = False
        self.last_heartbeat = None
        self.stopped = False
        self.started_room = False

    async def start(self):
        self.base = relay_base(self.config['live']['douyin_relay'])
        self.room_id = normalize_room(self.config['live']['douyin_room'])
        self.session = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=30))
        try:
            await self._connect_room()
        except Exception:
            await self.session.close()
            self.session = None
            raise
        self.supervisor = asyncio.create_task(self.run())

    async def _connect_room(self):
        try:
            async with self.session.post(f'{self.base}/api/rooms/connect',
                                         json={'roomId': self.room_id}) as response:
                if response.status not in (200, 201, 409):
                    raise ValueError(f'抖音弹幕中继拒绝连接（HTTP {response.status}）')
                self.started_room = response.status == 201
        except aiohttp.ClientError:
            raise ValueError('抖音弹幕中继未启动，请先启动本机 DyHub 中继（默认 127.0.0.1:8757）') from None

    def _ws_url(self):
        parsed = urlsplit(self.base)
        scheme = 'wss' if parsed.scheme == 'https' else 'ws'
        path = (parsed.path.rstrip('/') or '') + '/ws'
        return urlunsplit((scheme, parsed.netloc, path,
                           f'roomId={quote(self.room_id)}&types=chat', ''))

    async def run(self):
        while not self.stopped:
            try:
                async with self.session.ws_connect(self._ws_url(), heartbeat=20) as ws:
                    self.ws, self.connected = ws, True
                    self.last_heartbeat = time.monotonic()
                    async for message in ws:
                        if message.type == aiohttp.WSMsgType.TEXT:
                            self.last_heartbeat = time.monotonic()
                            self._handle(message.data)
                        elif message.type in (aiohttp.WSMsgType.CLOSED, aiohttp.WSMsgType.ERROR):
                            break
            except asyncio.CancelledError:
                raise
            except Exception as error:
                self.engine.record('listener_error', error=type(error).__name__)
            finally:
                self.ws, self.connected, self.last_heartbeat = None, False, None
            if not self.stopped:
                await asyncio.sleep(5)

    def _handle(self, raw):
        try:
            event = json.loads(raw)
        except (TypeError, json.JSONDecodeError):
            return
        if not isinstance(event, dict) or event.get('type') != 'chat':
            return
        data, user = event.get('data'), event.get('user')
        content = data.get('content') if isinstance(data, dict) else None
        if not isinstance(content, str) or not content.strip():
            return
        username = user.get('nickname') if isinstance(user, dict) else ''
        uid = user.get('id') if isinstance(user, dict) else ''
        self.accept(Danmaku(content.strip(), str(username or '抖音观众'), str(uid or ''),
                            self.room_id, str(event.get('id') or '')))

    def accept(self, message):
        result = self.engine.submit(message, source='douyin')
        if result != 'queued':
            self.engine.stats['skipped_' + result] += 1

    async def close(self):
        self.stopped = True
        if self.supervisor:
            self.supervisor.cancel()
            await asyncio.gather(self.supervisor, return_exceptions=True)
        if self.ws:
            await self.ws.close()
        if self.session:
            if self.started_room:
                try:
                    async with self.session.delete(
                            f'{self.base}/api/rooms/{quote(self.room_id)}') as response:
                        await response.read()
                except Exception:
                    pass
            await self.session.close()

    def status(self):
        age = None if self.last_heartbeat is None else round(time.monotonic() - self.last_heartbeat, 1)
        return {'platform': 'douyin', 'mode': 'relay', 'room_id': self.room_id,
                'connected': self.connected, 'heartbeat_age_seconds': age, 'relay': self.base}
