package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/pwh-pwh/aiwechat-vercel/db"
)

const (
	GPT  = "gpt"
	ECHO = "echo"

	Bot_Type_Key     = "botType"
	Bot_Type_Echo    = "echo"
	Bot_Type_Gpt     = "gpt"
	Bot_Type_Spark   = "spark"
	Bot_Type_Qwen    = "qwen"
	Bot_Type_Gemini  = "gemini"
	Bot_Type_Keyword = "keyword"
	Bot_Type_Image   = "image" // 新增图床机器人类型
	AdminUsersKey    = "ADMIN_USERS"
	Bot_Type_Claude  = "claude"

	KeywordMatchModeKey = "KEYWORD_MATCH_MODE"
	MatchModePartial    = "partial"
	MatchModeFull       = "full"
)

// 新增图床机器人的命令
const Wx_Command_Image = "/image"

var (
	Cache sync.Map

	Support_Bots = []string{Bot_Type_Gpt, Bot_Type_Spark, Bot_Type_Qwen, Bot_Type_Gemini, Bot_Type_Claude, Bot_Type_Keyword, Bot_Type_Image}
)

func IsSupportPrompt(botType string) bool {
	return botType == Bot_Type_Gpt || botType == Bot_Type_Qwen || botType == Bot_Type_Spark || botType == Bot_Type_Claude
}

func CheckBotConfig(botType string) (actualotType string, err error) {
	if botType == "" {
		botType = GetBotType()
	}
	actualotType = botType
	switch actualotType {
	case Bot_Type_Gpt:
		err = CheckGptConfig()
	case Bot_Type_Spark:
		_, err = GetSparkConfig()
	case Bot_Type_Qwen:
		_, err = GetQwenConfig()
	case Bot_Type_Gemini:
		err = CheckGeminiConfig()
	case Bot_Type_Keyword:
		err = nil // 本地实现，不需要外部配置
	case Bot_Type_Image:
		err = nil // 本地实现，不需要外部配置
	case Bot_Type_Claude:
		err = CheckClaudeConfig()
	}
	return
}

func CheckAllBotConfig() (botType string, checkRes map[string]bool) {
	botType = GetBotType()
	checkRes = map[string]bool{
		Bot_Type_Echo:    true,
		Bot_Type_Gpt:     true,
		Bot_Type_Spark:   true,
		Bot_Type_Qwen:    true,
		Bot_Type_Gemini:  true,
		Bot_Type_Keyword: true, // 增加对关键词模式的检查
		Bot_Type_Image:   true, // 增加对图床模式的检查
		Bot_Type_Claude:  true,
	}

	err := CheckGptConfig()
	if err != nil {
		checkRes[Bot_Type_Gpt] = false
	}
	_, err = GetSparkConfig()
	if err != nil {
		checkRes[Bot_Type_Spark] = false
	}
	_, err = GetQwenConfig()
	if err != nil {
		checkRes[Bot_Type_Qwen] = false
	}
	err = CheckGeminiConfig()
	if err != nil {
		checkRes[Bot_Type_Gemini] = false
	}
	err = CheckClaudeConfig()
	if err != nil {
		checkRes[Bot_Type_Claude] = false
	}
	return
}

func CheckGptConfig() error {
	gptToken := GetGptToken()
	token := GetWxToken()
	botType := GetBotType()
	if token == "" {
		return errors.New("请配置微信TOKEN")
	}
	if gptToken == "" && botType == Bot_Type_Gpt {
		return errors.New("请配置ChatGPTToken")
	}
	return nil
}

func CheckGeminiConfig() error {
	key := GetGeminiKey()
	if key == "" {
		return errors.New("请配置geminiKey")
	}
	return nil
}

func CheckClaudeConfig() error {
	return ValidateClaudeConfig()
}

func GetBotType() string {
	botType := os.Getenv(Bot_Type_Key)
	if slices.Contains(Support_Bots, botType) {
		return botType
	} else {
		return Bot_Type_Echo
	}
}

func GetUserBotType(userId string) (bot string) {
	bot, err := db.GetValue(fmt.Sprintf("%v:%v", Bot_Type_Key, userId))
	if err != nil {
		bot = GetBotType()
	}
	if !slices.Contains(Support_Bots, bot) {
		bot = GetBotType()
	}
	return
}

func GetMaxTokens() int {
	// 不设置或者设置不合法，均返回0，模型将使用默认值或者不设置
	maxTokensStr := os.Getenv("maxOutput")
	maxTokens, err := strconv.Atoi(maxTokensStr)
	if err != nil {
		return 0
	}
	return maxTokens
}

func GetDefaultSystemPrompt() string {
	return os.Getenv("defaultSystemPrompt")
}

// GetAdminUsers 从环境变量中获取管理员用户列表
func GetAdminUsers() []string {
	adminUsers := os.Getenv(AdminUsersKey)
	if adminUsers == "" {
		return nil
	}
	return strings.Split(adminUsers, ",")
}

// GetKeywordMatchMode returns the keyword matching mode, defaults to "partial".
func GetKeywordMatchMode() string {
	mode := strings.ToLower(os.Getenv(KeywordMatchModeKey))
	if mode == MatchModeFull {
		return MatchModeFull
	}
	return MatchModePartial
}