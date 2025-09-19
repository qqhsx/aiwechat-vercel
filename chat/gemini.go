package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/pwh-pwh/aiwechat-vercel/config"
	"github.com/pwh-pwh/aiwechat-vercel/db"
	"github.com/silenceper/wechat/v2/officialaccount/message"
	"google.golang.org/api/option"
)

const (
	GeminiUser = "user"
	GeminiBot  = "model"
)

type GeminiChat struct {
	BaseChat
	key       string
	maxTokens int
}

func (s *GeminiChat) toDbMsg(msg *genai.Content) db.Msg {
	// 目前只将文本内容存入数据库
	var textContent string
	for _, part := range msg.Parts {
		if text, ok := part.(genai.Text); ok {
			textContent += string(text)
		}
	}
	return db.Msg{
		Role: msg.Role,
		Msg:  textContent,
	}
}

func (s *GeminiChat) toChatMsg(msg db.Msg) *genai.Content {
	return &genai.Content{Parts: []genai.Part{genai.Text(msg.Msg)}, Role: msg.Role}
}

func (s *GeminiChat) getModel(userId string) string {
	if model, err := db.GetModel(userId, config.Bot_Type_Gemini); err == nil && model != "" {
		return model
	}
	return "gemini-2.0-flash"
}

func (s *GeminiChat) HandleMediaMsg(msg *message.MixMessage) string {
	return WithTimeChat(string(msg.FromUserName), msg.MsgId, func(userId, msgId string) string {
		var parts []genai.Part
		// 优先处理图片
		if msg.PicURL != "" {
			parts = append(parts, genai.FileURI(msg.PicURL))
		}
		// 处理文本消息（如果图片消息中包含文字描述）
		if msg.Content != "" {
			parts = append(parts, genai.Text(msg.Content))
		}
		if len(parts) == 0 {
			return "无法处理空消息"
		}
		return s.chatWithParts(string(msg.FromUserName), parts)
	})
}

func (s *GeminiChat) Chat(userId string, msg string) string {
	r, flag := DoAction(userId, msg)
	if flag {
		return r
	}
	return WithTimeChat(userId, msg, func(userId, msg string) string {
		return s.chatWithParts(userId, []genai.Part{genai.Text(msg)})
	})
}

func (s *GeminiChat) chatWithParts(userId string, parts []genai.Part) string {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.key))
	if err != nil {
		return err.Error()
	}
	defer client.Close()
	model := client.GenerativeModel(s.getModel(userId))
	if s.maxTokens > 0 {
		model.SetMaxOutputTokens(int32(s.maxTokens))
	}
	// 初始化聊天会话
	cs := model.StartChat()
	
	// 从数据库加载历史消息，并将其转换为 genai.Content 格式
	var history []*genai.Content
	if db.ChatDbInstance != nil {
		dbMsgs, err := db.ChatDbInstance.GetMsgList(config.Bot_Type_Gemini, userId)
		if err == nil {
			for _, dbMsg := range dbMsgs {
				// 跳过没有内容的系统消息或空消息
				if strings.TrimSpace(dbMsg.Msg) != "" {
					history = append(history, s.toChatMsg(dbMsg))
				}
			}
		}
	}
	cs.History = history
	
	// 发送消息
	resp, err := cs.SendMessage(ctx, parts...)
	if err != nil {
		return err.Error()
	}
	
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "没有收到有效的回复"
	}
	
	textPart, ok := resp.Candidates[0].Content.Parts[0].(genai.Text)
	if !ok {
		return "收到了非文本格式的回复"
	}
	
	responseText := string(textPart)
	
	// 组装新的消息列表并保存到数据库
	newHistory := append(history, &genai.Content{
		Parts: parts,
		Role: GeminiUser,
	})
	newHistory = append(newHistory, &genai.Content{
		Parts: []genai.Part{genai.Text(responseText)},
		Role: GeminiBot,
	})
	
	if db.ChatDbInstance != nil {
		go func() {
			var dbList []db.Msg
			for _, msg := range newHistory {
				dbList = append(dbList, s.toDbMsg(msg))
			}
			db.ChatDbInstance.SetMsgList(config.Bot_Type_Gemini, userId, dbList)
		}()
	}
	
	return responseText
}