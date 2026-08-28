# ===== 构建阶段 =====
FROM golang:1.25-alpine AS builder

WORKDIR /src

# 先复制依赖清单以利用 Docker 层缓存
COPY go.mod go.sum ./

# go.mod 声明了 toolchain go1.25.13，这里显式下载以利用层缓存
RUN go mod download

# 复制源码并构建
COPY main.go ./
COPY ui/ ./ui/
# 使用 go.mod 指定的工具链(go1.25.13，已修复 stdlib CVE)，禁用 cgo
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/simple-mqtt-broker .

# ===== 运行阶段 =====
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -H -u 1000 mqtt

WORKDIR /app

# 复制编译好的二进制
COPY --from=builder /out/simple-mqtt-broker /app/simple-mqtt-broker

# 非 root 运行
USER mqtt

# 暴露端口：
#   11883 — 管理接口 / WebUI（可用 MANAGE_PORT 覆盖）
#   1883  — MQTT 通信端口（启动时可用 port 参数或 MQTT_PORT 覆盖）
EXPOSE 11883 1883

# 健康检查：管理端点 /health 在 Broker 运行时返回 200
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:${MANAGE_PORT:-11883}/health || exit 1

ENTRYPOINT ["/app/simple-mqtt-broker"]