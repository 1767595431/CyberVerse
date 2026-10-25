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
