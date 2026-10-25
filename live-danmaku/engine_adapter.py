"""
监听器对接用的最小类型与 Engine 约定（摘自豆伴 studio/engine.py，仅保留接口部分）。

接入其他项目时：可保留 Danmaku，把 submit/record/stats 接到你的消息总线即可。
"""
import time
from collections import Counter
from dataclasses import dataclass, field


@dataclass
class Danmaku:
    text: str
    username: str = '观众'
    uid: str = ''
    room_id: int = 0
    message_id: str = ''
    received_at: float = field(default_factory=time.monotonic)


class EngineProtocol:
    """Listener 实际依赖的 engine 形状（不必继承本类）。"""

    stats: Counter

    def submit(self, message: Danmaku, source: str = 'bilibili') -> str:
        """返回如 received / duplicate / queued / no_trigger 等。"""
        raise NotImplementedError

    def record(self, state: str, **data):
        pass
