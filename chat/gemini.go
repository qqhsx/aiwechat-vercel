package chat

import (
	"context"

	"github.com/google/generative-ai-go/genai"
	"github.com/pwh-pwh/aiwechat-vercel/config"
	"github.com/pwh-pwh/aiwechat-vercel/db"
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
	dbMsg := db.Msg{
		Role: msg.Role,
		Parts: []db.ContentPart{},
	}
	for _, part := range msg.Parts {
		switch v := part.(type) {
		case genai.Text:
			dbMsg.Parts = append(dbMsg.Parts, db.ContentPart{Type: "text", Data: string(v)})
		case genai.ImageData:
			// For local image data, you would store a path or unique identifier.
		case genai.ImageFromURI:
			dbMsg.Parts = append(dbMsg.Parts, db.ContentPart{Type: "image", Data: string(v)})
		}
	}
	return dbMsg
}

func (s *GeminiChat) toChatMsg(msg db.Msg) *genai.Content {
	content := &genai.Content{
		Role: msg.Role,
		Parts: []genai.Part{},
	}
	for _, part := range msg.Parts {
		switch part.Type {
		case "text":
			content.Parts = append(content.Parts, genai.Text(part.Data))
		case "image":
			content.Parts = append(content.Parts, genai.ImageFromURI(part.Data))
		}
	}
	return content
}

func (s *GeminiChat) getModel(userId string) string {
	if model, err := db.GetModel(userId, config.Bot_Type_Gemini); err == nil && model != "" {
		return model
	}
	return "gemini-2.0-flash"
}

func (g *GeminiChat) Chat(userId string, msg string, imageURL ...string) string {
	r, flag := DoAction(userId, msg)
	if flag {
		return r
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(g.key))
	if err != nil {
		return err.Error()
	}
	defer client.Close()
	model := client.GenerativeModel(g.getModel(userId))
	if g.maxTokens > 0 {
		model.SetMaxOutputTokens(int32(g.maxTokens))
	}
	cs := model.StartChat()

	var parts []genai.Part
	parts = append(parts, genai.Text(msg))
	if len(imageURL) > 0 {
		parts = append(parts, genai.ImageFromURI(imageURL[0]))
	}

	var msgs = GetMsgListWithDb(config.Bot_Type_Gemini, userId, &genai.Content{
		Parts: parts,
		Role: GeminiUser,
	}, g.toDbMsg, g.toChatMsg)

	if len(msgs) > 1 {
		cs.History = msgs[:len(msgs)-1]
	}

	resp, err := cs.SendMessage(ctx, parts...)
	if err != nil {
		return err.Error()
	}

	var responseText string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			responseText += string(text)
		}
	}

	msgs = append(msgs, &genai.Content{
		Parts: []genai.Part{genai.Text(responseText)},
		Role: GeminiBot,
	})

	SaveMsgListWithDb(config.Bot_Type_Gemini, userId, msgs, g.toDbMsg)
	return responseText
}