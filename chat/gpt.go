package chat

import (
	"context"

	"os"

	"github.com/pwh-pwh/aiwechat-vercel/config"
	"github.com/pwh-pwh/aiwechat-vercel/db"
	"github.com/sashabaranov/go-openai"
)

type SimpleGptChat struct {
	token     string
	url       string
	maxTokens int
	BaseChat
}

func (s *SimpleGptChat) toDbMsg(msg openai.ChatCompletionMessage) db.Msg {
	return db.Msg{
		Role: msg.Role,
		Parts: []db.ContentPart{
			{Type: "text", Data: msg.Content},
		},
	}
}

func (s *SimpleGptChat) toChatMsg(msg db.Msg) openai.ChatCompletionMessage {
	text := ""
	if len(msg.Parts) > 0 {
		text = msg.Parts[0].Data
	}
	return openai.ChatCompletionMessage{
		Role:    msg.Role,
		Content: text,
	}
}

func (s *SimpleGptChat) getModel(userId string) string {
	if model, err := db.GetModel(userId, config.Bot_Type_Gpt); err == nil && model != "" {
		return model
	} else if model = os.Getenv("gptModel"); model != "" {
		return model
	}
	return "gpt-3.5-turbo"
}

func (s *SimpleGptChat) Chat(userId string, msg string, imageURL ...string) string {
	r, flag := DoAction(userId, msg)
	if flag {
		return r
	}

	cfg := openai.DefaultConfig(s.token)
	cfg.BaseURL = s.url
	client := openai.NewClientWithConfig(cfg)

	var msgs = GetMsgListWithDb(config.Bot_Type_Gpt, userId, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: msg}, s.toDbMsg, s.toChatMsg)
	req := openai.ChatCompletionRequest{
		Model:    s.getModel(userId),
		Messages: msgs,
	}
	// 如果设置了环境变量且合法，则增加maxTokens参数，否则不设置
	if s.maxTokens > 0 {
		req.MaxTokens = s.maxTokens // 参数名称参考：https://github.com/sashabaranov/go-openai
	}
	resp, err := client.CreateChatCompletion(context.Background(), req)
	if err != nil {
		return err.Error()
	}
	content := resp.Choices[0].Message.Content
	msgs = append(msgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: content})
	SaveMsgListWithDb(config.Bot_Type_Gpt, userId, msgs, s.toDbMsg)
	return content
}