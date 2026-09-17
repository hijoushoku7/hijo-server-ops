package mcversions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	cacheFilename = "versions.json"
	// CacheTTL はバージョン一覧を再取得するまでの時間。
	CacheTTL = 12 * time.Hour
)

// FabricLoaderCache は直近に選んだ Minecraft バージョンの Loader 一覧を保持する。
// 取得時刻を自分で持つのは、バージョン一覧と別のタイミングで取りに行くため。
type FabricLoaderCache struct {
	Minecraft string    `json:"minecraft"`
	FetchedAt time.Time `json:"fetched_at"`
	Loaders   []Loader  `json:"loaders"`
}

// Fresh は取得時刻から TTL 内なら true を返す。
func (c FabricLoaderCache) Fresh(now time.Time) bool {
	return fresh(c.FetchedAt, now)
}

// Cache は配布元から取得したバージョン一覧のキャッシュ。
type Cache struct {
	FetchedAt    time.Time          `json:"fetched_at"`
	Vanilla      []Version          `json:"vanilla,omitempty"`
	Fabric       []Version          `json:"fabric,omitempty"`
	Paper        []Version          `json:"paper,omitempty"`
	Forge        []Version          `json:"forge,omitempty"`
	NeoForge     []Version          `json:"neoforge,omitempty"`
	FabricLoader *FabricLoaderCache `json:"fabric_loader,omitempty"`
}

// Fresh は取得時刻から TTL 内なら true を返す。
func (c Cache) Fresh(now time.Time) bool {
	return fresh(c.FetchedAt, now)
}

func fresh(fetchedAt, now time.Time) bool {
	return !fetchedAt.IsZero() && !now.Before(fetchedAt) && now.Sub(fetchedAt) < CacheTTL
}

// ReadCache は dir 内の versions.json を読む。
func ReadCache(dir string) (Cache, error) {
	data, err := os.ReadFile(filepath.Join(dir, cacheFilename))
	if err != nil {
		return Cache{}, err
	}
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return Cache{}, err
	}
	return cache, nil
}

// WriteCache は dir 内の versions.json を一時ファイルからの rename で置き換える。
func WriteCache(dir string, cache Cache) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(dir, ".versions-*.json")
	if err != nil {
		return err
	}
	name := temporary.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(name)
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filepath.Join(dir, cacheFilename)); err != nil {
		return err
	}
	remove = false
	return nil
}
