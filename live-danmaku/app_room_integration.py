"""
豆伴 studio/app.py 中与直播间连接相关的片段（参考用，不可直接运行）。

展示：按 live.platform 选择 Listener，connect 时 start，断开时 close。
"""
import copy

# from bilibili import Listener
# from douyin import Listener as DouyinListener


async def room_handler_example(studio, data):
    async with studio.lock:
        if studio.listener:
            await studio.listener.close()
            studio.listener = None
        if data.get('connect'):
            platform = studio.config['live']['platform']
            if platform == 'bilibili':
                b = studio.config['bilibili']
                if b['mode'] != 'web' or b['room_id'] <= 0:
                    raise ValueError('请先填写 B 站直播间房间号')
                listener = Listener(copy.deepcopy(studio.config), studio.engine)
            elif platform == 'douyin':
                listener = DouyinListener(copy.deepcopy(studio.config), studio.engine)
            else:
                raise ValueError('开播平台不正确')
            await listener.start()
            studio.listener = listener
    return studio.status()


def status_live_snippet(studio):
    platform = studio.config['live']['platform']
    live = studio.listener.status() if studio.listener else {
        'platform': platform,
        'connected': False,
        'mode': 'relay' if platform == 'douyin' else studio.config['bilibili']['mode'],
        'room_id': (studio.config['live']['douyin_room'] if platform == 'douyin'
                    else studio.config['bilibili']['room_id']),
    }
    likes = getattr(studio.listener, 'likes', None) if studio.listener else None
    return {'live': live, 'likes': likes.status() if likes else None}
