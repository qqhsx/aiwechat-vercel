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
	var parts []db.ContentPart
	for _, part := range msg.Parts {
		switch v := part.(type) {
		case genai.Text:
			parts = append(parts, db.ContentPart{Type: "text", Data: string(v)})
		case genai.FileData:
			parts = append(parts, db.ContentPart{Type: "image", Data: v.URI, MIMEType: v.MIMEType})
		}
	}
	return db.Msg{
		Role:  msg.Role,
		Parts: parts,
	}
}

func (s *GeminiChat) toChatMsg(msg db.Msg) *genai.Content {
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

func (s *GeminiChat) HandleMediaMsg(msg *message.MixMessage) string {
	simpleChat := SimpleChat{}
	return simpleChat.HandleMediaMsg(msg)
}

func (s *GeminiChat) Chat(userId string, msg string, imageURL ...string) string {
	r, flag := DoAction(userId, msg)
	if flag {
		return r
	}
	
	cacheKey := fmt.Sprintf("%s:%s", userId, msg)
	if msg == "" && len(imageURL) > 0 && imageURL[0] != "" {
		cacheKey = fmt.Sprintf("%s:%s", userId, imageURL[0])
	}
	
	return WithTimeChat(userId, cacheKey, func(userId, key string) string {
		var parts []genai.Part
		if msg != "" {
			parts = append(parts, genai.Text(msg))
		}
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
	cs := model.StartChat()
	
	var history []*genai.Content
	if db.ChatDbInstance != nil {
		dbMsgs, err := db.ChatDbInstance.GetMsgList(config.Bot_Type_Gemini, userId)
		if err == nil {
			for _, dbMsg := range dbMsgs {
				if len(dbMsg.Parts) > 0 && dbMsg.Role != "system" {
					history = append(history, s.toChatMsg(dbMsg))
				}
			}
		}
	}
	cs.History = history
	
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