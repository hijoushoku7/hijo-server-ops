// Package mcversions は Minecraft サーバーのバージョン一覧を取得する。
package mcversions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const (
	vanillaURL      = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	fabricGameURL   = "https://meta.fabricmc.net/v2/versions/game"
	fabricLoaderURL = "https://meta.fabricmc.net/v2/versions/loader/"
	paperURL        = "https://fill.papermc.io/v3/projects/paper"
	forgeURL        = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"
	neoForgeURL     = "https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge"
)

// Version は Minecraft の版と、その版で選択できるローダーを表す。
// Stable はスナップショットを除外するか呼び出し側で判断するために使う。
type Version struct {
	Version string   `json:"version"`
	Stable  bool     `json:"stable"`
	Loaders []Loader `json:"loaders,omitempty"`
}

// Loader はローダーまたは Forge/NeoForge の版を表す。
type Loader struct {
	Version     string `json:"version"`
	Recommended bool   `json:"recommended,omitempty"`
}

// Client は各配布元からバージョン一覧を取得する。
type Client struct {
	httpClient *http.Client
	version    string
	urls       urls
}

type urls struct {
	vanilla, fabricGame, fabricLoader, paper, forge, neoForge string
}

// NewClient は一覧取得用のクライアントを作る。version は hso のバージョンで、
// Paper への User-Agent に使用する。
func NewClient(httpClient *http.Client, version string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{httpClient: httpClient, version: version, urls: urls{
		vanilla: vanillaURL, fabricGame: fabricGameURL, fabricLoader: fabricLoaderURL,
		paper: paperURL, forge: forgeURL, neoForge: neoForgeURL,
	}}
}

func (c *Client) get(ctx context.Context, url string, dst any, paper bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if paper {
		req.Header.Set("User-Agent", "hso/"+c.version+" (https://github.com/hijoushoku7/hijo-server-ops)")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

// Vanilla は Vanilla の Minecraft バージョン一覧を返す。
func (c *Client) Vanilla(ctx context.Context) ([]Version, error) {
	var response struct {
		Versions []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"versions"`
	}
	if err := c.get(ctx, c.urls.vanilla, &response, false); err != nil {
		return nil, err
	}
	versions := make([]Version, 0, len(response.Versions))
	for _, v := range response.Versions {
		versions = append(versions, Version{Version: v.ID, Stable: v.Type == "release"})
	}
	return versions, nil
}

// Fabric は Fabric が対応する Minecraft バージョン一覧を返す。
func (c *Client) Fabric(ctx context.Context) ([]Version, error) {
	var response []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := c.get(ctx, c.urls.fabricGame, &response, false); err != nil {
		return nil, err
	}
	versions := make([]Version, 0, len(response))
	for _, v := range response {
		versions = append(versions, Version{Version: v.Version, Stable: v.Stable})
	}
	return versions, nil
}

// FabricLoaders は指定した Minecraft バージョンの Fabric Loader 一覧を返す。
func (c *Client) FabricLoaders(ctx context.Context, minecraft string) ([]Loader, error) {
	var response []struct {
		Loader struct {
			Version string `json:"version"`
		} `json:"loader"`
	}
	if err := c.get(ctx, c.urls.fabricLoader+minecraft, &response, false); err != nil {
		return nil, err
	}
	loaders := make([]Loader, 0, len(response))
	for _, v := range response {
		loaders = append(loaders, Loader{Version: v.Loader.Version})
	}
	return loaders, nil
}

// Paper は Paper が対応する Minecraft バージョン一覧を返す。
func (c *Client) Paper(ctx context.Context) ([]Version, error) {
	var response struct {
		Versions map[string][]string `json:"versions"`
	}
	if err := c.get(ctx, c.urls.paper, &response, true); err != nil {
		return nil, err
	}
	var versions []Version
	for _, series := range response.Versions {
		for _, version := range series {
			stable := !strings.Contains(version, "-rc") && !strings.Contains(version, "-pre")
			versions = append(versions, Version{Version: version, Stable: stable})
		}
	}
	sortVersions(versions)
	return versions, nil
}

// Forge は promotions_slim.json から Minecraft と Forge の版を返す。
func (c *Client) Forge(ctx context.Context) ([]Version, error) {
	var response struct {
		Promos map[string]string `json:"promos"`
	}
	if err := c.get(ctx, c.urls.forge, &response, false); err != nil {
		return nil, err
	}
	type promos struct{ latest, recommended string }
	byMinecraft := make(map[string]promos)
	for key, forgeVersion := range response.Promos {
		minecraft, kind, ok := strings.Cut(key, "-")
		if !ok || (kind != "latest" && kind != "recommended") {
			continue
		}
		p := byMinecraft[minecraft]
		if kind == "latest" {
			p.latest = forgeVersion
		} else {
			p.recommended = forgeVersion
		}
		byMinecraft[minecraft] = p
	}
	versions := make([]Version, 0, len(byMinecraft))
	for minecraft, p := range byMinecraft {
		loaders := make([]Loader, 0, 2)
		if p.recommended != "" {
			loaders = append(loaders, Loader{Version: p.recommended, Recommended: true})
		}
		if p.latest != "" && p.latest != p.recommended {
			loaders = append(loaders, Loader{Version: p.latest})
		}
		if len(loaders) != 0 {
			versions = append(versions, Version{Version: minecraft, Stable: true, Loaders: loaders})
		}
	}
	sortVersions(versions)
	return versions, nil
}

// NeoForge は NeoForge の版から Minecraft の版を逆算して一覧を返す。
func (c *Client) NeoForge(ctx context.Context) ([]Version, error) {
	var response struct {
		Versions []string `json:"versions"`
	}
	if err := c.get(ctx, c.urls.neoForge, &response, false); err != nil {
		return nil, err
	}
	byMinecraft := make(map[string][]Loader)
	for _, loader := range response.Versions {
		minecraft, ok := neoForgeMinecraft(loader)
		if ok {
			byMinecraft[minecraft] = append(byMinecraft[minecraft], Loader{Version: loader})
		}
	}
	versions := make([]Version, 0, len(byMinecraft))
	for minecraft, loaders := range byMinecraft {
		versions = append(versions, Version{Version: minecraft, Stable: true, Loaders: loaders})
	}
	sortVersions(versions)
	return versions, nil
}

func neoForgeMinecraft(version string) (string, bool) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 && len(parts) != 4 {
		return "", false
	}
	// 通常版のほか、配布 API に含まれる「20.6.120-beta」の形も受け入れる。
	parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], "-beta")
	for _, part := range parts {
		if part == "" {
			return "", false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return "", false
			}
		}
	}
	minecraft := parts[:len(parts)-1]
	if len(parts) == 3 {
		minecraft = append([]string{"1"}, minecraft...)
	}
	if minecraft[len(minecraft)-1] == "0" {
		minecraft = minecraft[:len(minecraft)-1]
	}
	return strings.Join(minecraft, "."), true
}

// sortVersions は新しい版を先頭にする。文字列比較では "1.9" が "1.21" より
// 後ろになるので、数字の区切りごとに数値として比べる。
func sortVersions(versions []Version) {
	sort.Slice(versions, func(i, j int) bool {
		return compareVersion(versions[i].Version, versions[j].Version) > 0
	})
}

// compareVersion は a が b より新しければ正、古ければ負を返す。
// 数値にならない区切り（"1.7.10_pre4" の "10_pre4" など）は文字列として比べる。
func compareVersion(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		if aerr == nil && berr == nil {
			if an != bn {
				return an - bn
			}
			continue
		}
		if as[i] != bs[i] {
			return strings.Compare(as[i], bs[i])
		}
	}
	return len(as) - len(bs)
}
