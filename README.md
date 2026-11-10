# Live Stream New

基于 **Go、Beego、SQLite、Redis 和 Kafka** 的直播互动系统，包含房间管理、实时弹幕、保存确认、历史消息、点赞、热度榜、多实例广播和房间连接分片。前端使用 React、TypeScript 和 Vinext。

项目围绕直播间互动链路学习 Go 后端开发，重点是消息确认语义、消费重试与幂等、跨实例分发、并发顺序及退出清理。音视频推流接入留待后续。

## 已实现功能

| 模块 | 能力 |
| --- | --- |
| 用户与会话 | 注册、登录、当前用户、退出；随机 sid Cookie 和 Redis Session |
| 房间管理 | 创建房间、我的房间、公开列表、详情和房主开播 |
| 实时互动 | WebSocket 弹幕、游客观看、登录用户发送、房间隔离、点赞与基础限频 |
| 保存确认 | 区分 Kafka accepted 与 SQLite ack，前端维护待处理状态、超时提示和原 UUID 手动重试 |
| 幂等保存 | SQLite message_id 唯一约束，合法重复返回原记录，内容或身份冲突拒绝 |
| 消息历史 | 持久化查询、before_id 分页、HTTP/WS/ACK 按 UUID 合并去重 |
| Redis 缓存 | 最近消息写入与读取，缺失或损坏时按选定 ID 回源 SQLite |
| Kafka 双消费组 | 实时广播与持久化分别消费，失败重试、处理成功后提交消费进度 |
| 多实例 | 同机两个 Beego 进程，共享 SQLite；Redis Pub/Sub 转发弹幕、私有 ACK 和点赞 |
| 在线与热度 | TTL 在线快照汇总、近期互动分钟桶、定时刷新 Redis ZSET、榜单 API 与页面 |
| 房间连接分片 | 每实例 4 个固定 worker、每队列 256 个任务，连接稳定分片、房间广播有序入队 |
| 资源清理 | WS 退出、后台任务取消、广播 worker 停止、Kafka Reader/Producer 关闭 |

账户、房间、弹幕、缓存、热度榜、Kafka、多实例及连接分片已按服务器阶段反馈验收。当前没有发布吞吐或延迟提升数字。

## 技术栈

| 层级 | 选型 |
| --- | --- |
| 后端 | Go 1.27.3，Beego 2，Beego ORM，Gorilla WebSocket |
| 数据库 | SQLite，modernc.org/sqlite，WAL 模式 |
| 缓存与广播 | Redis，go-redis |
| 消息队列 | Apache Kafka 4.4.0，单节点 KRaft，kafka-go |
| 前端 | React 19，TypeScript，Vinext，npm |

版本以 [go.mod](go.mod) 和 [前端 package.json](live-stream-auth-frontend/package.json) 为准。

## 系统架构

```text
浏览器 ── HTTP / WebSocket ──→ Beego 实例 A / B
                                  │
                        Session 校验、UUID 绑定
                                  │
                               Producer
                                  │
                                  ▼
                           Kafka live-danma
                 ┌────────────────┴────────────────┐
                 ▼                                 ▼
        live-realtime-v1                    live-persist-v1
        发布实时弹幕                         SQLite 幂等保存
                 │                         缓存 / 指标 / 保存 ACK
                 └────────────────┬────────────────┘
                                  ▼
                       Redis live:room-events:v1
                 ┌────────────────┴────────────────┐
                 ▼                                 ▼
               实例 A                             实例 B
        房间广播 → 分片 worker             房间广播 → 分片 worker
        私有 ACK → SendToUser              私有 ACK → SendToUser
                 │                                 │
           本实例 WS 连接                     本实例 WS 连接
```

### 消息确认与重试

发送端生成 UUID。同一条逻辑消息手动重试时沿用原 UUID 和内容。入口会检查既有记录，并用 Redis 在有限 TTL 内固定首次提交的事件，防止同 UUID 改内容被实时展示。

- `accepted` 表示 Kafka 已确认接收，保存结果仍需等待。
- `ack` 表示 SQLite 已保存，返回原记录的 id、message_id 和 sent_at。
- `danma` 用于实时展示，可能早于或晚于 ACK。前端按 UUID 合并并补全数据库 ID。
- 持久化消费重试依靠 SQLite 唯一约束避免重复记录；辅助指标不是精确事件账本。

### 多实例分发

两个实例在同名 Kafka 消费组内分担分区，消费结果发布至 Redis，再由每个实例的订阅器投递给本地 Hub。发布者也经过订阅接收，避免本地重复广播。ACK 和私有错误按 room_id + user_id 投递。

SQLite 使用同一台机器上的同一个文件，启用 WAL 和锁等待。当前部署不包含多机数据库共享或热度任务选主。

### 热门房间连接分片

房间内连接按入房序号轮转分配至四个分片；同一连接在本次会话中固定由一个 FIFO worker 处理。每条房间广播给各非空分片各提交一个任务，让同一个大房间的投递由多个 worker 分担。

Hub 在房间锁下串行提交任务，worker 复制本分片成员后释放锁，再调用连接原有的 Send；网络写仍由连接 WriteLoop 完成。队列满时背压，Shutdown 唤醒等待入队者并停止 worker。当前统一使用分片结构，没有冷热迁移和自适应采样。

### 在线与热度

两个实例每 3 秒发布本地在线快照，TTL 为 12 秒，人数按连接统计，包含游客和同账号多页面。固定由 gw-a 每 5 秒刷新共享榜单：

```text
heat = online + 3 × recent_danmas + recent_likes
```

近期互动按分钟桶近似汇总，结果写入带过期时间的 Redis ZSET。gw-b 查询相同榜单，不自动接管刷新任务。

## 目录结构

```text
.
├── main.go                         # 初始化、依赖组装和服务启动
├── conf/*.example                  # 配置示例，真实配置不提交
├── routers/                        # 路由绑定
├── controllers/                    # HTTP / WebSocket 入口
├── filters/                        # 会话鉴权
├── internal/
│   ├── auth/                       # sid 与 Redis Session
│   ├── model/                      # ORM 模型
│   ├── repo/                       # SQLite 数据访问
│   ├── service/                    # 业务、Pipeline、RoomBus、热度刷新
│   ├── storage/                    # SQLite / Redis 初始化
│   ├── cache/                      # Redis 数据结构与缓存访问
│   ├── kafka/                      # Producer 与 Consumer
│   ├── ws/                         # Client、Hub、分片 worker
│   └── lifecycle/                  # 后台任务启动与退出编排
├── cmd/kafka-probe/                # Kafka 消息发送工具
└── live-stream-auth-frontend/       # React / TypeScript / Vinext 前端
```

## 运行环境与配置

建议使用 Ubuntu / Linux，准备 Go 1.27.3、Redis、Kafka 4.4.0、兼容的 Java 运行环境，以及 Node.js 22.13.0 以上版本。已使用的演示环境为 Node.js 24.22.1；Go 最低版本以 go.mod 为准。

| 服务 | 默认地址 |
| --- | --- |
| 前端 | http://127.0.0.1:13000 |
| 后端 A | http://127.0.0.1:19000 |
| 后端 B | http://127.0.0.1:19001 |
| Redis | 127.0.0.1:16379 |
| Kafka Broker / Controller | 127.0.0.1:19092 / 19093 |

在项目根目录准备配置和数据目录；已有配置时保留原文件：

```bash
mkdir -p conf data/redis data/kafka logs
cp -n conf/app.conf.example conf/app.conf
cp -n conf/redis.conf.example conf/redis.conf
cp -n conf/kafka.properties.example conf/kafka.properties
cp -n conf/kafka-env.sh.example conf/kafka-env.sh
```

实际配置、数据库、工具链、node_modules 和日志由 .gitignore 排除。HTTP 演示保持 cookie_secure=false；当前 WS Origin 允许 http://127.0.0.1:13000 和 http://localhost:13000。

## 启动

以下终端均从项目根目录执行。服务器离线时复用既有工具链与依赖；首次安装需要在联网机器准备发行包和依赖后传入。下方下载依赖命令用于联网环境，不要求离线服务器访问外网。

### 1. Redis 与 Kafka

Redis 独立终端启动：

```bash
redis-server conf/redis.conf
```

Kafka 发行包示例放在 tools/kafka_2.13-4.4.0，Java 路径在 conf/kafka-env.sh 中按实际安装位置设置：

```bash
source conf/kafka-env.sh
```

仅对全新的 Kafka 数据目录执行一次初始化；已有数据不要重复格式化：

```bash
tools/kafka_2.13-4.4.0/bin/kafka-storage.sh format --standalone \
  -t "$(tools/kafka_2.13-4.4.0/bin/kafka-storage.sh random-uuid)" \
  -c conf/kafka.properties
```

Kafka 独立终端启动：

```bash
KAFKA_HEAP_OPTS='-Xms256m -Xmx512m' \
LOG_DIR="$PWD/logs" \
tools/kafka_2.13-4.4.0/bin/kafka-server-start.sh conf/kafka.properties
```

另开已加载 Kafka 环境的终端创建业务 Topic：

```bash
tools/kafka_2.13-4.4.0/bin/kafka-topics.sh \
  --bootstrap-server 127.0.0.1:19092 \
  --create --if-not-exists --topic live-danma \
  --partitions 3 --replication-factor 1
```

### 2. 后端

联网环境首次准备 Go 依赖：

```bash
export GOPROXY=https://mirrors.aliyun.com/goproxy/,direct
go mod download
```

启动 A：

```bash
LIVE_INSTANCE_ID=gw-a go run .
```

需要演示多实例时，在另一个终端从同一项目根目录启动 B：

```bash
LIVE_INSTANCE_ID=gw-b go run .
```

两实例读取同一个 data/live-stream.db，不各自复制数据库。启动时自动建表、订阅 Redis，并启动 Kafka 消费和广播 worker；A 额外启动榜单刷新。

### 3. 前端

```bash
cd live-stream-auth-frontend
npm ci
npm run dev
```

已有 node_modules 的离线服务器只需运行 npm run dev。访问 http://127.0.0.1:13000；远程服务器通过 VS Code Remote SSH 转发该端口。前端同源代理把 /api 和 WS 转发至 19000，浏览器携带 sid Cookie。

## API

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | /health | HTTP 健康检查 |
| POST | /api/v1/auth/register | 注册 |
| POST | /api/v1/auth/login | 登录，设置 sid |
| GET | /api/v1/auth/me | 当前用户 |
| POST | /api/v1/auth/logout | 删除会话并清 Cookie |
| POST | /api/v1/rooms | 创建房间 |
| GET | /api/v1/rooms | 开播房间列表 |
| GET | /api/v1/rooms/mine | 我的房间 |
| GET | /api/v1/rooms/hot | 热度榜 |
| GET | /api/v1/rooms/:room_id | 房间详情 |
| POST | /api/v1/rooms/:room_id/start | 房主开播 |
| GET | /api/v1/rooms/:room_id/messages | before_id / limit 历史分页 |
| GET | /api/v1/rooms/:room_id/messages/sync | after_id / until_id 已保存消息增量查询 |
| GET / WS | /api/v1/rooms/:room_id/ws | 房间实时连接 |

增量查询后端已接入源码和路由；自动重连、分页补拉和 Kafka 落库滞后的前端恢复闭环仍待完成。SQLite ID 不等于 Kafka offset，房间内 ID 允许有间隔。

## WebSocket 协议

发送弹幕，UUID 由客户端生成；重试原消息时保留 UUID 和内容：

```json
{"type":"danma","message_id":"客户端生成的UUID","content":"你好，直播间"}
```

发送点赞：

```json
{"type":"like"}
```

常见响应类型为 accepted、ack、danma、room_state、system、error。room_state 可以只包含 like_count 或 online_count；缺失字段不覆盖前端已有状态。用户身份和房间取自服务端认证连接。

## 验收与后续

服务器已分阶段确认账户、房间、消息确认与手动重试、历史与缓存、热度榜、Kafka、多实例及分片功能。分片代表性验收使用同一房间的多条连接检查广播完整性、UUID 去重、同连接顺序和退出清理。本仓库上传整理未额外执行本机编译、测试或联调。

接下来的主线：

- [ ] 自动重连和完整增量补拉前端，兼顾 Kafka 保存滞后。
- [ ] 自适应采样与策略通知，持久化和控制消息不采样。
- [ ] 同条件压测：连接数、源消息率、实际收件量、延迟、CPU 和内存。

Docker 构建暂缓。关播、改标题、关注、管理后台和音视频服务也暂缓。当前不使用参考仓库的测试或性能数字作为本项目结果。

Redis Pub/Sub 不保存断线期间的历史投递；保存 ACK 表示 SQLite 已保存，不代表所有观众收件。后续恢复由增量查询与前端同步共同完成。

## 参考

学习路线与文档组织参考 [Lectuce/live-stream](https://github.com/Lectuce/live-stream)。本实现选择 Beego / SQLite，目录、协议和完成范围以当前代码为准；没有沿用参考项目的 Gin / GORM / MySQL / ZooKeeper，也没有把其 room_seq、自动恢复、采样或性能声明写作已实现能力。
