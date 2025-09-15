package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pwh-pwh/aiwechat-vercel/chat"
	"github.com/pwh-pwh/aiwechat-vercel/config"
	"github.com/pwh-pwh/aiwechat-vercel/db"
	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	offConfig "github.com/silenceper/wechat/v2/officialaccount/config"
	"github.com/silenceper/wechat/v2/officialaccount/message"
)

// 定义用于缓存文本消息的Redis键前缀
const textCacheKeyPrefix = "text_cache:"

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

	// 传入request和responseWriter
	server := officialAccount.GetServer(req, rw)
	server.SkipValidate(true)
	//设置接收消息的处理方法
	server.SetMessageHandler(func(msg *message.MixMessage) *message.Reply {
		//回复消息：演示回复用户发送的消息
		replyMsg := handleWxMessage(msg)
		text := message.NewText(replyMsg)
		return &message.Reply{MsgType: message.MsgTypeText, MsgData: text}
	})

	//处理消息接收以及回复
	err := server.Serve()
	if err != nil {
		fmt.Println(err)
		return
	}
	//发送回复的消息
	server.Send()
}

func handleWxMessage(msg *message.MixMessage) (replyMsg string) {
	msgType := msg.MsgType
	msgContent := msg.Content
	userId := string(msg.FromUserName)

	// Check if user is authenticated (only if ADDME_PASSWORD is set)
	if config.GetAddMePassword() != "" && !config.IsUserAuthenticated(userId) {
		if msgType == message.MsgTypeText {
			// Only allow /addme command for non-authenticated users
			if msgContent == "/addme" || len(msgContent) > len("/addme") && msgContent[:len("/addme")] == "/addme" {
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
	
	botType := config.GetUserBotType(userId)
	bot := chat.GetChatBot(botType)

	switch msgType {
	case message.MsgTypeText:
		// 如果是指令，优先处理
		if r, flag := chat.DoAction(userId, msgContent); flag {
			replyMsg = r
			return
		}
		
		// 如果当前bot是gemini，缓存文本消息，等待后续图片
		if botType == config.Bot_Type_Gemini {
			db.SetValue(textCacheKeyPrefix+userId, msgContent, 10*time.Second)
			// 返回一个临时回复，避免用户看到两个回复
			return "已收到文字，请发送图片进行处理。"
		}
		
		// 其他情况，直接调用bot.Chat处理
		replyMsg = bot.Chat(userId, msgContent)

	case message.MsgTypeImage:
		// 获取图片链接
		imageLink := msg.PicURL

		// 如果当前bot是gemini，检查是否有缓存的文本消息
		if botType == config.Bot_Type_Gemini {
			cachedText, err := db.GetValue(textCacheKeyPrefix + userId)
			if err == nil && cachedText != "" {
				// 找到缓存文本，组合图文消息
				db.DeleteKey(textCacheKeyPrefix + userId) // 删除缓存
				
				// 调用Gemini模型，同时传入文本和图片URL
				geminiReply := bot.Chat(userId, cachedText, imageLink)
				
				var replyBuilder strings.Builder
				replyBuilder.WriteString("图片链接：\n")
				replyBuilder.WriteString(imageLink)
				replyBuilder.WriteString("\n\nGemini 图片解读：\n")
				replyBuilder.WriteString(geminiReply)
				replyMsg = replyBuilder.String()
				return
			}
		}
		
		// 如果没有缓存文本，或者不是Gemini模型，只处理图片
		geminiReply := bot.Chat(userId, "", imageLink)
		
		var replyBuilder strings.Builder
		replyBuilder.WriteString("图片链接：\n")
		replyBuilder.WriteString(imageLink)
		replyBuilder.WriteString("\n\nGemini 图片解读：\n")
		replyBuilder.WriteString(geminiReply)
		replyMsg = replyBuilder.String()

	default:
		replyMsg = bot.HandleMediaMsg(msg)
	}
	return
}