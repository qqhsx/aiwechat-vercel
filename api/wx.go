// wx.go

package api

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pwh-pwh/aiwechat-vercel/chat"
	"github.com/pwh-pwh/aiwechat-vercel/config"
	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	offConfig "github.com/silenceper/wechat/v2/officialaccount/config"
	"github.com/silenceper/wechat/v2/officialaccount/message"
)

// =======================
// 用户上下文缓存
// =======================
type UserContext struct {
	LastText  string
	LastImage string
	Timestamp time.Time
}

var (
	userContextMap = make(map[string]*UserContext)
	ctxLock        sync.Mutex
)

func setUserContext(userID, text, image string) {
	ctxLock.Lock()
	defer ctxLock.Unlock()
	userContextMap[userID] = &UserContext{
		LastText:  text,
		LastImage: image,
		Timestamp: time.Now(),
	}
}

func getUserContext(userID string) *UserContext {
	ctxLock.Lock()
	defer ctxLock.Unlock()
	if ctx, ok := userContextMap[userID]; ok {
		// 超过 15 秒清理
		if time.Since(ctx.Timestamp) > 15*time.Second {
			delete(userContextMap, userID)
			return nil
		}
		return ctx
	}
	return nil
}

func clearUserContext(userID string) {
	ctxLock.Lock()
	defer ctxLock.Unlock()
	delete(userContextMap, userID)
}

// =======================
// 微信入口
// =======================
func Wx(rw http.ResponseWriter, req *http.Request) {
	wc := wechat.NewWechat()
	memory := cache.NewMemory()
	cfg := &offConfig.Config{
		AppID:     "",
		AppSecret: "",
		Token:     config.GetWxToken(),
		Cache:     memory,
	}
	officialAccount := wc.GetOfficialAccount(cfg)

	server := officialAccount.GetServer(req, rw)
	server.SkipValidate(true)

	server.SetMessageHandler(func(msg *message.MixMessage) *message.Reply {
		replyMsg := handleWxMessage(msg)
		text := message.NewText(replyMsg)
		return &message.Reply{MsgType: message.MsgTypeText, MsgData: text}
	})

	err := server.Serve()
	if err != nil {
		fmt.Println(err)
		return
	}
	server.Send()
}

// =======================
// 消息处理
// =======================
func handleWxMessage(msg *message.MixMessage) (replyMsg string) {
	msgType := msg.MsgType
	msgContent := msg.Content
	userId := string(msg.FromUserName)

	// 用户鉴权
	if config.GetAddMePassword() != "" && !config.IsUserAuthenticated(userId) {
		if msgType == message.MsgTypeText {
			if msgContent == "/addme" || (len(msgContent) > len("/addme") && msgContent[:len("/addme")] == "/addme") {
				bot := chat.GetChatBot(config.GetUserBotType(userId))
				replyMsg = bot.Chat(userId, msgContent)
			} else {
				replyMsg = "功能还在开发中"
			}
		} else {
			replyMsg = "功能还在开发中"
		}
		return
	}

	bot := chat.GetChatBot(config.GetUserBotType(userId))

	// =======================
	// 文本消息
	// =======================
	if msgType == message.MsgTypeText {
		ctx := getUserContext(userId)
		if ctx != nil && ctx.LastImage != "" {
			// 组合 “文字 + 最近图片”
			geminiReply := bot.Chat(userId, msgContent, ctx.LastImage)

			var replyBuilder strings.Builder
			replyBuilder.WriteString("图片链接：\n")
			replyBuilder.WriteString(ctx.LastImage)
			replyBuilder.WriteString("\n\nGemini 图片解读：\n")
			replyBuilder.WriteString(geminiReply)
			replyMsg = replyBuilder.String()

			// 用完清理
			clearUserContext(userId)
		} else {
			// 普通纯文本聊天
			replyMsg = bot.Chat(userId, msgContent)
			// 记录上下文，防止用户先发文字再发图片
			setUserContext(userId, msgContent, "")
		}

	// =======================
	// 图片消息
	// =======================
	} else if msgType == message.MsgTypeImage {
		ctx := getUserContext(userId)
		if ctx != nil && ctx.LastText != "" {
			// 组合 “文字 + 当前图片”
			geminiReply := bot.Chat(userId, ctx.LastText, msg.PicURL)

			var replyBuilder strings.Builder
			replyBuilder.WriteString("图片链接：\n")
			replyBuilder.WriteString(msg.PicURL)
			replyBuilder.WriteString("\n\nGemini 图片解读：\n")
			replyBuilder.WriteString(geminiReply)
			replyMsg = replyBuilder.String()

			// 用完清理
			clearUserContext(userId)
		} else {
			// 单独的图片消息
			geminiReply := bot.Chat(userId, "", msg.PicURL)

			var replyBuilder strings.Builder
			replyBuilder.WriteString("图片链接：\n")
			replyBuilder.WriteString(msg.PicURL)
			replyBuilder.WriteString("\n\nGemini 图片解读：\n")
			replyBuilder.WriteString(geminiReply)
			replyMsg = replyBuilder.String()

			// 记录上下文，防止用户先发图再发文字
			setUserContext(userId, "", msg.PicURL)
		}

	// =======================
	// 其他媒体
	// =======================
	} else {
		replyMsg = bot.HandleMediaMsg(msg)
	}

	return
}
