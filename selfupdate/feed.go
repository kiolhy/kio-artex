package selfupdate

// feed.go —— GitHub API 限流时的 releases.atom 回退（来源：二开 forxiucn 4f43953）。
//
// 问题：未认证的 GitHub API 是每 IP 每小时 60 次。共用出口 IP（公司网关、云主机、
// 家里的 CGNAT）时很容易撞上 403/429，而"一键更新"就彻底不可用了——明明
// releases 页面还在，只是接口不给了。
//
// 做法：限流时改读 releases.atom（网页 RSS，不吃 API 配额）。它只给标签名，不给资产
// 列表，所以资产 URL 按发布约定自己拼：仓库 CI 一直用
//   artex-<版本>-<os>-<arch>.zip 与 SHA256SUMS
// 这个名字，下载与校验链路（stage.go）完全不用改，SHA256SUMS 比对照旧是第二道闸门。
//
// 一个坑：atom 里**不区分 prerelease**（draft 不出现，预发布会出现）。所以按标签名
// 过滤 -rc / -beta / -alpha / -pre / -dev 这类后缀，避免把测试版当成正式版推给用户。

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
)

// feedURL 是仓库的 releases RSS。走 github.com 域，在 allowedHosts 白名单内。
const feedURL = "https://github.com/" + Repo + "/releases.atom"

// feedEntry 是 atom 里我们关心的字段。
type feedEntry struct {
	Title   string `xml:"title"`
	Updated string `xml:"updated"`
	Link    struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
}

type feedDoc struct {
	Entries []feedEntry `xml:"entry"`
}

// parseReleaseFeedTitles 解析 atom，按发布顺序（新→旧）返回标签名。
func parseReleaseFeedTitles(r io.Reader) ([]string, error) {
	var doc feedDoc
	dec := xml.NewDecoder(io.LimitReader(r, 1<<20))
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("解析 releases.atom 失败: %w", err)
	}
	out := make([]string, 0, len(doc.Entries))
	for _, e := range doc.Entries {
		t := strings.TrimSpace(e.Title)
		if t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// prereleaseMarkers 是标签名里出现即视为预发布的片段。
var prereleaseMarkers = []string{"-rc", "-beta", "-alpha", "-pre", "-dev", "-snapshot", "-nightly", "-test"}

// isPrereleaseTag 判断标签名是否像预发布版。
func isPrereleaseTag(tag string) bool {
	t := strings.ToLower(strings.TrimSpace(tag))
	if t == "" {
		return true
	}
	for _, m := range prereleaseMarkers {
		if strings.Contains(t, m) {
			return true
		}
	}
	return false
}

// pickStableTag 从新到旧挑第一个非预发布标签。
func pickStableTag(tags []string) (string, bool) {
	for _, t := range tags {
		if !isPrereleaseTag(t) {
			return t, true
		}
	}
	return "", false
}

// releaseFromTag 按发布约定拼出该标签下当前平台需要的资产。
// 只拼当前平台 + SHA256SUMS：自更新永远只为自己这台机器下载。
func releaseFromTag(tag, goos, goarch string) *Release {
	base := "https://github.com/" + Repo + "/releases/download/" + tag + "/"
	zipName := AssetName(tag, goos, goarch)
	return &Release{
		TagName: tag,
		Name:    tag,
		HTMLURL: "https://github.com/" + Repo + "/releases/tag/" + tag,
		Assets: []Asset{
			{Name: zipName, URL: base + zipName},
			{Name: sumsAsset, URL: base + sumsAsset},
		},
	}
}

// fetchLatestFromFeed 在 API 限流时用 releases.atom 兜底。
func fetchLatestFromFeed(ctx context.Context, c *http.Client) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	if err := checkURL(req.URL); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/atom+xml, application/xml;q=0.9")
	req.Header.Set("User-Agent", "artex-selfupdate")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("访问 releases.atom 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("releases.atom 返回 %d", resp.StatusCode)
	}
	tags, err := parseReleaseFeedTitles(resp.Body)
	if err != nil {
		return nil, err
	}
	tag, ok := pickStableTag(tags)
	if !ok {
		return nil, fmt.Errorf("releases.atom 里没有可用的正式版本标签")
	}
	return releaseFromTag(tag, runtime.GOOS, runtime.GOARCH), nil
}
