package mcversions

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ForgeInstaller は指定した Forge のサーバーインストーラを返す。
func (c *Client) ForgeInstaller(ctx context.Context, minecraft, forge string) (ServerJar, error) {
	version := minecraft + "-" + forge
	url := c.urls.forgeMaven + version + "/forge-" + version + "-installer.jar"
	sum, err := c.checksum(ctx, url+".sha1")
	if err != nil {
		return ServerJar{}, err
	}
	return ServerJar{URL: url, Sum: sum}, nil
}

// NeoForgeInstaller は指定した NeoForge のサーバーインストーラを返す。
func (c *Client) NeoForgeInstaller(ctx context.Context, version string) (ServerJar, error) {
	url := c.urls.neoForgeMaven + version + "/neoforge-" + version + "-installer.jar"
	sum, err := c.checksum(ctx, url+".sha1")
	if err != nil {
		return ServerJar{}, err
	}
	return ServerJar{URL: url, Sum: sum}, nil
}

// ForgeLoaders は指定した Minecraft バージョン向けの全 Forge ビルドを返す。
func (c *Client) ForgeLoaders(ctx context.Context, minecraft string) ([]Loader, error) {
	url := c.urls.forgeMaven + "maven-metadata.xml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	var metadata struct {
		Versions []string `xml:"versioning>versions>version"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode %s: %w", url, err)
	}
	prefix := minecraft + "-"
	loaders := make([]Loader, 0)
	for _, version := range metadata.Versions {
		if strings.HasPrefix(version, prefix) {
			loaders = append(loaders, Loader{Version: strings.TrimPrefix(version, prefix)})
		}
	}
	return loaders, nil
}

func (c *Client) checksum(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", err
	}
	sum := strings.TrimSpace(string(content))
	decoded, err := hex.DecodeString(sum)
	if err != nil || len(decoded) != sha1.Size {
		return "", fmt.Errorf("invalid SHA-1 from %s", url)
	}
	return strings.ToLower(sum), nil
}

// ServerJar はダウンロードするサーバー jar の在り処と検証値。
type ServerJar struct {
	URL    string
	Sum    string
	SHA256 bool
}

// VanillaJar は指定した Vanilla のサーバー jar を返す。
func (c *Client) VanillaJar(ctx context.Context, minecraft string) (ServerJar, error) {
	var manifest struct {
		Versions []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"versions"`
	}
	if err := c.get(ctx, c.urls.vanilla, &manifest, false); err != nil {
		return ServerJar{}, err
	}
	var versionURL string
	for _, version := range manifest.Versions {
		if version.ID == minecraft {
			versionURL = version.URL
			break
		}
	}
	if versionURL == "" {
		return ServerJar{}, fmt.Errorf("vanilla version not found: %s", minecraft)
	}
	var version struct {
		Downloads struct {
			Server *struct {
				URL  string `json:"url"`
				SHA1 string `json:"sha1"`
			} `json:"server"`
		} `json:"downloads"`
	}
	if err := c.get(ctx, versionURL, &version, false); err != nil {
		return ServerJar{}, err
	}
	if version.Downloads.Server == nil || version.Downloads.Server.URL == "" ||
		version.Downloads.Server.SHA1 == "" {
		return ServerJar{}, fmt.Errorf("vanilla server download not found: %s", minecraft)
	}
	return ServerJar{URL: version.Downloads.Server.URL, Sum: version.Downloads.Server.SHA1}, nil
}

// PaperJar は指定した Paper の最新ビルドの server jar を返す。
func (c *Client) PaperJar(ctx context.Context, minecraft string) (ServerJar, error) {
	type paperDownload struct {
		URL       string `json:"url"`
		Checksums struct {
			SHA256 string `json:"sha256"`
		} `json:"checksums"`
	}
	var builds []struct {
		Channel   string                   `json:"channel"`
		Downloads map[string]paperDownload `json:"downloads"`
	}
	if err := c.get(ctx, c.urls.paperBuilds+minecraft+"/builds", &builds, true); err != nil {
		return ServerJar{}, err
	}
	if len(builds) == 0 {
		return ServerJar{}, fmt.Errorf("paper build not found: %s", minecraft)
	}
	selected := builds[0]
	for _, build := range builds {
		if build.Channel == "STABLE" {
			selected = build
			break
		}
	}
	download, ok := selected.Downloads["server:default"]
	if !ok || download.URL == "" || download.Checksums.SHA256 == "" {
		return ServerJar{}, fmt.Errorf("paper server download not found: %s", minecraft)
	}
	return ServerJar{URL: download.URL, Sum: download.Checksums.SHA256, SHA256: true}, nil
}

// FabricJar は指定した Fabric のサーバー jar を返す。
func (c *Client) FabricJar(ctx context.Context, minecraft, loader string) (ServerJar, error) {
	var installers []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := c.get(ctx, c.urls.fabricInstaller, &installers, false); err != nil {
		return ServerJar{}, err
	}
	installer := ""
	for _, item := range installers {
		if item.Stable {
			installer = item.Version
			break
		}
	}
	if installer == "" {
		return ServerJar{}, fmt.Errorf("stable fabric installer not found")
	}
	return ServerJar{URL: c.urls.fabricServer + minecraft + "/" + loader + "/" + installer + "/server/jar"}, nil
}

// Download は jar を同じディレクトリの一時ファイルへ保存し、検証後に dest へ移す。
func Download(ctx context.Context, httpClient *http.Client, jar ServerJar, dest string, progress func(done, total int64)) error {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jar.URL, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: %s", jar.URL, resp.Status)
	}

	temporary, err := os.CreateTemp(filepath.Dir(dest), ".server-*.jar")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)

	var digest hash.Hash
	if jar.Sum != "" {
		if jar.SHA256 {
			digest = sha256.New()
		} else {
			digest = sha1.New()
		}
	}
	reader := io.Reader(resp.Body)
	if digest != nil {
		reader = io.TeeReader(reader, digest)
	}
	reader = &progressReader{reader: reader, total: resp.ContentLength, progress: progress}
	if _, err := io.Copy(temporary, reader); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if digest != nil && hex.EncodeToString(digest.Sum(nil)) != jar.Sum {
		return fmt.Errorf("checksum mismatch for %s", jar.URL)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	// rename は既存ファイルを置き換えてしまうので link で置く。dest があれば失敗する。
	return os.Link(name, dest)
}

type progressReader struct {
	reader   io.Reader
	total    int64
	done     int64
	last     time.Time
	progress func(done, total int64)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.done += int64(n)
	now := time.Now()
	if r.progress != nil && (r.last.IsZero() || now.Sub(r.last) >= 100*time.Millisecond || err == io.EOF) {
		r.progress(r.done, r.total)
		r.last = now
	}
	return n, err
}
