package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pwh-pwh/aiwechat-vercel/chat"
	"github.com/pwh-pwh/aiwechat-vercel/config"
	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	offConfig "github.com/silenceper/wechat/v2/officialaccount/config"
	"github.com/silenceper/wechat/v2/officialaccount/message"
)

func Wx(rw http.ResponseWriter, req *http.Request) {
	wc := wechat.NewWechat()
	memory := cache.NewMemory()
	cfg := &offConfig.Config{
		AppID: "",
		AppSecret: "",
		Token: config.GetWxToken(),
		Cache: memory,
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
		if msgType == message.MsgTypeImage {
			replyMsg = "功能还在开发中"
		} else if msgType == message.MsgTypeText {
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

	bot := chat.GetChatBot(config.GetUserBotType(userId))

	// 先处理文本消息
	if msgType == message.MsgTypeText {
		// bot.Chat 方法内部会处理所有指令和普通文本聊天
		replyMsg = bot.Chat(userId, msgContent)
		return
	}

	// 再处理媒体消息和事件
	if msgType == message.MsgTypeImage {
		// 如果当前 bot 是 ImageChat，直接返回 URL
		if _, ok := bot.(*chat.ImageChat); ok {
			replyMsg = bot.HandleMediaMsg(msg)
			return
		}
		
		// 如果是其他 AI bot，则进行图片解读
		geminiReply := bot.Chat(userId, msgContent, msg.PicURL)
		
		// 获取图片链接
		imageLink := msg.PicURL
		
		// 拼接回复内容，包括图片链接和 Gemini 的解读
		var replyBuilder strings.Builder
		replyBuilder.WriteString("图片链接：\n")
		replyBuilder.WriteString(imageLink)
		replyBuilder.WriteString("\n\nGemini 图片解读：\n")
		replyBuilder.WriteString(geminiReply)
		replyMsg = replyBuilder.String()
	} else {
		// 处理其他媒体和事件消息
		replyMsg = bot.HandleMediaMsg(msg)
	}

	return
}