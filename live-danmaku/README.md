# B 站 / 抖音弹幕对接参考包

本文件夹是 **从豆伴项目复制的静态参考**，可直接整目录拷贝到其他程序中使用。  
**不参与豆伴运行**；豆伴源码仍在 `studio/bilibili.py`、`studio/douyin.py`、`studio/likes.py`。

快照来源：豆伴仓库 `douban-source`（与主工程同步维护时可重新复制本目录）。

## 目录说明

| 文件 | 说明 |
|------|------|
| `bilibili.py` | B 站 Web 直播间监听（`blivedm`，匿名房间号） |
| `douyin.py` | 抖音监听（本机 DyHub 中继 WebSocket，仅 `chat`） |
| `likes.py` | B 站点赞（非礼物），可选 |
| `engine_adapter.py` | 监听器依赖的 `Danmaku` 与 `Engine` 最小接口说明 |
| `app_room_integration.py` | 豆伴里如何选平台、创建/关闭 `Listener` 的示例 |
| `config_example.json` | 相关配置字段示例 |
| `start-douyin-relay.ps1` | 启动 DyHub 的脚本（路径相对豆伴根目录；拷贝后请改 `runtime/dyhub`） |

## 接入到你的程序

### 1. 实现「引擎」对象

两个 `Listener` 构造函数均为 `(config, engine)`。你的 `engine` 至少需要：

- `submit(message: Danmaku, source='bilibili'|'douyin') -> str` — 收到一条弹幕
- `record(state, **data)` — 可选，记录 `listener_error` 等
- `stats` — 可选，`Counter`，用于 `skipped_*` 统计

消息体见 `engine_adapter.py` 中的 `Danmaku` dataclass。

### 2. 配置结构

见 `config_example.json`：

- B 站：`bilibili.mode` 固定 `web`，`bilibili.room_id` 为正整数
- 抖音：`live.platform` 为 `douyin`，`live.douyin_room` 为房间号或直播间链接，`live.douyin_relay` 默认 `http://127.0.0.1:8757`

### 3. B 站

**依赖：** `blivedm`（豆伴锁定 Git 上游 commit，勿误用 PyPI 同名包）、`aiohttp~=3.9`

**流程：**

```text
BLiveClient(room_id, uid=0) → join 循环 → _on_danmaku → engine.submit
```

- 不读 Cookie / 开放平台凭证（代码内写死匿名）
- 心跳 75 秒内视为已连接（`status()`）
- **礼物：** `blivedm` 能解析，本参考包 **未** 接礼物 handler
- **点赞：** `likes.py` + `LIKE_INFO_V3_*` CMD

### 4. 抖音

**依赖：** 本机 [DyHub](https://github.com/ymstar/dyhub)（Node 20+、Chrome/Edge）

**流程：**

```text
POST /api/rooms/connect  →  WS /ws?roomId=...&types=chat  →  chat 事件 → engine.submit
```

- 先启动中继（豆伴根目录 `start-douyin-relay.ps1`，或自行部署 DyHub）
- 中继仅允许本机地址（`127.0.0.1` / `localhost`）
- **礼物：** DyHub 支持 `types=chat,gift`（常需 Cookie）；本参考包 WS 只订 `chat`，`_handle` 只处理 `type==chat`

### 5. 生命周期

```python
listener = BilibiliListener(config, engine)  # 或 DouyinListener
await listener.start()
# ...
await listener.close()
```

参考 `app_room_integration.py` 中的连接/断开模式。

## 礼物能力对照

| 平台 | 底层 | 本参考包 |
|------|------|----------|
| B 站 | `blivedm` 礼物 CMD | 未接入 |
| 抖音 | DyHub `gift` 事件 | 未接入（可改 `douyin.py` 的 `types` 与 `_handle`） |

扩展建议：将礼物合成一条文本再 `submit`，例如 `【礼物】用户 送出 火箭 ×1`。

## 豆伴内对应路径

| 参考包 | 豆伴运行代码 |
|--------|----------------|
| `bilibili.py` | `studio/bilibili.py` |
| `douyin.py` | `studio/douyin.py` |
| `likes.py` | `studio/likes.py` |
| `engine_adapter.py` | `studio/engine.py`（完整队列/Vidu 逻辑） |
| `app_room_integration.py` | `studio/app.py` 的 `room()`、`Studio.status()` |

---

## 故障排查（CyberVerse 离线直播 + DyHub）

### 现象：界面显示「弹幕已连接」，但左侧弹幕列表始终为空

CyberVerse 的「已连接」只表示 **Go 进程已连上 DyHub 的 WebSocket**，不保证 DyHub 仍在向总线推送新的 `chat` 事件。

**典型根因：DyHub `lightweight` 采集会话僵死**

- 房间状态仍为 `live`，`frameCount` 持续增加，但 **`msgCount` / 总线 `totalPublished` 长时间不变**（例如卡在几百条不再涨）。
- 此时 WS 订阅 `types=chat` **收不到任何新弹幕**；Go 日志里只有 `danmaku douyin relay connected`，**没有** `danmaku chat session=...`。
- `/api/v1/sessions/{id}/danmaku/feed` 的 `items` 为空，但 `relay_ws_open: true`。

**快速自检**

```bash
# 1) DyHub 是否还在发布事件（totalPublished 应随直播弹幕增加）
curl -s http://127.0.0.1:8757/api/stats | jq '.bus.totalPublished, .rooms'

# 2) 直连 WS 10s 内是否出现 type=chat（替换 roomId）
# python - websockets 订阅 ws://127.0.0.1:8757/ws?roomId=ROOM&types=chat

# 3) Go 是否入库
grep 'danmaku chat' /tmp/cyberverse-api.log | tail
```

**处理**

1. 重启 DyHub（`runtime/dyhub` 下 `npm start`，默认 `DYHUB_COLLECTOR=lightweight`）。
2. 会话页弹幕区 **停止 → 连接**（或刷新后重连），让 Go 重新 `connect` 房间并订阅 WS。
3. 在 DyHub 控制台对该房间 **断开再连**（`POST /api/rooms/{roomId}/disconnect` 后 `connect`）。
4. 若频繁僵死：考虑 `DYHUB_COLLECTOR=browser` + 可用 Chrome/Chromium；数据中心 IP 下 lightweight 更易异常。

**勿与以下混淆**

- 左下角「输入消息」走 **LLM 对话**，不会进入抖音弹幕 feed。
- 右侧直播预览里的聊天是 **抖音 UI 模拟**，不等于 CyberVerse 左侧弹幕列表。
