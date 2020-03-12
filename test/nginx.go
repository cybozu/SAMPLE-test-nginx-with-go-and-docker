package main

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Nginx はテスト対象となる nginx のプロセスを表す。
type Nginx struct {
	cmd           *exec.Cmd
	containerName string
	exited        chan int // 終了したら ExitCode が入る
}

// NginxConfig は nginx を起動するために必要なオプションを保持する。
type NginxConfig struct {
	APServerAddress string
}

// StartNginx は新しい nginx のプロセスを起動する。
func StartNginx(t *testing.T, config NginxConfig) *Nginx {
	name := "nginx-" + randomSuffix()

	network := os.Getenv("DOCKER_NETWORK")
	if network == "" {
		t.Fatal("Please specify DOCKER_NETWORK")
	}

	if config.APServerAddress == "" {
		// 誰も listen していないポートを指定することで bad gateway になるようにする
		config.APServerAddress = "localhost:9999"
	}

	args := []string{
		"run", "--rm",
		"--name", name,
		"--net", network,
		"-e", fmt.Sprintf("AP_SERVER_ADDR=%s", config.APServerAddress),
		"sample-nginx:latest",
	}
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	nginx := &Nginx{
		cmd:           cmd,
		containerName: name,
		exited:        make(chan int),
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			t.Log(err)
		}
		nginx.exited <- cmd.ProcessState.ExitCode()
		close(nginx.exited)
	}()

	return nginx
}

// URL は nginx にアクセスするための URL を返す。
func (n *Nginx) URL() string {
	return fmt.Sprintf("http://%s:80", n.containerName)
}

// Wait は nginx が起動するまで待つ。
func (n *Nginx) Wait(t *testing.T) {
	maxRetry := 20
	for i := 0; i < maxRetry; i++ {
		t.Logf("Wait for nginx... (%d/%d)", i, maxRetry)

		select {
		case exitCode := <-n.exited:
			t.Fatalf("nginx exited unexpectedly: exitCode=%d", exitCode)
		default:
		}

		resp, err := http.Get(n.URL() + "/health")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		resp.Body.Close()
		return
	}
}

// Close は nginx を終了する。
func (n *Nginx) Close(t *testing.T) {
	cmd := exec.Command("docker", "kill", n.containerName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	<-n.exited // 終了するまで待つ
}

// randomSuffix はコンテナ名やボリューム名の suffix として使うためのランダム文字列を返す
func randomSuffix() string {
	b := make([]byte, 6)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return strings.ToLower(enc.EncodeToString(b))
}
