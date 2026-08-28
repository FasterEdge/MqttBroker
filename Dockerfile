# 构建阶段
FROM golang:1.25-alpine AS builder

WORKDIR /src

# 先复制依赖清单以利用缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并构建
COPY main.go ./
COPY ui/ ./ui/
RUN CGO_ENABLED=0 go build -o /app/simple-mqtt-broker .

# 运行阶段
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -H -u 1000 mqtt

WORKDIR /app
COPY --from=builder /app/simple-mqtt-broker /app/simple-mqtt-broker

USER mqtt

# 管理端口 / WebUI
EXPOSE 11883

ENTRYPOINT ["/app/simple-mqtt-broker"]
