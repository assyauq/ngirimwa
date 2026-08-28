package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LinkPreview adalah hasil ringkas OpenGraph/fallback dari satu URL.
type LinkPreview struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
	Favicon     string `json:"favicon,omitempty"`
}

// linkPreviewCache membatasi fetch berulang URL yang sama (thread panjang,
// bubble lama yang masuk viewport). Entry kecil; LRU sederhana via umur.
var linkPreviewCache = struct {
	sync.Mutex
	items    map[string]linkPreviewEntry
	maxItems int
}{items: make(map[string]linkPreviewEntry), maxItems: 200}

type linkPreviewEntry struct {
	preview   LinkPreview
	fetchedAt time.Time
}

const (
	linkPreviewTTL      = 10 * time.Minute
	linkPreviewMaxBody  = 256 << 10 // 256 KB cukup untuk <head>
	linkPreviewRedirect = 5
)

var (
	ogTitleRe       = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']{1,300})["']`)
	ogTitleReAlt    = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']{1,300})["'][^>]+property=["']og:title["']`)
	ogDescRe        = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:description["'][^>]+content=["']([^"']{1,600})["']`)
	ogDescReAlt     = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']{1,600})["'][^>]+property=["']og:description["']`)
	ogImageRe       = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']{1,1000})["']`)
	ogImageReAlt    = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']{1,1000})["'][^>]+property=["']og:image["']`)
	ogSiteRe        = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:site_name["'][^>]+content=["']([^"']{1,200})["']`)
	ogSiteReAlt     = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']{1,200})["'][^>]+property=["']og:site_name["']`)
	faviconLinkRe   = regexp.MustCompile(`(?is)<link[^>]+rel=["'](?:shortcut )?icon["'][^>]+href=["']([^"']{1,1000})["']`)
	faviconLinkReAlt = regexp.MustCompile(`(?is)<link[^>]+href=["']([^"']{1,1000})["'][^>]+rel=["'](?:shortcut )?icon["']`)
)

// FetchLinkPreview mengambil OpenGraph satu URL dengan proteksi SSRF yang sama
// dengan enrichment AI. Kosong (bukan error) bila tidak ada data og/title.
func FetchLinkPreview(rawURL string) (LinkPreview, error) {
	trimmed := strings.TrimSpace(rawURL)
	if err := assertPublicHTTPURL(trimmed); err != nil {
		return LinkPreview{}, err
	}

	key := strings.ToLower(trimmed)
	linkPreviewCache.Lock()
	if e, ok := linkPreviewCache.items[key]; ok && time.Since(e.fetchedAt) < linkPreviewTTL {
		linkPreviewCache.Unlock()
		return e.preview, nil
	}
	linkPreviewCache.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= linkPreviewRedirect {
				return fmt.Errorf("terlalu banyak redirect")
			}
			return assertPublicHTTPURL(req.URL.String())
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trimmed, nil)
	if err != nil {
		return LinkPreview{}, err
	}
	req.Header.Set("User-Agent", "KirimwaBot/1.0 (+link-preview; customer-service)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return LinkPreview{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return LinkPreview{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "html") && !strings.Contains(ct, "text/plain") {
		return LinkPreview{}, fmt.Errorf("bukan HTML")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, linkPreviewMaxBody))
	if err != nil || len(body) == 0 {
		return LinkPreview{}, fmt.Errorf("body kosong")
	}

	final := trimmed
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	base, _ := neturl.Parse(final)

	title := firstMatch(body, ogTitleRe, ogTitleReAlt)
	if title == "" {
		if m := htmlTitleRe.FindSubmatch(body); len(m) == 2 {
			title = cleanHTMLText(string(m[1]))
		}
	}
	desc := firstMatch(body, ogDescRe, ogDescReAlt)
	image := resolveRelative(base, firstMatch(body, ogImageRe, ogImageReAlt))
	site := firstMatch(body, ogSiteRe, ogSiteReAlt)
	favicon := resolveRelative(base, firstMatch(body, faviconLinkRe, faviconLinkReAlt))

	preview := LinkPreview{
		URL:         final,
		Title:       cleanHTMLText(title),
		Description: cleanHTMLText(desc),
		Image:       image,
		SiteName:    cleanHTMLText(site),
		Favicon:     favicon,
	}
	if preview.Title == "" && preview.Image == "" {
		return LinkPreview{}, fmt.Errorf("tidak ada data preview")
	}

	linkPreviewCache.Lock()
	if len(linkPreviewCache.items) >= linkPreviewCache.maxItems {
		// Buang setengah entri tertua bila penuh.
		now := time.Now()
		for k, e := range linkPreviewCache.items {
			if now.Sub(e.fetchedAt) > linkPreviewTTL/2 {
				delete(linkPreviewCache.items, k)
			}
		}
	}
	linkPreviewCache.items[key] = linkPreviewEntry{preview: preview, fetchedAt: time.Now()}
	linkPreviewCache.Unlock()

	return preview, nil
}

func firstMatch(body []byte, res ...*regexp.Regexp) string {
	for _, re := range res {
		if m := re.FindSubmatch(body); len(m) == 2 {
			if s := strings.TrimSpace(string(m[1])); s != "" {
				return s
			}
		}
	}
	return ""
}

// resolveRelative mengubah URL relatif (mis. "/img/cover.jpg") menjadi absolut
// terhadap base; mengembalikan string kosong bila tidak valid.
func resolveRelative(base *neturl.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || base == nil {
		return ""
	}
	u, err := neturl.Parse(raw)
	if err != nil {
		return ""
	}
	if u.IsAbs() {
		return u.String()
	}
	return base.ResolveReference(u).String()
}

// ClearLinkPreviewCacheForTest membersihkan cache (dipakai test regresi).
func ClearLinkPreviewCacheForTest() {
	linkPreviewCache.Lock()
	linkPreviewCache.items = make(map[string]linkPreviewEntry)
	linkPreviewCache.Unlock()
}
