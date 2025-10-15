package client

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/bytedance/sonic"
)

const (
	tmdbAPIURL = "https://api.themoviedb.org/3"
	tmdbApiKey = "TMDB_API_KEY"
)

// TMDbSearchResponse represents the response from the movie search API.
type TMDbSearchResponse struct {
	Results []TMDbMovieResult `json:"results"`
}

// TMDbMovieResult represents a single movie result from the search.
type TMDbMovieResult struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// TMDbReleaseDatesResponse represents the response from the release dates API.
type TMDbReleaseDatesResponse struct {
	Results []TMDbCountryRelease `json:"results"`
}

// TMDbCountryRelease represents the release information for a specific country.
type TMDbCountryRelease struct {
	ISO31661 string `json:"iso_3166_1"`
}

// searchMovieID searches for a movie by title and returns its TMDb ID.
func searchMovieID(title string) (int, error) {
	apiKey := os.Getenv(tmdbApiKey)
	if apiKey == "" {
		return 0, fmt.Errorf("TMDB_API_KEY 环境变量未设置")
	}

	encodedTitle := url.QueryEscape(title)
	url := fmt.Sprintf("%s/search/movie?api_key=%s&query=%s", tmdbAPIURL, apiKey, encodedTitle)

	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("请求 TMDb 搜索 API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("TMDb API 返回错误，状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("读取 TMDb 搜索响应失败: %w", err)
	}

	var searchResp TMDbSearchResponse
	if err := sonic.Unmarshal(body, &searchResp); err != nil {
		return 0, fmt.Errorf("解析 TMDb 搜索响应失败: %w", err)
	}

	if len(searchResp.Results) == 0 {
		return 0, nil
	}
	
	// 返回第一个匹配结果的ID
	return searchResp.Results[0].ID, nil
}

// CheckChinaRelease checks if a movie has a release date in China.
func CheckChinaRelease(movieTitle string) (bool, error) {
	movieID, err := searchMovieID(movieTitle)
	if err != nil {
		return false, err
	}
	if movieID == 0 {
		return false, nil // 找不到电影，自然也找不到在中国上映的信息
	}

	apiKey := os.Getenv(tmdbApiKey)
	if apiKey == "" {
		return false, fmt.Errorf("TMDB_API_KEY 环境变量未设置")
	}

	url := fmt.Sprintf("%s/movie/%d/release_dates?api_key=%s", tmdbAPIURL, movieID, apiKey)

	resp, err := http.Get(url)
	if err != nil {
		return false, fmt.Errorf("请求 TMDb 发布日期 API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("TMDb API 返回错误，状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取 TMDb 发布日期响应失败: %w", err)
	}

	var releaseDatesResp TMDbReleaseDatesResponse
	if err := sonic.Unmarshal(body, &releaseDatesResp); err != nil {
		return false, fmt.Errorf("解析 TMDb 发布日期响应失败: %w", err)
	}

	// 检查结果中是否有中国区的发布信息
	for _, result := range releaseDatesResp.Results {
		if strings.ToUpper(result.ISO31661) == "CN" {
			return true, nil
		}
	}

	return false, nil
}