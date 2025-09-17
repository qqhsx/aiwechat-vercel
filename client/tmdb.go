package client

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

const TMDB_API_URL = "https://api.themoviedb.org/3/movie/%s?api_key=%s&language=%s"

// TMDbNowPlayingResponse represents the overall API response
type TMDbNowPlayingResponse struct {
	Results []TMDbMovie `json:"results"`
}

// TMDbMovie represents a single movie result
type TMDbMovie struct {
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"`
}

// GetMoviesByCategory fetches a list of movies by category from TMDb
func GetMoviesByCategory(category string) (string, error) {
	apiKey := os.Getenv("TMDB_API_KEY")
	if apiKey == "" {
		return "TMDB_API_KEY环境变量未设置。", nil
	}

	// Map category to API endpoint
	var endpoint string
	var title string
	switch category {
	case "now_playing":
		endpoint = "now_playing"
		title = "正在上映的电影"
	case "popular":
		endpoint = "popular"
		title = "流行电影"
	case "top_rated":
		endpoint = "top_rated"
		title = "热门电影"
	case "upcoming":
		endpoint = "upcoming"
		title = "即将上映的电影"
	default:
		return "不支持的电影类别。", nil
	}

	// You can customize the language. For this example, let's use simplified Chinese.
	language := "zh-CN"

	// Create the full request URL
	url := fmt.Sprintf(TMDB_API_URL, endpoint, apiKey, language)
	
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("请求 TMDb API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("TMDb API 返回错误，状态码: %d，信息: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 TMDb API 响应失败: %w", err)
	}

	var moviesResp TMDbNowPlayingResponse
	err = sonic.Unmarshal(bodyBytes, &moviesResp)
	if err != nil {
		return "", fmt.Errorf("解析 TMDb API 响应失败: %w", err)
	}

	if len(moviesResp.Results) == 0 {
		return fmt.Sprintf("未找到%s。", title), nil
	}

	var resultBuilder strings.Builder
	resultBuilder.WriteString(fmt.Sprintf("%s：\n", title))
	
	// Limit to top 5 results to avoid long messages
	limit := 5
	if len(moviesResp.Results) < limit {
		limit = len(moviesResp.Results)
	}

	for i, movie := range moviesResp.Results[:limit] {
		// Parse and format the release date
		releaseDate, err := time.Parse("2006-01-02", movie.ReleaseDate)
		formattedDate := movie.ReleaseDate
		if err == nil {
			formattedDate = releaseDate.Format("2006年1月2日")
		}

		resultBuilder.WriteString(fmt.Sprintf("%d. %s（%s）\n", i+1, movie.Title, formattedDate))
	}

	return resultBuilder.String(), nil
}