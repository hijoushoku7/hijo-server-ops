package mcversions

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fetchedAt := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	want := Cache{
		FetchedAt: fetchedAt,
		Vanilla:   []Version{{Version: "1.21.1", Stable: true}},
		Forge: []Version{{Version: "1.21.1", Stable: true,
			Loaders: []Loader{{Version: "52.0.0", Recommended: true}}}},
		FabricLoader: &FabricLoaderCache{Minecraft: "1.21.1", Loaders: []Loader{{Version: "0.16.10"}}},
	}
	if err := WriteCache(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadCache() = %#v, want %#v", got, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "versions.json" {
		t.Fatalf("cache files = %#v", entries)
	}
}

func TestCacheFresh(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		fetched time.Time
		want    bool
	}{
		{"TTL 内", now.Add(-CacheTTL + time.Second), true},
		{"TTL 境界", now.Add(-CacheTTL), false},
		{"未来", now.Add(time.Second), false},
		{"ゼロ値", time.Time{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Cache{FetchedAt: tt.fetched}).Fresh(now); got != tt.want {
				t.Errorf("Fresh() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadCacheMissingAndInvalid(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadCache(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "versions.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCache(dir); err == nil {
		t.Fatal("壊れたキャッシュでエラーが返らなかった")
	}
}
