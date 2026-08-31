<div align="center">
  <img src="Logo.png" alt="MqttBroker" width="120"/>
  <h2>MqttBroker</h2>
  <h3>基于 FasterEdge MqttBrokerCore 的轻量级 MQTT 服务</h3>
</div>

### 一、项目简介
- 基于 **[FasterEdge MqttBrokerCore](https://github.com/FasterEdge/MqttBrokerCore)** 构建的 MQTT 代理服务，通过 HTTP 接口与内置 **WebUI** 轻松控制底层 Broker 的生命周期。
- 统一使用一个管理端口 `11883`，同时提供：
  - **REST API**：`/startup`、`/heartbeat`、`/shutdown`。
  - **Web 管理面板**：在浏览器中启动 / 停止 Broker，并实时查看运行状态与日志。
- MQTT 通信端口默认 `1883`，可通过 `/startup?port=...` 动态指定。
- 底层能力全部来自 `MqttBrokerCore`：支持 **MQTT 3.1.1 / 5.0**、**QoS 0/1/2**、遗嘱消息、保留消息、主题通配符（`+`、`#`）。
- 全部静态资源通过 `go:embed` 嵌入二进制，单文件即可部署，无需额外静态目录。

### 二、主要特性
| 特性 | 说明 |
|------|------|
| HTTP 控制 | 通过 REST 接口启停 Broker，方便与脚本、监控、告警系统集成 |
| 内置 WebUI | 免安装，浏览器网页即可启停服务、查看日志，适合快速试用 |
| 动态端口 | 启动时用 `port` 参数指定监听端口，灵活适应不同环境 |
| 内存持久化 | 默认 `MemoryPersistence`，开箱即用、无外部依赖 |
| 单文件部署 | `go build` 后只需一个可执行文件即可运行 |
| 容器化 | 提供 `Dockerfile`，支持多阶段构建与非 root 运行 |

### 三、快速启动

> **环境要求**：`go.mod` 声明 `toolchain go1.25.13`（已修复标准库 CVE）。Go 1.21+ 会自动下载并使用该工具链；如需手动指定可用 `GOTOOLCHAIN=go1.25.13 go build`。构建始终禁用 CGO（`CGO_ENABLED=0`）。

```bash
go mod tidy
go build ./...

# 启动管理服务（默认监听 11883）
./com.tyza66.SimpleMqttBrokerApi
# 或直接运行源码
go run main.go
```

启动后浏览器访问 `http://127.0.0.1:11883/` 即可打开 Web 管理面板。

**使用 Docker 运行：**

```bash
docker build -t fasteredge/mqtt-broker .

# 启动容器，映射管理端口(11883)与 MQTT 端口(1883)
docker run -d -p 11883:11883 -p 1883:1883 fasteredge/mqtt-broker
```

**Docker 环境变量：**

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `MANAGE_PORT` | `11883` | 管理接口 / WebUI 端口 |
| `MQTT_PORT` | `1883` | 未指定 `port` 参数时 MQTT 监听的默认端口 |

> 在容器内访问 `http://127.0.0.1:11883/health` 可用于健康检查（Broker 运行中返回 200）。

### 四、REST API

| 接口 | 方法 | 说明 |
|------|------|------|
| `/startup?port=1883` | GET | 启动 MQTT Broker，监听指定端口（缺省 `1883`）。重复启动返回 `already running`。 |
| `/heartbeat` | GET | 返回 JSON，包含版本、运行状态、日志缓存、时间戳与 Broker 端口。日志在每次读取后被清空。 |
| `/shutdown` | GET | 优雅关闭正在运行的 Broker。 |

**`/heartbeat` 返回示例：**

```json
{
  "Version": "1.0.20260831",
  "State": "running",
  "Timestamp": "2026-08-28 13:20:00",
  "Logs": [
    "MQTT Broker service started on port 1883"
  ],
  "BrokerPort": "1883"
}
```

### 五、用法示例

```bash
# 启动 MQTT Broker（监听 1883）
curl "http://127.0.0.1:11883/startup?port=1883"

# 查看状态与日志
curl "http://127.0.0.1:11883/heartbeat"

# 停止 Broker
curl "http://127.0.0.1:11883/shutdown"
```

### 六、目录结构

```
MqttBroker/
├─ main.go          # 入口：HTTP 接口 + 嵌入式 WebUI（支持 MANAGE_PORT / MQTT_PORT）
├─ ui/
│  └─ index.html    # Web 管理面板（go:embed 内嵌）
├─ go.mod           # 依赖 FasterEdge MqttBrokerCore 模块
├─ Dockerfile       # 多阶段容器构建 + HEALTHCHECK
├─ .dockerignore
├─ Logo.png
└─ README.md
```

### 七、内部实现
- **内核**：`hrotti.NewHrotti(100, &hrotti.MemoryPersistence{})` 创建基于内存持久化的 Broker 实例。
- **日志**：自定义 `LogInterceptor` 接管 `hrotti.INFO/DEBUG/ERROR` 输出，缓存最近 200 条日志供 `/heartbeat` 查询，并同步打印到标准输出。
- **WebUI**：通过 `go:embed` 把 `ui/` 静态资源嵌入二进制，根路径 `/` 由 `http.FileServer` 服务，与管理接口共用一个端口。
- **线程安全**：`started` 标志与日志缓存由 `sync.Mutex` 保护，避免并发读写竞态。

### 八、已内置的改进（来自 MqttBrokerCore）
- 报文解析长度限制与内存上限，防止 OOM 和协议错误。
- 断线 / 慢客户端下的非阻塞发送，避免服务死锁。
- 空 `ClientIdentifier` 且 `CleanSession=0` 时按规范拒绝连接。
- 消息 ID 耗尽时的安全处理。

---

> 本项目使用的 MQTT 内核来自内部公开库 **[github.com/FasterEdge/MqttBrokerCore](https://github.com/FasterEdge/MqttBrokerCore)**，其上游为开源的 [alsm/hrotti](https://github.com/alsm/hrotti) 项目，特此致谢。