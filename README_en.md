<div align="center">
  <img src="Logo.png" alt="MqttBroker" width="120"/>
  <h2>MqttBroker</h2>
  <h3>A Lightweight MQTT Service Based on FasterEdge MqttBrokerCore</h3>
</div>

### 1. Introduction
- An MQTT broker service built on **[FasterEdge MqttBrokerCore](https://github.com/FasterEdge/MqttBrokerCore)**, with HTTP endpoints and a built-in **WebUI** to easily control the lifecycle of the underlying Broker.
- Uses a single management port `11883`, providing both:
  - **REST API**: `/startup`, `/heartbeat`, `/shutdown`.
  - **Web management panel**: start / stop the Broker in the browser and view runtime status and logs in real time.
- The MQTT communication port defaults to `1883`, and can be specified dynamically via `/startup?port=...`.
- All underlying capabilities come from `MqttBrokerCore`: supports **MQTT 3.1.1 / 5.0**, **QoS 0/1/2**, will messages, retained messages, and topic wildcards (`+`, `#`).
- All static assets are embedded into the binary via `go:embed`, so a single file can be deployed without any additional static directory.

### 2. Key Features
| Feature | Description |
|---------|-------------|
| HTTP control | Start/stop the Broker via REST endpoints, easy to integrate with scripts, monitoring and alerting systems |
| Built-in WebUI | No installation needed; start/stop the service and view logs from a web page, ideal for quick trials |
| Dynamic port | Specify the listening port with the `port` parameter at startup, adapting flexibly to different environments |
| In-memory persistence | `MemoryPersistence` by default — works out of the box with no external dependencies |
| Single-file deployment | After `go build`, only one executable is needed to run |
| Containerized | Ships a `Dockerfile` with multi-stage builds and non-root execution |

### 3. Quick Start

> **Environment requirement**: `go.mod` declares `toolchain go1.25.13` (with standard-library CVEs fixed). Go 1.21+ will automatically download and use this toolchain; to specify manually, use `GOTOOLCHAIN=go1.25.13 go build`. Builds always disable CGO (`CGO_ENABLED=0`).

```bash
go mod tidy
go build ./...

# Start the management service (listens on 11883 by default)
./com.tyza66.SimpleMqttBrokerApi
# Or run from source directly
go run main.go
```

After startup, open `http://127.0.0.1:11883/` in the browser to access the Web management panel.

**Run with Docker:**

```bash
docker build -t fasteredge/mqtt-broker .

# Start the container, mapping the management port (11883) and the MQTT port (1883)
docker run -d -p 11883:11883 -p 1883:1883 fasteredge/mqtt-broker
```

**Docker environment variables:**

| Environment variable | Default | Description |
|----------------------|---------|-------------|
| `MANAGE_PORT` | `11883` | Management API / WebUI port |
| `MQTT_PORT` | `1883` | Default MQTT listening port when the `port` parameter is not specified |

> Inside the container, `http://127.0.0.1:11883/health` can be used for health checks (returns 200 while the Broker is running).

### 4. REST API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/startup?port=1883` | GET | Starts the MQTT Broker listening on the specified port (default `1883`). Repeated startup returns `already running`. |
| `/heartbeat` | GET | Returns JSON with version, running state, log cache, timestamp and the Broker port. Logs are cleared after each read. |
| `/shutdown` | GET | Gracefully shuts down the running Broker. |

**Example `/heartbeat` response:**

```json
{
  "Version": "1.0.20260829",
  "State": "running",
  "Timestamp": "2026-08-28 13:20:00",
  "Logs": [
    "MQTT Broker service started on port 1883"
  ],
  "BrokerPort": "1883"
}
```

### 5. Usage Examples

```bash
# Start the MQTT Broker (listening on 1883)
curl "http://127.0.0.1:11883/startup?port=1883"

# View status and logs
curl "http://127.0.0.1:11883/heartbeat"

# Stop the Broker
curl "http://127.0.0.1:11883/shutdown"
```

### 6. Directory Structure

```
MqttBroker/
├─ main.go          # Entry point: HTTP endpoints + embedded WebUI (supports MANAGE_PORT / MQTT_PORT)
├─ ui/
│  └─ index.html    # Web management panel (embedded via go:embed)
├─ go.mod           # Depends on the FasterEdge MqttBrokerCore module
├─ Dockerfile       # Multi-stage container build + HEALTHCHECK
├─ .dockerignore
├─ Logo.png
└─ README.md
```

### 7. Internal Implementation
- **Kernel**: `hrotti.NewHrotti(100, &hrotti.MemoryPersistence{})` creates a Broker instance backed by in-memory persistence.
- **Logging**: a custom `LogInterceptor` takes over `hrotti.INFO/DEBUG/ERROR` output, caches the most recent 200 log entries for `/heartbeat` and also prints them to standard output.
- **WebUI**: the `ui/` static assets are embedded into the binary via `go:embed`; the root path `/` is served by `http.FileServer`, sharing one port with the management endpoints.
- **Thread safety**: the `started` flag and log cache are protected by `sync.Mutex` to avoid concurrent read/write races.

### 8. Improvements Built In (from MqttBrokerCore)
- Packet parsing length limits and memory caps to prevent OOM and protocol errors.
- Non-blocking sends for disconnected / slow clients to avoid deadlocks.
- Connections with an empty `ClientIdentifier` and `CleanSession=0` are rejected per spec.
- Safe handling when the message ID pool is exhausted.

---

> The MQTT kernel used by this project comes from the internal public library **[github.com/FasterEdge/MqttBrokerCore](https://github.com/FasterEdge/MqttBrokerCore)**, whose upstream is the open-source [alsm/hrotti](https://github.com/alsm/hrotti) project. Special thanks.