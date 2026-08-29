package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	hrotti "github.com/FasterEdge/MqttBrokerCore/broker"
)

//go:embed ui/*
var uiFS embed.FS

// 管理端口（可通过环境变量 MANAGE_PORT 覆盖，默认 11883）
var managePort = envOr("MANAGE_PORT", "11883")

// envOr 读取环境变量，为空时返回默认值
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// 回参结构体
type HeartbeatInfo struct {
	Version    string   // 当前内核版本
	State      string   // 当前服务状态
	Info       string   // 传递信息
	Timestamp  string   // 时间戳
	Logs       []string // 当前的一批日志
	BrokerPort string   // 当前在运行的Broker的端口
}

// 全局状态
var (
	version        = "1.0.20260829"             // 当前的内核版本
	started        = false               // 是否启动
	brokerPort     = "1883"              // 默认 MQTT 端口
	logsCache      []string              // 日志缓存
	shutdownSignal = make(chan struct{}) // 服务关闭信号
	mu             sync.Mutex            // 用于保护日志缓存的互斥锁
)

// 自定义日志拦截器
type LogInterceptor struct{}

// 实现 io.Writer 接口
func (li *LogInterceptor) Write(p []byte) (n int, err error) {
	mu.Lock()         // 加锁保护日志缓存
	defer mu.Unlock() // 延迟解锁

	logsCache = append(logsCache, string(p))

	// 限制 logsCache 的大小，防止内存溢出
	if len(logsCache) > 200 {
		logsCache = logsCache[len(logsCache)-200:]
	}

	// 同时输出到标准输出
	fmt.Print(string(p))

	return len(p), nil
}

// 启动 MQTT Broker
func startMqttBroker(w http.ResponseWriter, r *http.Request) {
	// 从查询参数中获取端口号，缺省使用 MQTT_PORT 环境变量或 1883
	port := r.URL.Query().Get("port")
	if port == "" {
		port = envOr("MQTT_PORT", "1883")
	}

	brokerPort = port

	// 避免重复启动
	mu.Lock()
	if started {
		mu.Unlock()
		fmt.Fprintln(w, "already running")
		return
	}
	mu.Unlock()

	go func() {
		h := hrotti.NewHrotti(100, &hrotti.MemoryPersistence{})

		// 使用自定义日志处理器
		logInterceptor := &LogInterceptor{}
		hrotti.INFO.SetOutput(logInterceptor)
		hrotti.DEBUG.SetOutput(logInterceptor)
		hrotti.ERROR.SetOutput(logInterceptor)

		lc, err := hrotti.NewListenerConfigWithError("tcp://0.0.0.0:" + port)
		if err != nil {
			fmt.Println("Failed to create listener config:", err)
			return
		}
		if err := h.AddListener("test", lc); err != nil {
			fmt.Println("Failed to start listener:", err)
			return
		}

		mu.Lock()
		started = true
		mu.Unlock()
		fmt.Println("MQTT Broker service started on port", port)

		<-shutdownSignal
		h.Stop()
		mu.Lock()
		started = false
		mu.Unlock()
		fmt.Println("MQTT Broker service stopped.")
	}()

	fmt.Fprintln(w, "ok")
}

// 心跳接口
func heartbeat(w http.ResponseWriter, r *http.Request) {
	mu.Lock()         // 加锁保护日志读取
	defer mu.Unlock() // 延迟解锁

	now_state := "unknown"
	if started {
		now_state = "running"
	} else {
		now_state = "stopped"
	}

	currentTime := time.Now()
	// 创建日志副本，避免清空缓存时影响并发写入
	logsCopy := make([]string, len(logsCache))
	copy(logsCopy, logsCache)

	// 清空日志缓存（需在解锁前操作，确保原子性）
	logsCache = make([]string, 0)

	info := HeartbeatInfo{
		Version:    version,
		State:      now_state,
		Logs:       logsCopy,
		Timestamp:  currentTime.Format("2006-01-02 15:04:05"),
		BrokerPort: brokerPort,
	}

	jsonData, err := json.Marshal(info)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Fprintln(w, string(jsonData))
}

// 关闭 Broker
func stopBroker(w http.ResponseWriter, r *http.Request) {
	close(shutdownSignal) // 发送关闭信号
	// 重置关闭信号通道
	shutdownSignal = make(chan struct{})
	fmt.Fprintln(w, "ok")
}

// 主函数
func main() {
	// 管理接口
	http.HandleFunc("/startup", startMqttBroker)
	http.HandleFunc("/heartbeat", heartbeat)
	http.HandleFunc("/shutdown", stopBroker)

	// 健康检查端点，供 Docker HEALTHCHECK 使用
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if started {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "ok")
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, "stopped")
	})

	// 嵌入式 WebUI，作为根路径的兜底
	http.Handle("/", http.FileServer(http.FS(uiFS)))

	fmt.Println("Web UI available at http://127.0.0.1:" + managePort + "/")
	err := http.ListenAndServe(":"+managePort, nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
