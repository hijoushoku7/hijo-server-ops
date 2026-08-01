// Package serveraddr は、Minecraft サーバーの公開アドレス表示に必要な
// 公開 IPv4 と server-port を取得する。
package serveraddr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	publicIPv4URL     = "https://api.ipify.org"
	requestTimeout    = 3 * time.Second
	maxIPAddressBytes = 64
)

type Result struct {
	IP      netip.Addr
	Port    uint16
	IPErr   error
	PortErr error
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Resolve は公開 IPv4 と server.properties の server-port を取得する。
// ネットワーク障害は呼び出し側で非致命として扱えるよう Result に残す。
func Resolve(ctx context.Context, workDir string) Result {
	port, portErr := ReadPort(filepath.Join(workDir, "server.properties"))
	client := &http.Client{Timeout: requestTimeout}
	ip, ipErr := fetchPublicIPv4(ctx, client, publicIPv4URL)
	return Result{IP: ip, Port: port, IPErr: ipErr, PortErr: portErr}
}

func fetchPublicIPv4(
	ctx context.Context,
	client httpDoer,
	endpoint string,
) (netip.Addr, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("build public IPv4 request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("fetch public IPv4: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return netip.Addr{}, fmt.Errorf(
			"failed to fetch public IPv4: HTTP %d",
			response.StatusCode,
		)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxIPAddressBytes+1))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("read public IPv4 response: %w", err)
	}
	if len(body) > maxIPAddressBytes {
		return netip.Addr{}, errors.New("public IPv4 response is too long")
	}

	ip, err := netip.ParseAddr(strings.TrimSpace(string(body)))
	if err != nil || !ip.Is4() {
		return netip.Addr{}, errors.New("public IPv4 response is not an IPv4 address")
	}
	return ip, nil
}

// ReadPort は server.properties から server-port を読む。Minecraft が通常
// 出力する key=value 形式だけを対象にし、ファイル全体を書き換えない。
func ReadPort(path string) (uint16, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open server.properties: %w", err)
	}
	defer file.Close()

	var value string
	found := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "!") {
			continue
		}
		key, candidate, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "server-port" {
			continue
		}
		value = strings.TrimSpace(candidate)
		found = true
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read server.properties: %w", err)
	}
	if !found {
		return 0, errors.New("server.properties has no server-port")
	}

	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("invalid server-port: %q", value)
	}
	return uint16(parsed), nil
}
