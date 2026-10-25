# CyberVerse

CyberVerse 是开源的**实时数字人 Agent 框架**：WebRTC / 直连媒体、可换 LLM·TTS·ASR·Omni 插件、人设与 RAG、本地 FlashHead / LiveAct 或云端数字人，并支持**离线成片**与**离线直播（待机循环 + 弹幕回答视频）**。

| 文档 | 说明 |
|------|------|
| 本文 | **安装、运行、模型、云上调试**（团队主文档） |
| [README.zh-CN.md](./README.zh-CN.md) | 产品介绍、Demo、能力矩阵（偏对外） |
| [部署文档.md](./部署文档.md) | AutoDL / LiveAct 实测参数备忘 |
| [live-danmaku/README.md](./live-danmaku/README.md) | B 站 / 抖音弹幕参考与 DyHub |
| [AGENTS.md](./AGENTS.md) | 贡献与 Agent 约定 |

许可证：[GPL v3](./LICENSE)

---

## 1. 架构与端口

```
浏览器 ──HTTP/WS──► Go API (cyberverse-server)
                         │
                         ├── gRPC ──► Python Inference (LLM/TTS/ASR/Avatar)
                         └── WebRTC / 直连媒体（实时会话）
```

| 组件 | 默认端口 | 说明 |
|------|----------|------|
| Go API | **6006**（`config/cyberverse.yaml` → `server.http_port`） | REST、`/ws` 会话 WebSocket |
| Python Inference | **50051** gRPC | 模型推理 |
| 前端 dev / preview | **6008** | Vite；`/api`、`/ws` 代理到 6006 |
| 抖音 DyHub 中继 | **8757**（可选） | 离线直播接抖音弹幕时使用 |

本地开发常见组合：

- `make inference` + `make server` + `make frontend` → 浏览器打开 **http://127.0.0.1:6008**
- 或 `frontend` 用 `npm run preview`（同样 6008），API 指向本机 6006

---

## 2. 环境要求

| 依赖 | 版本建议 |
|------|----------|
| Python | 3.10（Conda 环境名建议 `cyberverse`） |
| Go | 1.25+（需 `protoc-gen-go`、`protoc-gen-go-grpc`） |
| Node.js | 18+（推荐 22） |
| FFmpeg | 含 libvpx（离线 MP4） |
| 系统库 | libopus、libopusfile、libsoxr、pkg-config（Go 侧 LiveKit 媒体） |

**仅语音 Agent、不开本地 Avatar**：可不装 CUDA / 大模型权重，在配置里 `inference.avatar.enabled: false`。

**本地数字人（FlashHead / LiveAct）**：CUDA 12.8+、PyTorch 2.8（cu128）、足够显存；LiveAct 另需 `vllm==0.11.0` 等（见 `pyproject.toml` 可选依赖）。

---

## 3. 安装与首次配置

### 3.1 克隆与 Python 环境

```bash
git clone https://github.com/dsd2077/CyberVerse.git
cd CyberVerse

conda create -n cyberverse python=3.10 -y
conda activate cyberverse
```

### 3.2 本地配置目录（勿提交 Git）

```bash
cp -r infra/config config
```

- `config/env`：API Key、LiveKit、云端域名等（见下）
- `config/cyberverse.yaml`：服务端口、Avatar、流水线默认值
- `config/*_models/`、`config/avatar_models/`：仅在需要覆盖内置插件配置时再改

`config/` 已在 `.gitignore` 中，**每个环境各自维护**。

### 3.3 环境变量（`config/env` 示例）

```env
# 通义 / Qwen（LLM、TTS、ASR、Omni 常用）
DASHSCOPE_API_KEY=sk-...

# 豆包（可选）
DOUBAO_ACCESS_TOKEN=...
DOUBAO_APP_ID=...

# LiveKit（实时 WebRTC 模式需要）
LIVEKIT_URL=wss://...
LIVEKIT_API_KEY=...
LIVEKIT_API_SECRET=...
```

也可在运行后于 Web **`/settings`** 修改部分密钥与端点。

### 3.4 安装依赖

```bash
make setup
# 按需安装可选能力，例如全部插件：
pip install -e ".[all]"
```

纯语音最小示例：在 `config/cyberverse.yaml` 中设置 `inference.avatar.enabled: false`，然后只启动 inference + server + frontend。

---

## 4. 运行

### 4.1 开发（三个终端）

```bash
# 终端 1
conda activate cyberverse
make inference

# 终端 2
make server

# 终端 3
make frontend
```

浏览器访问：**http://127.0.0.1:6008**

健康检查：

```bash
curl -s http://127.0.0.1:6006/api/v1/health
# 期望：status ok，且 inference_connected: true（Avatar/推理起来之后）
```

### 4.2 生产 / 云上长期运行

```bash
conda activate cyberverse
make build          # bin/cyberverse-server + frontend/dist

# Inference（后台）
nohup bash scripts/inference.sh >> /path/to/cv-inference.log 2>&1 &

# Go API（后台）
cd server
nohup ./cyberverse-server --config ../config/cyberverse.yaml >> /path/to/cv-server.log 2>&1 &

# 前端静态：由 Go 托管 dist，或单独 preview
cd frontend && npm run preview -- --host 0.0.0.0 --port 6008
```

也可用 `scripts/start_services.sh` 管理 `.run/logs/` 下的 inference / server / frontend（需项目内 `.venv` 与 Makefile 约定一致）。

### 4.3 常用 Make 目标

| 命令 | 作用 |
|------|------|
| `make proto` | 重新生成 gRPC 桩代码 |
| `make test` | Python 单元测试 + Go 测试 |
| `make inference` | 按 `config/cyberverse.yaml` 启动推理（含 torchrun 多卡） |
| `make server` | 启动 Go API（livekit tag） |
| `make frontend` | Vite 开发服务器 |
| `make build` | 编译后端 + 前端 dist |

---

## 5. 模型与权重下载

权重目录默认 **`./checkpoints/`**（已在 `.gitignore`，不要提交 Git）。

```bash
pip install "huggingface_hub[cli]"
# 国内可选：export HF_ENDPOINT=https://hf-mirror.com
```

### FlashHead

| 组件 | 目标路径 |
|------|----------|
| [SoulX-FlashHead-1_3B](https://huggingface.co/Soul-AILab/SoulX-FlashHead-1_3B) | `./checkpoints/SoulX-FlashHead-1_3B` |
| [wav2vec2-base-960h](https://huggingface.co/facebook/wav2vec2-base-960h) | `./checkpoints/wav2vec2-base-960h` |

```bash
hf download Soul-AILab/SoulX-FlashHead-1_3B --local-dir ./checkpoints/SoulX-FlashHead-1_3B
hf download facebook/wav2vec2-base-960h --local-dir ./checkpoints/wav2vec2-base-960h
```

在 `config/cyberverse.yaml` 中设置 `inference.avatar.default: flash_head`，并编辑 `config/avatar_models/flash_head.yaml` 中的路径。

### LiveAct

| 组件 | 目标路径 |
|------|----------|
| [Soul-AILab/LiveAct](https://huggingface.co/Soul-AILab/LiveAct) | `./checkpoints/LiveAct` |
| [chinese-wav2vec2-base](https://huggingface.co/TencentGameMate/chinese-wav2vec2-base) | `./checkpoints/chinese-wav2vec2-base` |

```bash
hf download Soul-AILab/LiveAct --local-dir ./checkpoints/LiveAct
hf download TencentGameMate/chinese-wav2vec2-base --local-dir ./checkpoints/chinese-wav2vec2-base
```

LiveAct 性能相关配置见 `config/avatar_models/live_act.yaml`（如 `fp4_gemm`、`size`、`fps`）。**改分辨率 / world_size 后需重启 inference**。

### 不下载本地 Avatar 时

- 使用 **纯语音**（`avatar.enabled: false`），或  
- 角色选用 **百度曦灵 / 讯飞 / Vidu** 等云端 Avatar（在角色编辑页配置，密钥见 `config/env`）。

---

## 6. 云上环境（AutoDL、SeetaCloud 等）

### 6.1 访问路径

平台通常映射：**实例内 6006 或 6008 → 公网 HTTPS（如 `:8443`）**。

- 仅跑 **Go + 内置静态前端**：对外暴露 **6006** 即可。  
- 跑 **Vite preview/dev（6008）**：浏览器入口为 **6008**，Vite 把 `/api`、`/ws` 反代到 **127.0.0.1:6006**（见 `frontend/vite.config.ts`）。

### 6.2 CORS 与 WebSocket

在 **`config/cyberverse.yaml`** 的 `server.cors_origins` 中加入你的公网源，例如：

```yaml
server:
  cors_origins:
    - https://你的实例域名.seetacloud.com:8443
    - https://uu你的实例前缀.seetacloud.com:8443
```

修改后**重启 Go 服务**。否则浏览器会报跨域或 WS 握手失败。

### 6.3 Vite 远程 HMR（可选）

开发模式下若用平台域名访问，需在 `frontend/vite.config.ts` 中设置与面板一致的 `publicOrigin`、`hmr.host`、`hmr.clientPort`（一般为 **8443**）。**生产 preview 可不改 HMR**，只要 `proxy` 指向本机 6006。

### 6.4 健康检查与日志

| 现象 | 排查 |
|------|------|
| `inference_connected: false` | Inference 未起、50051 被占用、或启动尚未完成（LiveAct 加载较慢） |
| 页面能开但会话无响应 | 看 Go 日志 WS；看 `curl localhost:6006/api/v1/health` |
| TTS/LLM 报错 | `cv-inference.log` 中 DashScope / 插件错误 |
| 离线直播无回答视频 | Go 日志 `offline_live:`、`TTS stream error`；确认 TTS 模型与音色 |

建议日志路径（自行约定，勿提交 Git）：

- `/path/to/cv-inference.log` — Python inference  
- `/path/to/cv-server.log` — Go API  

### 6.5 防火墙与端口

- **50051** 仅需本机访问（Go → Inference）。  
- 对外只开放平台映射的 **HTTPS 端口**；不要误把 50051 暴露公网。

---

## 7. 功能入口（Web）

| 路径 | 用途 |
|------|------|
| `/` | 角色列表 |
| `/characters/:id/offline-videos` | 离线视频管理（待机/动作/唱歌成片） |
| `/launch/:id/offline-live` | **离线直播**开播（待机、动作、弹幕、TTS/LLM 管线） |
| `/session/:sessionId` | 会话页（实时或离线直播） |
| `/settings` | 全局密钥与组件 |

离线直播成片命名约定（视频管理标题前缀）：**待机-**、**动作-**、**唱歌-**，供开播页筛选。

弹幕：Go 内置调度；抖音需本机 **DyHub**（默认 `http://127.0.0.1:8757`），详见 [live-danmaku/README.md](./live-danmaku/README.md)。

---

## 8. 调试清单

1. **health**：`curl -s http://127.0.0.1:6006/api/v1/health`  
2. **推理日志**：是否出现 `CyberVerse Inference Server started on port 50051`  
3. **Go 日志**：`CyberVerse Server starting on 0.0.0.0:6006`  
4. **浏览器控制台**：`[switch]` 日志（待机 / 回答片切换）；WS 是否连上同源  
5. **Avatar 卡顿**：Inference 日志 **RTP**；参考 README 旧版 QA 或 [部署文档.md](./部署文档.md) 中的分辨率/帧率表  
6. **改 config 后**：改 Avatar 参数 → **重启 inference**；改端口/CORS → **重启 Go**

---

## 9. 仓库与提交说明

- **应提交**：源码、`infra/config` 模板、`scripts/`、文档、`frontend/package.json` 等。  
- **不应提交**：见 [.gitignore](./.gitignore)（`config/`、权重、`data/`、`.env`、构建产物、日志、本地 MP4 等）。

---

## 10. 获取帮助

- Issue：[GitHub Issues](https://github.com/dsd2077/CyberVerse/issues)  
- 云镜像（免手工装环境）：README.zh-CN.md 中的 AutoDL / 优云智算链接  

英文版功能说明与 Demo 见 [README.zh-CN.md](./README.zh-CN.md)（结构与历史 [README.md](https://github.com/dsd2077/CyberVerse/blob/main/README.md) 英文营销版类似，以仓库 `main` 为准）。
