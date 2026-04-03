package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	readability "codeberg.org/readeck/go-readability/v2"
)

// SSRF protection: deny private and link-local IP ranges.
var denylistedCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12",
		"192.168.0.0/16", "169.254.0.0/16", "::1/128", "fc00::/7",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, ipNet, _ := net.ParseCIDR(c)
		nets = append(nets, ipNet)
	}
	return nets
}()

func isPrivateIP(ip net.IP) bool {
	for _, cidr := range denylistedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func validateHost(hostname string) error {
	host := hostname
	if h, _, err := net.SplitHostPort(hostname); err == nil {
		host = h
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")

	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("access to private IP address is not allowed")
		}
		return nil
	}

	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("could not resolve host: %w", err)
	}
	for _, addr := range addrs {
		if ip := net.ParseIP(addr); ip != nil && isPrivateIP(ip) {
			return fmt.Errorf("access to private IP address is not allowed")
		}
	}
	return nil
}

const (
	fetchTimeout     = 10 * time.Second
	fetchMaxBody     = 2 << 20 // 2MB
	extractionMaxLen = 8000
	fetchUserAgent   = "imago/1.0"
	fetchDelay       = 1 * time.Second
)

// pageFetcher handles fetching web pages and extracting readable content.
type pageFetcher struct {
	client    *http.Client
	mu        sync.Mutex
	lastFetch time.Time
}

type pageFetcherOption func(*pageFetcher)

func withHTTPClient(c *http.Client) pageFetcherOption {
	return func(pf *pageFetcher) { pf.client = c }
}

func newPageFetcher(opts ...pageFetcherOption) *pageFetcher {
	pf := &pageFetcher{
		client: &http.Client{Timeout: fetchTimeout},
	}
	for _, opt := range opts {
		opt(pf)
	}
	return pf
}

func (f *pageFetcher) fetchAndExtract(ctx context.Context, rawURL string) (string, error) {
	f.mu.Lock()
	if !f.lastFetch.IsZero() {
		if elapsed := time.Since(f.lastFetch); elapsed < fetchDelay {
			f.mu.Unlock()
			time.Sleep(fetchDelay - elapsed)
			f.mu.Lock()
		}
	}
	f.lastFetch = time.Now()
	f.mu.Unlock()

	body, err := f.fetchPage(ctx, rawURL)
	if err != nil {
		return "", err
	}

	text, err := extractReadableText(rawURL, body)
	if err != nil || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("could not extract readable content from this page")
	}

	if len(text) > extractionMaxLen {
		text = text[:extractionMaxLen]
		for len(text) > 0 && !utf8.Valid([]byte(text)) {
			text = text[:len(text)-1]
		}
	}

	return text, nil
}

func (f *pageFetcher) fetchPage(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %s", rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme: %s", parsed.Scheme)
	}
	if err := validateHost(parsed.Host); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("could not fetch page: %v", err)
	}
	req.Header.Set("User-Agent", fetchUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not fetch page: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not fetch page: HTTP %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml") {
		return "", fmt.Errorf("URL does not point to a web page (content-type: %s)", ct)
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBody))
	if err != nil {
		return "", fmt.Errorf("could not read page: %v", err)
	}
	return string(respBody), nil
}

func extractReadableText(pageURL, htmlStr string) (string, error) {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	article, err := readability.FromReader(strings.NewReader(htmlStr), parsed)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := article.RenderText(&buf); err != nil {
		return "", fmt.Errorf("no readable content extracted")
	}
	text := buf.String()
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("no readable content extracted")
	}
	return text, nil
}

// searxngClient talks to a SearXNG instance.
type searxngClient struct {
	baseURL    string
	httpClient *http.Client
}

func newSearXNGClient(baseURL string) *searxngClient {
	return &searxngClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

type searxngResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

func (c *searxngClient) search(ctx context.Context, query string, limit int) ([]searchResult, error) {
	u, err := url.Parse(strings.TrimRight(c.baseURL, "/") + "/search")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("categories", "general")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng returned %d", resp.StatusCode)
	}

	var sr searxngResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	results := make([]searchResult, len(sr.Results))
	for i, r := range sr.Results {
		results[i] = searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
