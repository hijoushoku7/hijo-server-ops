package mcversions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

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
	client, url := testClient(t, `{"versions":{"1.21.1":{},"1.20.6":{}}}`, func(r *http.Request) {
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
	want := []Version{{Version: "1.21.1", Stable: true}, {Version: "1.20.6", Stable: true}}
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
	body := `{"versions":["21.1.209","21.1.208","20.6.120-beta","0.25w14craftmine.3-beta","21.1.2.3"]}`
	client, url := testClient(t, body, nil)
	client.urls.neoForge = url
	got, err := client.NeoForge(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{
		{Version: "1.21.1", Stable: true, Loaders: []Loader{{Version: "21.1.209"}, {Version: "21.1.208"}}},
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
	versions := []Version{{Version: "1.9"}, {Version: "1.21.1"}, {Version: "1.21"}, {Version: "1.7.10_pre4"}, {Version: "1.20.6"}}
	sortVersions(versions)
	got := make([]string, len(versions))
	for i, v := range versions {
		got[i] = v.Version
	}
	want := []string{"1.21.1", "1.21", "1.20.6", "1.9", "1.7.10_pre4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortVersions = %v, want %v", got, want)
	}
}
