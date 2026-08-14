package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

var latestReleaseURL = "https://api.github.com/repos/hijoushoku7/hijo-server-ops/releases/latest"

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	Tag    string         `json:"tag_name"`
	Assets []releaseAsset `json:"assets"`
}

// latestRelease は最新タグと資産一覧を取得する唯一の入口。
// 取得方式を変える場合は、この関数だけを差し替える。
func latestRelease(client *http.Client) (release, error) {
	request, err := githubRequest(latestReleaseURL)
	if err != nil {
		return release{}, msg.LatestReleaseRequestFailed(err)
	}

	response, err := client.Do(request)
	if err != nil {
		return release{}, msg.LatestReleaseRequestFailed(err)
	}
	defer response.Body.Close()

	if minutes, limited := rateLimitWait(response, time.Now()); limited {
		return release{}, msg.GitHubRateLimited(minutes)
	}
	if response.StatusCode != http.StatusOK {
		return release{}, msg.LatestReleaseStatus(response.Status)
	}

	var latest release
	if err := json.NewDecoder(response.Body).Decode(&latest); err != nil {
		return release{}, msg.LatestReleaseDecodeFailed(err)
	}
	if !validReleaseTag(latest.Tag) {
		return release{}, msg.InvalidReleaseTag(latest.Tag)
	}
	return latest, nil
}

func githubRequest(url string) (*http.Request, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "hso/"+version)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request, nil
}

func rateLimitWait(response *http.Response, now time.Time) (int, bool) {
	if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusTooManyRequests {
		return 0, false
	}
	if response.Header.Get("X-RateLimit-Remaining") != "0" {
		return 0, false
	}

	resetUnix, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		return 0, false
	}
	remaining := time.Unix(resetUnix, 0).Sub(now)
	if remaining <= 0 {
		return 0, true
	}
	return int((remaining + time.Minute - 1) / time.Minute), true
}

func validReleaseTag(tag string) bool {
	if tag == "" {
		return false
	}
	for i := 0; i < len(tag); i++ {
		c := tag[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

func (r release) asset(name string) (releaseAsset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return releaseAsset{}, false
}
