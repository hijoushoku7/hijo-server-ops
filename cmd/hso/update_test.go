package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

func preserveUpdateGlobals(t *testing.T) {
	t.Helper()
	previousVersion := version
	previousURL := latestReleaseURL
	previousVersionClient := versionHTTPClient
	previousUpdateClient := updateHTTPClient
	previousExecutablePath := executablePath
	t.Cleanup(func() {
		version = previousVersion
		latestReleaseURL = previousURL
		versionHTTPClient = previousVersionClient
		updateHTTPClient = previousUpdateClient
		executablePath = previousExecutablePath
	})
}

func TestLatestReleaseExtractsTagAndAssets(t *testing.T) {
	preserveUpdateGlobals(t)
	version = "v1.2.3"
	t.Setenv("GITHUB_TOKEN", "test-token")

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("User-Agent"); got != "hso/v1.2.3" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		fmt.Fprint(response, `{
			"tag_name":"v2.0.0",
			"assets":[
				{"name":"hso_v2.0.0_linux_amd64_ja.tar.gz","browser_download_url":"https://example.test/hso.tar.gz"},
				{"name":"checksums.txt","browser_download_url":"https://example.test/checksums.txt"}
			]
		}`)
	}))
	defer server.Close()
	latestReleaseURL = server.URL

	latest, err := latestRelease(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if latest.Tag != "v2.0.0" {
		t.Fatalf("tag = %q", latest.Tag)
	}
	if len(latest.Assets) != 2 {
		t.Fatalf("assets = %#v", latest.Assets)
	}
	asset, ok := latest.asset("checksums.txt")
	if !ok || asset.URL != "https://example.test/checksums.txt" {
		t.Fatalf("checksums asset = %#v, ok = %t", asset, ok)
	}
}

func TestLatestReleaseReturnsRateLimitRecovery(t *testing.T) {
	preserveUpdateGlobals(t)
	reset := time.Now().Add(5 * time.Minute).Unix()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("X-RateLimit-Remaining", "0")
		response.Header().Set("X-RateLimit-Reset", fmt.Sprint(reset))
		response.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	latestReleaseURL = server.URL

	_, err := latestRelease(server.Client())
	if err == nil {
		t.Fatal("レート制限がエラーにならなかった")
	}
	if !strings.Contains(err.Error(), "5") {
		t.Fatalf("回復までの分数がない: %v", err)
	}
}

func TestVerifyChecksum(t *testing.T) {
	asset := "hso_v2_linux_amd64_ja.tar.gz"
	archive := []byte("verified archive")
	digest := sha256.Sum256(archive)
	checksums := []byte(fmt.Sprintf("%x  %s\n", digest, asset))

	if err := verifyChecksum(checksums, bytes.NewReader(archive), asset); err != nil {
		t.Fatalf("一致するチェックサム: %v", err)
	}
	if err := verifyChecksum(checksums, strings.NewReader("tampered"), asset); err == nil {
		t.Fatal("一致しないチェックサムが通った")
	}
}

func TestRunUpdateRejectsReleaseWithoutPlatformAsset(t *testing.T) {
	preserveUpdateGlobals(t)
	version = "v1.0.0"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		fmt.Fprint(response, `{
			"tag_name":"v2.0.0",
			"assets":[{"name":"checksums.txt","browser_download_url":"https://example.test/checksums.txt"}]
		}`)
	}))
	defer server.Close()
	latestReleaseURL = server.URL
	updateHTTPClient = server.Client()

	err := runUpdate(io.Discard)
	if err == nil {
		t.Fatal("対象資産がないリリースが通った")
	}
	wantAsset := fmt.Sprintf("hso_v2.0.0_linux_%s_%s.tar.gz", runtime.GOARCH, msg.Lang)
	if !strings.Contains(err.Error(), wantAsset) {
		t.Fatalf("error = %q に %q がない", err, wantAsset)
	}
	if requests != 1 {
		t.Fatalf("資産がないのにダウンロードした: requests = %d", requests)
	}
}

func TestRunUpdateDoesNothingWhenAlreadyLatest(t *testing.T) {
	preserveUpdateGlobals(t)
	version = "v2.0.0"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		fmt.Fprint(response, `{"tag_name":"v2.0.0","assets":[]}`)
	}))
	defer server.Close()
	latestReleaseURL = server.URL
	updateHTTPClient = server.Client()

	var output bytes.Buffer
	if err := runUpdate(&output); err != nil {
		t.Fatal(err)
	}
	if output.String() != msg.AlreadyLatest("v2.0.0")+"\n" {
		t.Fatalf("output = %q", output.String())
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestRunVersionSuggestsUpdateAndSurvivesNetworkFailure(t *testing.T) {
	preserveUpdateGlobals(t)
	version = "v1.0.0"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(response, `{"tag_name":"v2.0.0","assets":[]}`)
	}))
	latestReleaseURL = server.URL
	versionHTTPClient = server.Client()

	var output bytes.Buffer
	if err := runVersion(&output); err != nil {
		t.Fatal(err)
	}
	want := msg.VersionOutput(version, msg.Lang, runtime.GOARCH) + "\n" + msg.UpdateAvailable("v2.0.0") + "\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}

	server.Close()
	output.Reset()
	if err := runVersion(&output); err != nil {
		t.Fatalf("通信失敗で version が失敗した: %v", err)
	}
	want = msg.VersionOutput(version, msg.Lang, runtime.GOARCH) + "\n"
	if output.String() != want {
		t.Fatalf("通信失敗時 output = %q, want %q", output.String(), want)
	}
}

func TestRunUpdateDownloadsVerifiesAndReplacesLast(t *testing.T) {
	preserveUpdateGlobals(t)
	version = "v1.0.0"
	newBinary := []byte("new hso binary")
	member := fmt.Sprintf("hso_v2.0.0_linux_%s_%s/hso", runtime.GOARCH, msg.Lang)
	archive := makeArchive(t, member, newBinary)
	digest := sha256.Sum256(archive)
	assetName := fmt.Sprintf("hso_v2.0.0_linux_%s_%s.tar.gz", runtime.GOARCH, msg.Lang)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest":
			fmt.Fprintf(response, `{
				"tag_name":"v2.0.0",
				"assets":[
					{"name":%q,"browser_download_url":%q},
					{"name":"checksums.txt","browser_download_url":%q}
				]
			}`, assetName, server.URL+"/archive", server.URL+"/checksums")
		case "/archive":
			response.Write(archive)
		case "/checksums":
			fmt.Fprintf(response, "%x  %s\n", digest, assetName)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	latestReleaseURL = server.URL + "/latest"
	updateHTTPClient = server.Client()

	target := filepath.Join(t.TempDir(), "hso")
	if err := os.WriteFile(target, []byte("old hso binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	executablePath = func() (string, error) { return target, nil }

	var output bytes.Buffer
	if err := runUpdate(&output); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBinary) {
		t.Fatalf("binary = %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %04o", info.Mode().Perm())
	}
	if output.String() != msg.UpdateComplete("v2.0.0", target)+"\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestReplacementPrivilegeUsesSudoForProtectedDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "system-bin")
	if err := os.Mkdir(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(directory, 0o700) })

	commandDirectory := t.TempDir()
	sudo := filepath.Join(commandDirectory, "sudo")
	if err := os.WriteFile(sudo, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", commandDirectory)

	got, err := replacementPrivilege(filepath.Join(directory, "hso"))
	if err != nil {
		t.Fatal(err)
	}
	if got != sudo {
		t.Fatalf("privileged = %q, want %q", got, sudo)
	}
}

func TestPrivilegedReplacementRunsCreateCopyChmodMove(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "downloaded-hso")
	target := filepath.Join(directory, "hso")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(directory, "privileged-args")
	privileged := filepath.Join(directory, "sudo")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$HSO_TEST_PRIVILEGE_LOG\"\nexec \"$@\"\n"
	if err := os.WriteFile(privileged, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HSO_TEST_PRIVILEGE_LOG", logPath)

	if err := replaceExecutable(source, target, privileged); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("binary = %q", got)
	}
	arguments, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"mktemp \"$2/.$4-XXXXXX\"",
		"cp \"$1\" \"$staging\"",
		"chmod 0755 \"$staging\"",
		"mv -f \"$staging\" \"$3\"",
	} {
		if !strings.Contains(string(arguments), command) {
			t.Errorf("privileged command に %q がない: %s", command, arguments)
		}
	}
}

func TestReplaceExecutableDoesNotFollowStagingSymlink(t *testing.T) {
	for _, privileged := range []bool{false, true} {
		name := "unprivileged"
		if privileged {
			name = "privileged"
		}
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			source := filepath.Join(directory, "downloaded-hso")
			target := filepath.Join(directory, "hso")
			if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			stagingSymlinks := []string{".hso.new", ".hso-0", ".hso-000000"}
			for _, name := range stagingSymlinks {
				if err := os.Symlink(target, filepath.Join(directory, name)); err != nil {
					t.Fatal(err)
				}
			}

			privilegeCommand := ""
			if privileged {
				privilegeCommand = filepath.Join(directory, "sudo")
				if err := os.WriteFile(privilegeCommand, []byte("#!/bin/sh\nexec \"$@\"\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			if err := replaceExecutable(source, target, privilegeCommand); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "new" {
				t.Fatalf("binary = %q", got)
			}
			for _, name := range stagingSymlinks {
				info, err := os.Lstat(filepath.Join(directory, name))
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode()&os.ModeSymlink == 0 {
					t.Errorf("%s が symlink ではなくなった", name)
				}
			}
		})
	}
}

func makeArchive(t *testing.T, member string, body []byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: member,
		Mode: 0o755,
		Size: int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
