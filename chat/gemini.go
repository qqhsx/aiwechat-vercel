package chat

import (
	"context"
	"fmt"
	"strconv"

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
	// 将 genai.Content 转换为新的 db.Msg 结构
	var parts []db.ContentPart
	for _, part := range msg.Parts {
		switch v := part.(type) {
		case genai.Text:
			parts = append(parts, db.ContentPart{Type: "text", Data: string(v)})
		case *genai.FileData:
			parts = append(parts, db.ContentPart{Type: "image", Data: v.URI, MIMEType: v.MIMEType})
		}
	}
	return db.Msg{
		Role:  msg.Role,
		Parts: parts,
	}
}

func (s *GeminiChat) toChatMsg(msg db.Msg) *genai.Content {
	// 将新的 db.Msg 结构转换回 genai.Content
	var parts []genai.Part
	for _, part := range msg.Parts {
		if part.Type == "text" {
			parts = append(parts, genai.Text(part.Data))
		}
	}
	return &genai.Content{Parts: parts, Role: msg.Role}
}

func (s *GeminiChat) getModel(userId string) string {
	if model, err := db.GetModel(userId, config.Bot_Type_Gemini); err == nil && model != "" {
		return model
	}
	return "gemini-2.0-flash"
}

// HandleMediaMsg 处理所有多媒体消息（图片、语音等）
func (s *GeminiChat) HandleMediaMsg(msg *message.MixMessage) string {
	// 此方法在新的架构中已不再直接被调用，但为了满足接口定义仍需保留
	// 实际的多媒体消息处理逻辑已迁移至 api/wx.go 和 Chat 方法中
	simpleChat := SimpleChat{}
	return simpleChat.HandleMediaMsg(msg)
}

// Chat 处理纯文本和多媒体消息
func (s *GeminiChat) Chat(userId string, msg string, imageURL ...string) string {
	r, flag := DoAction(userId, msg)
	if flag {
		return r
	}
	return WithTimeChat(userId, msg, func(userId, msg string) string {
		var parts []genai.Part
		// 添加文本部分
		if msg != "" {
			parts = append(parts, genai.Text(msg))
		}
		// 添加图片部分（如果存在）
		if len(imageURL) > 0 && imageURL[0] != "" {
			parts = append(parts, genai.NewPartFromURI(imageURL[0], "image/jpeg"))
		}
		
		if len(parts) == 0 {
			return "无法处理空消息"
		}
		
		return s.chatWithParts(userId, parts)
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
	
	// 从数据库加载历史消息
	var history []*genai.Content
	if db.ChatDbInstance != nil {
		dbMsgs, err := db.ChatDbInstance.GetMsgList(config.Bot_Type_Gemini, userId)
		if err == nil {
			for _, dbMsg := range dbMsgs {
				// 跳过没有内容的系统消息或空消息
				if len(dbMsg.Parts) > 0 && dbMsg.Role != "system" {
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