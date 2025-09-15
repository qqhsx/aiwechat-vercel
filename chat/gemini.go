// gemini.go

package chat

import (
	"context"
	"io"
	"net/http"
	"encoding/base64"
	"fmt"
	"time"
	"strconv"
	"strings"

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
		case genai.Blob:
			// 将图片数据编码为 Base64 字符串
			encodedData := base64.StdEncoding.EncodeToString(v.Data)
			// 这里硬编码了MIME类型，因为数据库中没有存储
			dbMsg.Parts = append(dbMsg.Parts, db.ContentPart{Type: "image", Data: encodedData, MIMEType: v.MIMEType})
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
			// 解码 Base64 字符串以获取图片数据
			imageData, err := base64.StdEncoding.DecodeString(part.Data)
			if err != nil {
				fmt.Printf("Base64 decoding failed: %v", err)
				continue
			}
			// 将数据转换为 genai.Blob
			content.Parts = append(content.Parts, genai.ImageData(part.MIMEType, imageData))
		}
	}
	return content
}

func (s *GeminiChat) getModel(userId string) string {
	if model, err := db.GetModel(userId, config.Bot_Type_Gemini); err == nil && model != "" {
		return model
	}
	// Use a valid model name for a recent version
	return "gemini-1.5-flash-latest"
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

	var parts []genai.Part
	
	// 处理图片 URL
	if len(imageURL) > 0 && imageURL[0] != "" {
		// 1. 发起HTTP请求下载图片
		resp, err := http.Get(imageURL[0])
		if err != nil {
			return "下载图片失败: " + err.Error()
		}
		defer resp.Body.Close()

		// 2. 检查响应状态码
		if resp.StatusCode != http.StatusOK {
			return fmt.Sprintf("下载图片失败，状态码: %d", resp.StatusCode)
		}
	
		// 3. 读取图片数据到内存
		imageData, err := io.ReadAll(resp.Body)
		if err != nil {
			return "读取图片数据失败: " + err.Error()
		}
		
		// 4. 获取图片MIME类型
		mimeType := http.DetectContentType(imageData)
		
		// 5. 创建genai.Blob
		imagePart := genai.ImageData(mimeType, imageData) 
	
		// 6. 将图片数据添加到parts中
		parts = append(parts, imagePart)
	}

	// 将文本消息添加到 parts 中，如果文本消息存在
	if msg != "" {
		parts = append(parts, genai.Text(msg))
	}
	
	var resp *genai.GenerateContentResponse
	
	// 加入重试机制
	for i := 0; i < 3; i++ {
		resp, err = model.GenerateContent(ctx, parts...)
		if err == nil {
			break
		}
		
		// 检查是否是 429 错误
		if strings.Contains(err.Error(), "429") {
			// 尝试从错误信息中解析精确的重试时间
			const retryPrefix = "retry in "
			if retryIndex := strings.Index(err.Error(), retryPrefix); retryIndex != -1 {
				// 提取重试时间字符串
				retryStr := err.Error()[retryIndex+len(retryPrefix):]
				parts := strings.Split(retryStr, "s")
				if len(parts) > 0 {
					if waitTime, errParse := strconv.Atoi(parts[0]); errParse == nil {
						fmt.Printf("Waiting for %d seconds before retrying...\n", waitTime)
						time.Sleep(time.Duration(waitTime) * time.Second)
						continue // 继续下一次重试
					}
				}
			}
			
			// 如果无法解析，使用指数退避
			fmt.Printf("Encountered 429 error, retrying with exponential backoff...\n")
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		return err.Error()
	}

	if err != nil {
		return err.Error()
	}

	var responseText string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			responseText += string(text)
		}
	}

	// 储存完整的聊天历史到数据库
	// 这是为了支持其他模型，如 GPT，因为它们需要历史记录来维持上下文
	var msgs = GetMsgListWithDb(config.Bot_Type_Gemini, userId, &genai.Content{
		Parts: parts,
		Role: GeminiUser,
	}, g.toDbMsg, g.toChatMsg)
	msgs = append(msgs, &genai.Content{
		Parts: []genai.Part{genai.Text(responseText)},
		Role: GeminiBot,
	})
	SaveMsgListWithDb(config.Bot_Type_Gemini, userId, msgs, g.toDbMsg)

	return responseText
}