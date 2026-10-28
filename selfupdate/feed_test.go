package selfupdate

import (
	"net/url"
	"strings"
	"testing"
)

const sampleFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Release notes from ARTEX</title>
  <entry>
    <title>v0.3.16-rc1</title>
    <updated>2026-10-28T10:00:00Z</updated>
    <link rel="alternate" href="https://github.com/Autumn-27/artex/releases/tag/v0.3.16-rc1"/>
  </entry>
  <entry>
    <title>v0.3.15</title>
    <updated>2026-10-07T09:00:00Z</updated>
    <link rel="alternate" href="https://github.com/Autumn-27/artex/releases/tag/v0.3.15"/>
  </entry>
  <entry>
    <title>v0.3.14</title>
    <updated>2026-09-20T09:00:00Z</updated>
  </entry>
</feed>`

func TestParseReleaseFeedTitles(t *testing.T) {
	tags, err := parseReleaseFeedTitles(strings.NewReader(sampleFeed))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	want := []string{"v0.3.16-rc1", "v0.3.15", "v0.3.14"}
	if len(tags) != len(want) {
		t.Fatalf("标签数应为 %d，实际 %v", len(want), tags)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Errorf("第 %d 个标签 = %q，期望 %q", i, tags[i], want[i])
		}
	}
}

func TestParseReleaseFeedTitlesRejectsGarbage(t *testing.T) {
	if _, err := parseReleaseFeedTitles(strings.NewReader("not xml at all")); err == nil {
		t.Fatal("非 XML 应报错")
	}
}

// atom 不区分 prerelease，必须靠标签名过滤：否则会把 -rc 测试版推给用户。
func TestPickStableTagSkipsPrerelease(t *testing.T) {
	got, ok := pickStableTag([]string{"v0.4.0-rc1", "v0.4.0-beta.2", "v0.3.16", "v0.3.15"})
	if !ok || got != "v0.3.16" {
		t.Fatalf("应跳过预发布选到 v0.3.16，实际 %q ok=%v", got, ok)
	}

	if _, ok := pickStableTag([]string{"v1.0.0-rc1", "v1.0.0-alpha"}); ok {
		t.Fatal("全是预发布时应返回 false，而不是硬推一个测试版")
	}
	if _, ok := pickStableTag(nil); ok {
		t.Fatal("空列表应返回 false")
	}
}

func TestIsPrereleaseTag(t *testing.T) {
	pre := []string{"v0.3.16-rc1", "v1.0.0-BETA", "0.9-alpha", "v2.0.0-pre1", "v1.0.0-dev", "v1.0.0-SNAPSHOT", "v1.0.0-nightly", ""}
	for _, tag := range pre {
		if !isPrereleaseTag(tag) {
			t.Errorf("%q 应判为预发布", tag)
		}
	}
	stable := []string{"v0.3.15", "0.3.15", "v1.0.0", "release-2026"}
	for _, tag := range stable {
		if isPrereleaseTag(tag) {
			t.Errorf("%q 不该判为预发布", tag)
		}
	}
}

// 回退路径拼出来的资产必须能被 stage.go 原样消费：名字与 build.sh 约定一致，
// URL 落在白名单域名上，SHA256SUMS 必须在列。
func TestReleaseFromTagBuildsExpectedAssets(t *testing.T) {
	rel := releaseFromTag("v0.3.15", "linux", "amd64")
	if rel.TagName != "v0.3.15" {
		t.Fatalf("TagName = %q", rel.TagName)
	}

	zipName := AssetName("v0.3.15", "linux", "amd64")
	if zipName != "artex-0.3.15-linux-amd64.zip" {
		t.Fatalf("资产名与发布约定不一致: %q", zipName)
	}
	asset, ok := rel.FindAsset(zipName)
	if !ok {
		t.Fatalf("缺少当前平台资产 %s，实际 %+v", zipName, rel.Assets)
	}
	wantURL := "https://github.com/" + Repo + "/releases/download/v0.3.15/" + zipName
	if asset.URL != wantURL {
		t.Errorf("下载地址 = %q，期望 %q", asset.URL, wantURL)
	}
	if _, ok := rel.FindAsset(sumsAsset); !ok {
		t.Fatalf("缺少 %s，stage 会拒绝安装未经校验的二进制", sumsAsset)
	}

	// 所有拼出来的 URL 都必须在白名单域名内，否则 checkURL 会拦下（这里提前暴露）。
	for _, a := range rel.Assets {
		if !strings.HasPrefix(a.URL, "https://github.com/") {
			t.Errorf("资产 %s 的地址不在白名单域: %s", a.Name, a.URL)
		}
	}
}

// feed 地址本身也必须在白名单里，否则回退永远失败。
func TestFeedURLIsAllowed(t *testing.T) {
	u, err := url.Parse(feedURL)
	if err != nil {
		t.Fatalf("feedURL 解析失败: %v", err)
	}
	if err := checkURL(u); err != nil {
		t.Fatalf("feedURL 不在允许域名内: %v", err)
	}
}
