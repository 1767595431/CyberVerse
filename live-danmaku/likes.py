"""Keep personal like notifications separate from room-wide totals."""

import time
import uuid
from collections import OrderedDict, deque


def count_value(value):
    if isinstance(value, str) and value.isascii() and value.isdigit() and len(value) <= 16:
        value = int(value)
    if type(value) is int and 0 <= value <= 2**53 - 1:
        return value
    return None


class LikeFeed:
    def __init__(self):
        self.stream_id = uuid.uuid4().hex
        self.started_at = time.time()
        self.events = deque(maxlen=100)
        self.seen = OrderedDict()
        self.notifications = 0
        self.known_count = 0
        self.unknown_notifications = 0
        self.room_total = None
        self.room_updated_at = None

    def update_total(self, value):
        value = count_value(value)
        if value is not None:
            # This is a snapshot, not a delta; a new broadcast can reset it.
            self.room_total = value
            self.room_updated_at = time.time()

    def add(self, username, uid="", count=None, message_id="", source="web"):
        if isinstance(message_id, str) and message_id and len(message_id) <= 256:
            key = (source, message_id)
            if key in self.seen:
                return
            self.seen[key] = None
            if len(self.seen) > 5000:
                self.seen.popitem(last=False)
        count = count_value(count)
        # Zero/missing personal counts do not prove that a user clicked once.
        count = count if count else None
        self.notifications += 1
        self.known_count += count or 0
        self.unknown_notifications += count is None
        self.events.append({
            "id": self.notifications, "time": time.time(),
            "username": username[:80] if isinstance(username, str) and username.strip() else "匿名观众",
            "uid": str(uid)[:100] if isinstance(uid, (str, int)) else "",
            "count": count, "source": source,
        })

    def status(self):
        return {"stream_id": self.stream_id, "started_at": self.started_at,
                "room_total": self.room_total, "room_updated_at": self.room_updated_at,
                "notifications": self.notifications, "known_count": self.known_count,
                "unknown_notifications": self.unknown_notifications,
                "events": list(self.events)}
