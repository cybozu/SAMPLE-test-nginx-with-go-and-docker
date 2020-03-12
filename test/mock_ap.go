package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
)

type MockAP struct {
	server *http.Server

	host string
	port int

	exited chan error // server が終了したらその err が入る

	lastRequest *http.Request
	mutex       sync.Mutex
}

func StartMockAP(t *testing.T) *MockAP {
	host := os.Getenv("TESTER_NAME")
	if host == "" {
		t.Fatal("Please specify TESTER_NAME")
	}

	l, err := net.Listen("tcp", ":0")
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

	go func() {
		if err := ap.server.Serve(l); err != nil && err != http.ErrServerClosed {
			t.Log(err)
		}
		ap.exited <- err
		close(ap.exited)
	}()

	return ap
}

func (a *MockAP) Address() string {
	return fmt.Sprintf("%s:%d", a.host, a.port)
}

func (a *MockAP) LastRequest() *http.Request {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.lastRequest
}

func (a *MockAP) Close(t *testing.T) {
	if err := a.server.Close(); err != nil {
		t.Log(err)
	}

	<-a.exited // 終了を待つ
}
