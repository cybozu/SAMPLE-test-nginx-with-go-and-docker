package main

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

//-----------------------------------------------------------------------------

type Nginx struct {
	cmd           *exec.Cmd
	containerName string
	exited        chan int // 終了したら ExitCode が入る
}

type NginxConfig struct {
	APServerAddress string
}

// StartNginx は新しい nginx のインスタンスを起動する。
func StartNginx(t *testing.T, config NginxConfig) *Nginx {
	name := "nginx-" + randomSuffix()

	network := os.Getenv("DOCKER_NETWORK")
	if network == "" {
		t.Fatal("Please specify DOCKER_NETWORK")
	}

	if config.APServerAddress == "" {
		config.APServerAddress = "localhost:9999" // 無効なポート
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

// URL はこの nginx にアクセスするための URL を返す。
func (n *Nginx) URL() string {
	return fmt.Sprintf("http://%s:80", n.containerName)
}

// Wait はこの nginx が起動するまで待つ。
func (n *Nginx) Wait(t *testing.T) {
	maxRetry := 20
	for i := 0; i < maxRetry; i++ {
		t.Logf("Wait for nginx... (%d/%d)", i, maxRetry)

		select {
		case exitCode := <- n.exited:
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

// Close はこの nginx を終了する。
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
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return strings.ToLower(enc.EncodeToString(b))
}


//-----------------------------------------------------------------------------

func TestRobotsTxt(t *testing.T) {
	t.Parallel()

	nginx := StartNginx(t, NginxConfig{})
	defer nginx.Close(t)
	nginx.Wait(t)

	resp, err := http.Get(nginx.URL() + "/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code should be 200, but %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	expected := "User-agent: *\nDisallow: /\n"
	if string(body) != expected {
		t.Errorf("unexpected response body: %s", string(body))
	}
}
