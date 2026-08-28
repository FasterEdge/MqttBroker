<div align="center">
  <img src="Logo.png" alt="MqttBroker" width="120"/>
  <h2>MqttBroker</h2>
  <h3>基于 FasterEdge MqttBrokerCore 的轻量级 MQTT 服务</h3>
</div>

### 一、项目简介
- 基于 **FasterEdge MqttBrokerCore** 构建的 MQTT 代理服务，通过 HTTP 接口与内置 **WebUI** 控制底层 Broker 的生命周期。
- 提供一个管理端口 `11883`，同时承担：
  - **REST API**：`/startup`、`/heartbeat`、`/shutdown`。
  - **Web 面板**：在浏览器中启动 / 停止 Broker，并实时查看日志与运行状态。
- MQTT 通信端口默认 `1883`，可通过 `/startup?port=...` 动态指定。
- 底层能力全部来自 [MqttBrokerCore](https://github.com/FasterEdge/MqttBrokerCore)（含 QoS 0/1/2、遗嘱、保留消息、主题通配符等）。

### 二、快速启动

```bash
go mod tidy
go build ./...

# 启动管理服务（默认监听 11883）
./com.tyza66.SimpleMqttBrokerApi
# 或
go run main.go
```

启动后浏览器访问 `http://127.0.0.1:11883/` 即可打开 Web 管理面板。

### 三、REST API

| 接口 | 方法 | 说明 |
|------|------|------|
| `/startup?port=1883` | GET | 启动 MQTT Broker，监听指定端口（缺省 `1883`）。重复启动返回 `already running`。 |
| `/heartbeat` | GET | 返回 JSON，包含版本、运行状态、日志缓存、时间戳与 Broker 端口。日志在每次读取后被清空。 |
| `/shutdown` | GET | 优雅关闭正在运行的 Broker。 |

**`/heartbeat` 返回示例：**

```json
{
  "Version": "1.1.0",
  "State": "running",
  "Timestamp": "2026-08-28 13:20:00",
  "Logs": [
    "MQTT Broker service started on port 1883"
  ],
  "BrokerPort": "1883"
}
```

### 四、用法示例

```bash
# 启动 MQTT Broker（监听 1883）
curl "http://127.0.0.1:11883/startup?port=1883"

# 查看状态与日志
curl "http://127.0.0.1:11883/heartbeat"

# 停止 Broker
curl "http://127.0.0.1:11883/shutdown"
```

### 五、内部实现
- **内核**：`hrotti.NewHrotti(100, &hrotti.MemoryPersistence{})` 创建一个使用内存持久化的 Broker 实例。
- **日志**：自定义 `LogInterceptor` 接管 `hrotti.INFO/DEBUG/ERROR` 输出，缓存最近 200 条日志供 `/heartbeat` 查询，并同步打印到标准输出。
- **WebUI**：通过 `go:embed` 将 `ui/` 静态资源嵌入到二进制，根路径 `/` 由 `http.FileServer` 服务，接口与面板共用一个端口。
- **线程安全**：`started` 标志与日志缓存由 `sync.Mutex` 保护，避免并发读写竞态。

### 六、已内置的改进（来自 MqttBrokerCore）
- 报文解析长度限制与内存上限，防止 OOM 和协议错误。
- 断线/慢客户端下的非阻塞发送，避免服务死锁。
- 空 `ClientIdentifier` 且 `CleanSession=0` 时按规范拒绝连接。
- 消息 ID 耗尽时的安全处理。

---

> 本项目使用的 MQTT 内核来自内部公开库 **[github.com/FasterEdge/MqttBrokerCore](https://github.com/FasterEdge/MqttBrokerCore)**，其上游为开源的 [alsm/hrotti](https://github.com/alsm/hrotti) 项目，特此致谢。