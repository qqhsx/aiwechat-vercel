package client

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// GetMoviesByKeyword searches for movies on a third-party website and returns a formatted list.
func GetMoviesByKeyword(keyword string) string {
	if keyword == "" {
		return "请输入关键词进行搜索。"
	}

	// Escape the keyword for URL
	encodedKeyword := url.PathEscape(keyword)
	searchURL := fmt.Sprintf("https://xykmovie.com/s/1/%s", encodedKeyword)

	// Create HTTP request with a User-Agent header
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return "搜索失败，请稍后再试。"
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error fetching URL: %v\n", err)
		return "搜索失败，请稍后再试。"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("请求失败: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		return "搜索失败，请稍后再试。"
	}

	// Use regex to find all matches of data-code
	re := regexp.MustCompile(`<a[^>]+class="copy"[^>]+data-code="([^"]+)"`)
	matches := re.FindAllStringSubmatch(string(body), -1)

	if len(matches) == 0 {
		return "未找到结果"
	}

	// Format the results
	var resultBuilder strings.Builder
	resultBuilder.WriteString(fmt.Sprintf("为您找到以下结果：\n\n"))
	for i, match := range matches {
		if i >= 5 {
			break
		}
		formattedResult := fmt.Sprintf("%d. %s", i+1, match[1])
		resultBuilder.WriteString(formattedResult)
		resultBuilder.WriteString("\n")
	}

	return resultBuilder.String()
}