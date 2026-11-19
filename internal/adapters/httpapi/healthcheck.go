package httpapi

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// CheckHealth проверяет, что сервер, слушающий addr (формат ADDR: ":8080", "0.0.0.0:9000"),
// отвечает на /healthz. Нужен для HEALTHCHECK в Docker: в образе нет curl.
func CheckHealth(addr string) error {
	url, err := healthURL(addr)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return nil
}

// healthURL строит адрес проверки по ADDR: пустой хост и «любой адрес» (0.0.0.0, ::) заменяются на локальный.
func healthURL(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("ADDR=%q: %w", addr, err)
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz", nil
}
