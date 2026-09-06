// FasterEdge 开源项目 · https://github.com/FasterEdge · https://gitee.com/FasterEdge
package main

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	hrotti "github.com/FasterEdge/MqttBrokerCore/broker"
)

//go:embed ui/*
var uiFS embed.FS

// 管理端口（可通过环境变量 MANAGE_PORT 覆盖，默认 11883）
var managePort = envOr("MANAGE_PORT", "11883")

// 管理接口监听地址（默认仅本机 127.0.0.1）。
// 对外暴露（MANAGE_ADDR 非回环地址）时必须配置 MANAGE_TOKEN，否则拒绝启动——
// 防止局域网任意主机通过无鉴权的 /startup、/shutdown、/heartbeat 控制/干扰 Broker
// (与 DontCrack4 家族的 "对外监听必须配密码" fail-closed 守卫一致)。
var manageAddr = envOr("MANAGE_ADDR", "127.0.0.1")

// 管理接口访问令牌（空 = 本机模式无需令牌；非空 = /startup /heartbeat /shutdown 均需携带）
var manageToken = os.Getenv("MANAGE_TOKEN")

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
	version        = "1.0.20260902"      // 当前的内核版本
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

// 启动 MQTT Broker (幂等: 已启动则直接返回)
func launchBroker(port string) {
	mu.Lock()
	if started {
		mu.Unlock()
		return
	}
	brokerPort = port
	// 每次启动使用全新的关闭信号通道, 避免并发 /shutdown 重复 close
	shutdownSignal = make(chan struct{})
	sig := shutdownSignal
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

		<-sig
		h.Stop()
		mu.Lock()
		started = false
		mu.Unlock()
		fmt.Println("MQTT Broker service stopped.")
	}()
}

// isLoopbackAddr 判断地址是否为本机回环（仅本机可访问）。
func isLoopbackAddr(addr string) bool {
	switch strings.ToLower(strings.TrimSpace(addr)) {
	case "", "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// checkManageToken 校验管理接口令牌。
// 未配置令牌(本机模式)直接放行；凭据来源优先级: Authorization: Bearer > X-MqttBroker-Token > token 查询参数。
// 使用常数时间比较, 避免时序侧信道。
func checkManageToken(r *http.Request) bool {
	if manageToken == "" {
		return true
	}
	pw := ""
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		pw = strings.TrimPrefix(auth, "Bearer ")
	} else if v := r.Header.Get("X-MqttBroker-Token"); v != "" {
		pw = v
	} else {
		pw = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(pw), []byte(manageToken)) == 1
}

// manageAuth 包装管理接口: 令牌校验失败返回 401。
func manageAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkManageToken(r) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// startMqttBroker HTTP 入口: /startup?port=...
func startMqttBroker(w http.ResponseWriter, r *http.Request) {
	// 从查询参数中获取端口号，缺省使用 MQTT_PORT 环境变量或 1883
	port := r.URL.Query().Get("port")
	if port == "" {
		port = envOr("MQTT_PORT", "1883")
	}
	// 端口白名单: 仅接受 1-65535 纯数字, 拒绝任意字符串注入 (如 "25", "0", "abc",
	// 避免把 broker 绑到意外端口或被拼接进 tcp:// 地址)
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		http.Error(w, "invalid port: "+port, http.StatusBadRequest)
		return
	}

	mu.Lock()
	already := started
	mu.Unlock()
	launchBroker(port)
	if already {
		fmt.Fprintln(w, "already running")
		return
	}
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

// 关闭 Broker (幂等: 重复调用不 panic)
func stopBroker(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	select {
	case <-shutdownSignal:
		// 已关闭过, 直接返回
		mu.Unlock()
		fmt.Fprintln(w, "ok")
		return
	default:
	}
	close(shutdownSignal) // 发送关闭信号
	// 重置关闭信号通道
	shutdownSignal = make(chan struct{})
	mu.Unlock()
	fmt.Fprintln(w, "ok")
}

// 主函数
func main() {
	// 安全守卫: 对外暴露管理端口必须配置 MANAGE_TOKEN (fail-closed)。
	// 管理接口含 /startup /shutdown /heartbeat, 无鉴权时可能被局域网任意主机
	// 远程启停 Broker (DoS) 或读取日志; 默认仅绑定 127.0.0.1 拒绝外部访问。
	if !isLoopbackAddr(manageAddr) && manageToken == "" {
		fmt.Fprintln(os.Stderr, "安全拒绝启动: MANAGE_ADDR 指向外部网卡("+manageAddr+")时必须设置 MANAGE_TOKEN, 否则请保持默认 127.0.0.1")
		os.Exit(1)
	}
	if manageToken != "" && isLoopbackAddr(manageAddr) {
		fmt.Println("管理接口已启用令牌鉴权 (127.0.0.1:" + managePort + ")")
	}

	// 管理接口: /health 保持开放供 Docker HEALTHCHECK 使用, 其余控制端点均需令牌(配置时)
	http.HandleFunc("/startup", manageAuth(startMqttBroker))
	http.HandleFunc("/heartbeat", manageAuth(heartbeat))
	http.HandleFunc("/shutdown", manageAuth(stopBroker))

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

	// 容器/服务化场景: 可用 MQTT_AUTOSTART=1 启动时即监听 MQTT,
	// 使 Docker HEALTHCHECK 可立即通过 (否则 1883 需手动 /startup)
	if v := os.Getenv("MQTT_AUTOSTART"); v == "1" || strings.EqualFold(v, "true") {
		port := envOr("MQTT_PORT", "1883")
		fmt.Println("MQTT_AUTOSTART=1: 自动启动 MQTT Broker on port", port)
		launchBroker(port)
	}

	// 管理服务器带全套超时与头大小上限: 防御慢连接占资源(slowloris)与超大请求头
	srv := &http.Server{
		Addr:              net.JoinHostPort(manageAddr, managePort),
		Handler:           nil,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	err := srv.ListenAndServe()
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
