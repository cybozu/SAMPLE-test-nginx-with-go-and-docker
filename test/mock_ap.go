package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
)

// MockAP は AP サーバーのモック
type MockAP struct {
	server *http.Server

	host string
	port int

	exited chan error // server が終了したらその err が入る

	lastRequest *http.Request
	mutex       sync.Mutex
}

// StartMockAP は MockAP を起動する。
func StartMockAP(t *testing.T) *MockAP {
	host := os.Getenv("TESTER_NAME")
	if host == "" {
		t.Fatal("Please specify TESTER_NAME")
	}

	l, err := net.Listen("tcp", ":0") // 空いているポートを自動的に選ぶ
	if err != nil {
		t.Fatal(err)
	}

	ap := &MockAP{
		host:   host,
		port:   l.Addr().(*net.TCPAddr).Port,
		exited: make(chan error),
	}
	ap.server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ap.mutex.Lock()
			defer ap.mutex.Unlock()
			ap.lastRequest = req

			w.Write([]byte("I am AP server"))
		}),
	}

	// 別の goroutine でサーバーを走らせる。終了したら ap.exited に値が入るようにしておく。
	go func() {
		if err := ap.server.Serve(l); err != nil && err != http.ErrServerClosed {
			t.Log(err)
		}
		ap.exited <- err
		close(ap.exited)
	}()

	return ap
}

// Address は MockAP にアクセスするためのアドレスを返す。
func (a *MockAP) Address() string {
	return fmt.Sprintf("%s:%d", a.host, a.port)
}

// LastRequest は最後に受け取ったリクエストを返す。
// リクエストをまだ受け取っていない場合は nil を返す。
func (a *MockAP) LastRequest() *http.Request {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.lastRequest
}

// Close は MockAP を破棄する。
func (a *MockAP) Close(t *testing.T) {
	if err := a.server.Close(); err != nil {
		t.Log(err)
	}

	<-a.exited // 終了を待つ
}
