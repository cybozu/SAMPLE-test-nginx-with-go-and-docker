package main

import (
	"io/ioutil"
	"net/http"
	"testing"
)

func TestStaticResponses(t *testing.T) {
	t.Parallel()

	nginx := StartNginx(t, NginxConfig{})
	defer nginx.Close(t)
	nginx.Wait(t)

	t.Run("GET /robots.txt", func(t *testing.T) {
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
	})

	t.Run("GET /secret/", func(t *testing.T) {
		resp, err := http.Get(nginx.URL() + "/secret/")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("status code should be 400, but %d", resp.StatusCode)
		}
	})
}

func TestReverseProxy(t *testing.T) {
	t.Parallel()

	ap := StartMockAP(t)
	defer ap.Close(t)

	nginx := StartNginx(t, NginxConfig{
		APServerAddress: ap.Address(),
	})
	defer nginx.Close(t)
	nginx.Wait(t)

	t.Run("response should be returned from AP", func(t *testing.T) {
		resp, err := http.Get(nginx.URL() + "/")
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
		expected := "I am AP server"
		if string(body) != expected {
			t.Errorf("unexpected response body: %s", string(body))
		}
	})

	t.Run("X-Request-Id should be sent to AP", func(t *testing.T) {
		resp, err := http.Get(nginx.URL() + "/")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		requestID := ap.LastRequest().Header.Get("X-Request-Id")
		if requestID == "" {
			t.Error("X-Request-Id header does not exist")
		}
	})
}
