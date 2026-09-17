package mcversions

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestForgeAndNeoForgeInstaller(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/forge/1.20.1-47.4.0/forge-1.20.1-47.4.0-installer.jar.sha1":
			fmt.Fprintln(w, "0123456789abcdef0123456789abcdef01234567")
		case "/neoforge/21.1.209/neoforge-21.1.209-installer.jar.sha1":
			fmt.Fprintln(w, "89abcdef0123456789abcdef0123456789abcdef")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.Client(), "test")
	client.urls.forgeMaven = server.URL + "/forge/"
	client.urls.neoForgeMaven = server.URL + "/neoforge/"
	forge, err := client.ForgeInstaller(context.Background(), "1.20.1", "47.4.0")
	if err != nil {
		t.Fatal(err)
	}
	if forge.URL != server.URL+"/forge/1.20.1-47.4.0/forge-1.20.1-47.4.0-installer.jar" || forge.Sum != "0123456789abcdef0123456789abcdef01234567" || forge.SHA256 {
		t.Fatalf("ForgeInstaller() = %#v", forge)
	}
	neoForge, err := client.NeoForgeInstaller(context.Background(), "21.1.209")
	if err != nil {
		t.Fatal(err)
	}
	if neoForge.URL != server.URL+"/neoforge/21.1.209/neoforge-21.1.209-installer.jar" || neoForge.Sum != "89abcdef0123456789abcdef0123456789abcdef" || neoForge.SHA256 {
		t.Fatalf("NeoForgeInstaller() = %#v", neoForge)
	}
}

func TestForgeLoaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<metadata><versioning><versions>
<version>1.21-51.0.0</version><version>1.20.1-47.4.0</version>
<version>1.20.1-47.3.22</version><version>1.19.4-45.2.0</version>
</versions></versioning></metadata>`)
	}))
	defer server.Close()
	client := NewClient(server.Client(), "test")
	client.urls.forgeMaven = server.URL + "/"
	got, err := client.ForgeLoaders(context.Background(), "1.20.1")
	if err != nil {
		t.Fatal(err)
	}
	want := []Loader{{Version: "47.4.0"}, {Version: "47.3.22"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ForgeLoaders() = %#v, want %#v", got, want)
	}
}
