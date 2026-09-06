package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"":          true,
		"127.0.0.1": true,
		"localhost": true,
		"::1":       true,
		"0.0.0.0":   false,
		"192.168.1.5": false,
		"10.0.0.1":  false,
	}
	for addr, want := range cases {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestCheckManageToken(t *testing.T) {
	old := manageToken
	defer func() { manageToken = old }()

	// 未配置令牌: 任何请求都放行 (本机模式)
	manageToken = ""
	if !checkManageToken(httptest.NewRequest(http.MethodGet, "/heartbeat", nil)) {
		t.Fatal("no-token mode should allow all requests")
	}

	// 配置令牌: 三种凭据来源均可通过, 错误令牌/缺失被拒绝
	manageToken = "s3cret-token"
	reqs := []struct {
		name string
		url  string
		hdrs map[string]string
		want bool
	}{
		{"bearer-ok", "/heartbeat", map[string]string{"Authorization": "Bearer s3cret-token"}, true},
		{"bearer-bad", "/heartbeat", map[string]string{"Authorization": "Bearer wrong"}, false},
		{"x-header-ok", "/heartbeat", map[string]string{"X-MqttBroker-Token": "s3cret-token"}, true},
		{"query-ok", "/heartbeat?token=s3cret-token", nil, true},
		{"query-bad", "/heartbeat?token=wrong", nil, false},
		{"missing", "/heartbeat", nil, false},
	}
	for _, c := range reqs {
		r := httptest.NewRequest(http.MethodGet, c.url, nil)
		for k, v := range c.hdrs {
			r.Header.Set(k, v)
		}
		if got := checkManageToken(r); got != c.want {
			t.Errorf("%s: checkManageToken = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestManageAuth401(t *testing.T) {
	old := manageToken
	defer func() { manageToken = old }()
	manageToken = "tk"

	h := manageAuth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/startup", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: got %d, want 401", rec.Code)
	}
	rec2 := httptest.NewRecorder()
	h(rec2, httptest.NewRequest(http.MethodGet, "/startup?token=tk", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("with token: got %d, want 200", rec2.Code)
	}
}

func TestStartupPortValidation(t *testing.T) {
	old := manageToken
	defer func() { manageToken = old }()
	manageToken = "" // 本机模式, 只测端口校验

	// 非法端口: 拒绝
	bad := []string{"0", "-1", "65536", "abc", "80/x"}
	for _, p := range bad {
		rec := httptest.NewRecorder()
		startMqttBroker(rec, httptest.NewRequest(http.MethodGet, "/startup?port="+p, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("port %q: got %d, want 400", p, rec.Code)
		}
	}
	// 合法/回退: 空端口回退默认 1883; "1883;rm" 被 Go query 解析在 ';' 处拆分,
	// 实际得到 port=1883 (合法) —— 注入面已被 Atoi 纯数字校验封死
	ok := []string{"", "1883;rm", "8080"}
	for _, p := range ok {
		rec := httptest.NewRecorder()
		startMqttBroker(rec, httptest.NewRequest(http.MethodGet, "/startup?port="+p, nil))
		if rec.Code == http.StatusBadRequest {
			t.Errorf("port %q: unexpected 400", p)
		}
	}
}
