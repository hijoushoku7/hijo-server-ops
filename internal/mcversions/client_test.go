package mcversions

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestVanillaJar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest":
			fmt.Fprintf(w, `{"versions":[{"id":"1.21.1","url":%q}]}`, "http://"+r.Host+"/version")
		case "/version":
			_, _ = w.Write([]byte(`{"downloads":{"server":{"url":"https://example.test/server.jar","sha1":"abc123"}}}`))
		}
	}))
	defer server.Close()
	client := NewClient(server.Client(), "dev")
	client.urls.vanilla = server.URL + "/manifest"
	got, err := client.VanillaJar(context.Background(), "1.21.1")
	if err != nil {
		t.Fatal(err)
	}
	want := ServerJar{URL: "https://example.test/server.jar", Sum: "abc123"}
	if got != want {
		t.Fatalf("VanillaJar() = %#v, want %#v", got, want)
	}
}

func TestPaperJar(t *testing.T) {
	body := `[{"id":61,"channel":"EXPERIMENTAL","downloads":{"server:default":{"url":"https://example.test/new.jar","checksums":{"sha256":"new"}}}},{"id":60,"channel":"STABLE","downloads":{"server:default":{"url":"https://example.test/stable.jar","checksums":{"sha256":"stable"}}}}]`
	client, url := testClient(t, body, func(r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "hso/v1.2.3 (https://github.com/hijoushoku7/hijo-server-ops)" {
			t.Errorf("User-Agent = %q", got)
		}
	})
	client.urls.paperBuilds = url + "/"
	got, err := client.PaperJar(context.Background(), "1.21.8")
	if err != nil {
		t.Fatal(err)
	}
	want := ServerJar{URL: "https://example.test/stable.jar", Sum: "stable", SHA256: true}
	if got != want {
		t.Fatalf("PaperJar() = %#v, want %#v", got, want)
	}
}

func TestFabricJar(t *testing.T) {
	client, url := testClient(t, `[{"version":"1.1.3","stable":false},{"version":"1.1.2","stable":true}]`, nil)
	client.urls.fabricInstaller = url
	client.urls.fabricServer = "https://example.test/loader/"
	got, err := client.FabricJar(context.Background(), "1.21.1", "0.16.10")
	if err != nil {
		t.Fatal(err)
	}
	want := ServerJar{URL: "https://example.test/loader/1.21.1/0.16.10/1.1.2/server/jar"}
	if got != want {
		t.Fatalf("FabricJar() = %#v, want %#v", got, want)
	}
}

func TestDownload(t *testing.T) {
	content := []byte("server jar")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(content) }))
	defer server.Close()
	t.Run("sha1", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "server.jar")
		sum := sha1.Sum(content)
		if err := Download(context.Background(), server.Client(), ServerJar{URL: server.URL, Sum: fmt.Sprintf("%x", sum)}, dest, nil); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(dest)
		if string(got) != string(content) {
			t.Fatalf("content = %q", got)
		}
	})
	t.Run("sha256 mismatch", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "server.jar")
		sum := sha256.Sum256([]byte("other"))
		if err := Download(context.Background(), server.Client(), ServerJar{URL: server.URL, Sum: fmt.Sprintf("%x", sum), SHA256: true}, dest, nil); err == nil {
			t.Fatal("不一致がエラーにならない")
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatalf("dest が残った: %v", err)
		}
	})
	t.Run("existing", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "server.jar")
		if err := os.WriteFile(dest, []byte("existing"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Download(context.Background(), server.Client(), ServerJar{URL: server.URL}, dest, nil); err == nil {
			t.Fatal("既存ファイルを拒否しない")
		}
		got, _ := os.ReadFile(dest)
		if string(got) != "existing" {
			t.Fatalf("existing = %q", got)
		}
	})
}

func testClient(t *testing.T, body string, check func(*http.Request)) (*Client, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return NewClient(server.Client(), "v1.2.3"), server.URL
}

func TestVanilla(t *testing.T) {
	client, url := testClient(t, `{"versions":[{"id":"1.21.1","type":"release"},{"id":"24w33a","type":"snapshot"}]}`, nil)
	client.urls.vanilla = url
	got, err := client.Vanilla(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{{Version: "1.21.1", Stable: true}, {Version: "24w33a"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Vanilla() = %#v, want %#v", got, want)
	}
}

func TestFabric(t *testing.T) {
	client, url := testClient(t, `[{"version":"1.21.1","stable":true},{"version":"24w33a","stable":false}]`, nil)
	client.urls.fabricGame = url
	got, err := client.Fabric(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{{Version: "1.21.1", Stable: true}, {Version: "24w33a"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Fabric() = %#v, want %#v", got, want)
	}
}

func TestFabricLoaders(t *testing.T) {
	client, url := testClient(t, `[{"loader":{"version":"0.16.10"}},{"loader":{"version":"0.16.9"}}]`, func(r *http.Request) {
		if r.URL.Path != "/1.21.1" {
			t.Errorf("path = %q", r.URL.Path)
		}
	})
	client.urls.fabricLoader = url + "/"
	got, err := client.FabricLoaders(context.Background(), "1.21.1")
	if err != nil {
		t.Fatal(err)
	}
	want := []Loader{{Version: "0.16.10"}, {Version: "0.16.9"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FabricLoaders() = %#v, want %#v", got, want)
	}
}

func TestPaper(t *testing.T) {
	client, url := testClient(t, `{"versions":{"26.3":["26.3","26.3-rc-3"],"1.21":["1.21.11","1.21.11-rc3","1.21.11-pre1","1.21.1","1.21"]}}`, func(r *http.Request) {
		want := "hso/v1.2.3 (https://github.com/hijoushoku7/hijo-server-ops)"
		if got := r.Header.Get("User-Agent"); got != want {
			t.Errorf("User-Agent = %q, want %q", got, want)
		}
	})
	client.urls.paper = url
	got, err := client.Paper(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{
		{Version: "26.3", Stable: true},
		{Version: "26.3-rc-3"},
		{Version: "1.21.11", Stable: true},
		{Version: "1.21.11-rc3"},
		{Version: "1.21.11-pre1"},
		{Version: "1.21.1", Stable: true},
		{Version: "1.21", Stable: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Paper() = %#v, want %#v", got, want)
	}
}

func TestForge(t *testing.T) {
	body := `{"promos":{"1.21.1-latest":"52.0.1","1.21.1-recommended":"52.0.0","1.20.6-latest":"50.1.0","1.20.6-broken":"ignored"}}`
	client, url := testClient(t, body, nil)
	client.urls.forge = url
	got, err := client.Forge(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{
		{Version: "1.21.1", Stable: true, Loaders: []Loader{{Version: "52.0.0", Recommended: true}, {Version: "52.0.1"}}},
		{Version: "1.20.6", Stable: true, Loaders: []Loader{{Version: "50.1.0"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Forge() = %#v, want %#v", got, want)
	}
}

func TestNeoForge(t *testing.T) {
	body := `{"versions":["21.1.209","20.6.120-beta","21.0.167","26.1.2.94","26.2.0.88","0.25w14craftmine.3-beta","26.1.0.0-alpha.1+snapshot-1"]}`
	client, url := testClient(t, body, nil)
	client.urls.neoForge = url
	got, err := client.NeoForge(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{
		{Version: "26.2", Stable: true, Loaders: []Loader{{Version: "26.2.0.88"}}},
		{Version: "26.1.2", Stable: true, Loaders: []Loader{{Version: "26.1.2.94"}}},
		{Version: "1.21.1", Stable: true, Loaders: []Loader{{Version: "21.1.209"}}},
		{Version: "1.21", Stable: true, Loaders: []Loader{{Version: "21.0.167"}}},
		{Version: "1.20.6", Stable: true, Loaders: []Loader{{Version: "20.6.120-beta"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NeoForge() = %#v, want %#v", got, want)
	}
}

func TestHTTPErrorAndInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusBadGateway)
	}))
	defer server.Close()
	client := NewClient(server.Client(), "dev")
	client.urls.vanilla = server.URL
	if _, err := client.Vanilla(context.Background()); err == nil {
		t.Fatal("HTTP エラーが返らなかった")
	}

	client, url := testClient(t, `{`, nil)
	client.urls.vanilla = url
	if _, err := client.Vanilla(context.Background()); err == nil {
		t.Fatal("JSON エラーが返らなかった")
	}
}

func TestSortVersionsOrdersNumerically(t *testing.T) {
	versions := []Version{{Version: "1.9"}, {Version: "26.3"}, {Version: "26.3-rc-3"}, {Version: "1.21.11"}, {Version: "1.21.1"}, {Version: "1.21"}, {Version: "1.7.10_pre4"}, {Version: "1.20.6"}}
	sortVersions(versions)
	got := make([]string, len(versions))
	for i, v := range versions {
		got[i] = v.Version
	}
	want := []string{"26.3", "26.3-rc-3", "1.21.11", "1.21.1", "1.21", "1.20.6", "1.9", "1.7.10_pre4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortVersions = %v, want %v", got, want)
	}
}
