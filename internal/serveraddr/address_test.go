package serveraddr

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchPublicIPv4(t *testing.T) {
	client := fakeHTTPClient{response: response(http.StatusOK, "203.0.113.10\n")}
	ip, err := fetchPublicIPv4(context.Background(), client, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if got := ip.String(); got != "203.0.113.10" {
		t.Fatalf("IP = %q", got)
	}
}

func TestFetchPublicIPv4RejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "status", status: http.StatusServiceUnavailable, body: "unavailable"},
		{name: "invalid", status: http.StatusOK, body: "not-an-ip"},
		{name: "ipv6", status: http.StatusOK, body: "2001:db8::1"},
		{name: "oversized", status: http.StatusOK, body: strings.Repeat("1", maxIPAddressBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := fetchPublicIPv4(
				context.Background(),
				fakeHTTPClient{response: response(test.status, test.body)},
				"https://example.test",
			); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

type fakeHTTPClient struct {
	response *http.Response
}

func (client fakeHTTPClient) Do(*http.Request) (*http.Response, error) {
	return client.response, nil
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestReadPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.properties")
	content := "# Minecraft server properties\r\n" +
		" server-port = 25565 \r\n" +
		"motd=test\r\n" +
		"server-port=25566\r\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	port, err := ReadPort(path)
	if err != nil {
		t.Fatal(err)
	}
	if port != 25566 {
		t.Fatalf("port = %d", port)
	}
}

func TestReadPortRejectsUnavailableAndInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "missing", content: "motd=test\n"},
		{name: "empty", content: "server-port=\n"},
		{name: "zero", content: "server-port=0\n"},
		{name: "overflow", content: "server-port=65536\n"},
		{name: "text", content: "server-port=minecraft\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "server.properties")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadPort(path); err == nil {
				t.Fatal("expected an error")
			}
		})
	}

	if _, err := ReadPort(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file was accepted")
	}
}
