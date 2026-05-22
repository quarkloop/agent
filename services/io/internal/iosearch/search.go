package iosearch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type Result struct {
	Title   string
	URL     string
	Snippet string
}

func Search(query string, maxResults int) ([]Result, string, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, "", fmt.Errorf("query is required")
	}
	if key := os.Getenv("BRAVE_API_KEY"); key != "" {
		results, err := searchBrave(query, maxResults, key)
		return results, query, err
	}
	if key := os.Getenv("SERPAPI_KEY"); key != "" {
		results, err := searchSerpAPI(query, maxResults, key)
		return results, query, err
	}
	return []Result{{
		Title:   fmt.Sprintf("No search provider configured for: %s", query),
		URL:     "https://example.com",
		Snippet: "Set BRAVE_API_KEY or SERPAPI_KEY to enable real search results.",
	}}, query, nil
}

func searchBrave(query string, maxResults int, apiKey string) ([]Result, error) {
	reqURL := fmt.Sprintf(
		"https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), maxResults,
	)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("brave search: read body: %w", err)
	}

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("brave search: parse: %w", err)
	}

	out := make([]Result, 0, len(result.Web.Results))
	for _, r := range result.Web.Results {
		out = append(out, Result{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return out, nil
}

func searchSerpAPI(query string, maxResults int, apiKey string) ([]Result, error) {
	reqURL := fmt.Sprintf(
		"https://serpapi.com/search.json?q=%s&num=%d&api_key=%s",
		url.QueryEscape(query), maxResults, apiKey,
	)
	resp, err := http.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("serpapi: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("serpapi: read body: %w", err)
	}

	var result struct {
		OrganicResults []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("serpapi: parse: %w", err)
	}

	out := make([]Result, 0, len(result.OrganicResults))
	for _, r := range result.OrganicResults {
		out = append(out, Result{Title: r.Title, URL: r.Link, Snippet: r.Snippet})
	}
	return out, nil
}
