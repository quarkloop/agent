package iofetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultMaxBytes     = 1 << 20
	DefaultTimeout      = 30 * time.Second
	DefaultMaxRedirects = 5
)

type Result struct {
	URL         string
	StatusCode  int32
	ContentType string
	Body        string
	Truncated   bool
	BodyBytes   int32
	Error       string
}

func Fetch(ctx context.Context, rawURL, method string, maxBytes, timeoutSeconds, maxRedirects int32) Result {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return Result{Error: "url is required"}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Result{URL: rawURL, Error: fmt.Sprintf("invalid url: %v", err)}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Result{URL: rawURL, Error: "only http and https URLs are allowed"}
	}
	if err := blockPrivateHost(parsed.Hostname()); err != nil {
		return Result{URL: rawURL, Error: err.Error()}
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return Result{URL: rawURL, Error: "only GET and HEAD are allowed"}
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	timeout := DefaultTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	if maxRedirects <= 0 {
		maxRedirects = DefaultMaxRedirects
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= int(maxRedirects) {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if err := blockPrivateHost(req.URL.Hostname()); err != nil {
				return err
			}
			return nil
		},
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, rawURL, nil)
	if err != nil {
		return Result{URL: rawURL, Error: err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{URL: rawURL, Error: err.Error()}
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	var body []byte
	if method != http.MethodHead {
		limited := io.LimitReader(resp.Body, int64(maxBytes)+1)
		body, err = io.ReadAll(limited)
		if err != nil {
			return Result{URL: rawURL, StatusCode: int32(resp.StatusCode), ContentType: contentType, Error: err.Error()}
		}
	}
	truncated := int32(len(body)) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}
	text := ""
	if len(body) > 0 && utf8.Valid(body) {
		text = string(body)
	}
	return Result{
		URL:         rawURL,
		StatusCode:  int32(resp.StatusCode),
		ContentType: contentType,
		Body:        text,
		Truncated:   truncated,
		BodyBytes:   int32(len(body)),
	}
}

func blockPrivateHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return errors.New("host is required")
	}
	if strings.EqualFold(host, "localhost") {
		return errors.New("localhost is not allowed")
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("resolve host: %w", err)
		}
		for _, resolved := range ips {
			if isPrivateIP(resolved) {
				return errors.New("private network addresses are not allowed")
			}
		}
		return nil
	}
	if isPrivateIP(ip) {
		return errors.New("private network addresses are not allowed")
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"0.0.0.0/8",
	}
	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
