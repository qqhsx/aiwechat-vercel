package chat

import (
	"fmt"
	"strings"

	"github.com/pwh-pwh/aiwechat-vercel/client"
	"github.com/pwh-pwh/aiwechat-vercel/config"
	"github.com/pwh-pwh/aiwechat-vercel/db"
	"github.com/silenceper/wechat/v2/officialaccount/message"
)

type KeywordChat struct {
	BaseChat
}

func (k *KeywordChat) Chat(userID string, msg string, imageURL ...string) string {
	// 1. 检查是否为指令，如果是则交给DoAction处理 (保留，确保 /ai, /help 等命令仍然有效)
	r, flag := DoAction(userID, msg)
	if flag {
		return r
	}

	// 2. 尝试匹配数据库中的关键词
	replies, err := db.GetKeywordReplies()
	if err != nil {
		return "获取关键词回复失败"
	}

	matchMode := config.GetKeywordMatchMode()

	for _, reply := range replies {
		if matchMode == config.MatchModeFull {
			if msg == reply.Keyword {
				// 匹配成功，返回关键词回复
				return k.processReply(userID, reply.Reply)
			}
		} else {
			if strings.Contains(msg, reply.Keyword) {
				// 匹配成功，返回关键词回复
				return k.processReply(userID, reply.Reply)
			}
		}
	}

	// 3. 关键词匹配失败，执行电影搜索（将用户输入视为电影名）
	movieTitle := msg
	
	// 优先检查 TMDb 是否有中国上映信息
	isReleased, err := client.CheckChinaRelease(movieTitle)
	if err != nil {
		// 如果检查 TMDb 发生错误，打印错误并回退到旧的搜索方式
		fmt.Printf("Error checking TMDb release: %v\n", err)
		return client.GetMoviesByKeyword(movieTitle)
	}
	
	if isReleased {
		return fmt.Sprintf("《%s》已在中国上映，请在正规渠道观看。", movieTitle)
	}

	// 如果没有中国上映信息，则使用旧的搜索方式
	return client.GetMoviesByKeyword(movieTitle)
}

func (k *KeywordChat) HandleMediaMsg(msg *message.MixMessage) string {
	if msg.MsgType == message.MsgTypeImage {
		return msg.PicURL
	}
	if msg.MsgType == message.MsgTypeEvent {
		// 将事件消息委托给通用的 SimpleChat 处理
		simpleChat := SimpleChat{}
		return simpleChat.HandleMediaMsg(msg)
	}
	return "关键词回复模式不支持处理多媒体消息"
}

// processReply handles dynamic keyword replies based on special markers.
func (k *KeywordChat) processReply(userID string, reply string) string {
	// Check for a special marker to trigger dynamic behavior
	switch reply {
	case "__NOW_PLAYING__":
		movies, err := client.GetMoviesByCategory("now_playing")
		if err != nil {
			return "获取正在上映电影列表失败：" + err.Error()
		}
		return movies
	case "__POPULAR__":
		movies, err := client.GetMoviesByCategory("popular")
		if err != nil {
			return "获取流行电影列表失败：" + err.Error()
		}
		return movies
	case "__TOP_RATED__":
		movies, err := client.GetMoviesByCategory("top_rated")
		if err != nil {
			return "获取热门电影列表失败：" + err.Error()
		}
		return movies
	case "__UPCOMING__":
		movies, err := client.GetMoviesByCategory("upcoming")
		if err != nil {
			return "获取即将上映电影列表失败：" + err.Error()
		}
		return movies
	}

	// For all other cases, return the static reply
	return reply
}