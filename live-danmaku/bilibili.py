# 参考副本：接入时 from engine_adapter import Danmaku；from likes import LikeFeed
import asyncio
import time

import aiohttp
import blivedm

from engine_adapter import Danmaku
from likes import LikeFeed


class Listener(blivedm.BaseHandler):
    def __init__(self, config, engine):
        self.config, self.engine = config, engine
        self.client = None
        self.session = None
        self.supervisor = None
        self.last_heartbeat = None
        self.stopped = False
        self.likes = LikeFeed()

    def handle(self, client, command):
        cmd = command.get("cmd", "").split(":", 1)[0]
        if cmd in ("LIKE_INFO_V3_CLICK", "LIKE_INFO_V3_UPDATE"):
            data = command.get("data")
            if not isinstance(data, dict):
                return
            if cmd == "LIKE_INFO_V3_UPDATE":
                self.likes.update_total(data.get("click_count"))
            else:
                # Web CLICK identifies a liker but does not document a personal
                # count. Never attribute UPDATE totals (or click_count) to them.
                self.likes.add(data.get("uname"), data.get("uid", ""),
                               message_id=data.get("msg_id", ""))
            return
        return super().handle(client, command)

    def _on_open_live_like(self, client, message):
        self.likes.add(message.uname, message.open_id, message.like_count,
                       message.msg_id, source="open_live")

    async def start(self):
        b = self.config["bilibili"]
        if b['mode'] != 'web' or b['room_id'] <= 0:
            raise ValueError('仅支持通过直播间房间号接入')
        self.session = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=20))
        # Anonymous room-number access only; never read account cookies or OpenLive credentials.
        self.client = blivedm.BLiveClient(b["room_id"], uid=0, session=self.session)
        self.client.set_handler(self)
        self.supervisor = asyncio.create_task(self.run())

    async def run(self):
        while not self.stopped:
            self.client.start()
            try:
                await self.client.join()
            except Exception as error:
                self.engine.record("listener_error", error=type(error).__name__)
            self.last_heartbeat = None
            if not self.stopped:
                await asyncio.sleep(5)

    async def close(self):
        self.stopped = True
        if self.supervisor:
            self.supervisor.cancel()
            await asyncio.gather(self.supervisor, return_exceptions=True)
        if self.client:
            await self.client.stop_and_close()
        if self.session:
            await self.session.close()

    def _on_heartbeat(self, client, message):
        self.last_heartbeat = time.monotonic()

    def on_client_stopped(self, client, exception):
        self.last_heartbeat = None
        if exception is not None and not self.stopped:
            self.engine.record("listener_error", error=type(exception).__name__)

    def _on_danmaku(self, client, message):
        if getattr(message, "is_mirror", False):
            return
        timestamp, rnd = getattr(message, "timestamp", 0), getattr(message, "rnd", 0)
        message_id = f"web:{timestamp}:{rnd}:{getattr(message, 'uid_crc32', '')}" if timestamp and rnd else ""
        self.accept(Danmaku(message.msg, message.uname, str(message.uid), client.room_id, message_id))

    def _on_open_live_danmaku(self, client, message):
        if getattr(message, "is_mirror", False):
            return
        self.accept(Danmaku(message.msg, message.uname,
            str(getattr(message, "open_id", "") or getattr(message, "uid", "")),
            message.room_id, str(message.msg_id)))

    def accept(self, message):
        result = self.engine.submit(message)
        if result != "queued":
            self.engine.stats["skipped_" + result] += 1

    def status(self):
        age = None if self.last_heartbeat is None else time.monotonic() - self.last_heartbeat
        return {"platform": "bilibili", "mode": self.config["bilibili"]["mode"],
                "room_id": getattr(self.client, "room_id", None) or self.config["bilibili"]["room_id"],
                "connected": age is not None and age < 75,
                "heartbeat_age_seconds": round(age, 1) if age is not None else None}
