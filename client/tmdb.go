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

const TMDB_API_URL = "https://api.themoviedb.org/3/movie/now_playing?api_key=%s&language=%s"

// TMDbNowPlayingResponse represents the overall API response
type TMDbNowPlayingResponse struct {
	Results []TMDbMovie `json:"results"`
}

// TMDbMovie represents a single movie result
type TMDbMovie struct {
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"`
}

// GetNowPlayingMovies fetches a list of movies currently in theaters from TMDb
func GetNowPlayingMovies() (string, error) {
	apiKey := os.Getenv("TMDB_API_KEY")
	if apiKey == "" {
		return "TMDB_API_KEY环境变量未设置。", nil
	}

	// You can customize the language. For this example, let's use simplified Chinese.
	language := "zh-CN"

	// Create the full request URL
	url := fmt.Sprintf(TMDB_API_URL, apiKey, language)
	
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
		return "未找到正在上映的电影。", nil
	}

	var resultBuilder strings.Builder
	resultBuilder.WriteString("正在上映的电影：\n")
	
	// Limit to top 20 results to avoid long messages
	limit := 20
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