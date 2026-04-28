package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	defaultTopN    = 20
	defaultBaseURL = "https://catnose.me/lab/hackernews-ja"
)

type NewsItem struct {
	TitleJa            string `json:"titleJa"`
	Score              int    `json:"score"`
	CommentSummaryHtml string `json:"commentSummaryHtml"`
}

func main() {
	jst := time.FixedZone("JST", 9*60*60)
	date := time.Now().In(jst).Format("2006-01-02")
	if envDate := os.Getenv("PODCAST_DATE"); envDate != "" {
		date = envDate
	}
	if len(os.Args) > 1 {
		date = os.Args[1]
	}

	dataURL := fmt.Sprintf("%s/%s.txt", defaultBaseURL, date)

	log.Printf("Fetching data from: %s", dataURL)

	items, err := fetchAndParseNews(dataURL)
	if err != nil {
		log.Fatalf("Failed to fetch news: %v", err)
	}

	log.Printf("Found %d news items", len(items))

	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})

	topItems := items
	if len(topItems) > defaultTopN {
		topItems = topItems[:defaultTopN]
	}

	if err := generatePodcast(date, topItems); err != nil {
		log.Fatalf("Podcast generation failed: %v", err)
	}

	log.Println("Successfully generated podcast!")
}

func fetchAndParseNews(url string) ([]NewsItem, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return parseRSCData(body)
}

func parseRSCData(data []byte) ([]NewsItem, error) {
	str := string(data)
	var items []NewsItem

	searchStr := `"item":`
	idx := 0
	for {
		pos := strings.Index(str[idx:], searchStr)
		if pos == -1 {
			break
		}
		pos += idx + len(searchStr)

		for pos < len(str) && (str[pos] == ' ' || str[pos] == '\t') {
			pos++
		}

		if pos >= len(str) || str[pos] != '{' {
			idx = pos
			continue
		}

		jsonStr := extractJSON(str[pos:])
		if jsonStr == "" {
			idx = pos + 1
			continue
		}

		var item NewsItem
		if err := json.Unmarshal([]byte(jsonStr), &item); err != nil {
			idx = pos + 1
			continue
		}

		if item.TitleJa != "" {
			items = append(items, item)
		}

		idx = pos + len(jsonStr)
	}

	return items, nil
}

func extractJSON(s string) string {
	if len(s) == 0 || s[0] != '{' {
		return ""
	}

	depth := 0
	inString := false
	escaped := false

	for i, c := range s {
		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}

	return ""
}
