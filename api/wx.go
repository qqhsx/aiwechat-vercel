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

	// 首先，检查并处理所有命令
	if msgType == message.MsgTypeText {
		if actionReply, isAction := chat.DoAction(userId, msgContent); isAction {
			return actionReply
		}
	}
	
	// 如果不是命令，再根据用户当前选择的模式处理
	bot := chat.GetChatBot(config.GetUserBotType(userId))
	
	if msgType == message.MsgTypeText {
		replyMsg = bot.Chat(userId, msgContent)
	} else if msgType == message.MsgTypeImage {
		// 检查当前机器人是否支持图片输入
		if _, ok := bot.(*chat.KeywordChat); ok {
			// 关键词模式，直接返回图片链接
			replyMsg = bot.HandleMediaMsg(msg)
			return
		}

		// 检查当前的 bot 是否是支持多模态的AI模型
		if botType := config.GetUserBotType(userId); botType != config.Bot_Type_Gemini {
			// 当前AI模式不支持多模态，返回提示
			replyMsg = fmt.Sprintf("您当前的 %s 机器人只支持文本输入。如需图片解读，请使用 /gemini 切换到 Gemini 机器人。", botType)
			return
		}
		
		// 如果当前是 Gemini 模式，则进行图片解读
		geminiReply := bot.Chat(userId, "", msg.PicURL)
		replyBuilder := strings.Builder{}
		replyBuilder.WriteString("Gemini 图片解读：\n")
		replyBuilder.WriteString(geminiReply)
		replyMsg = replyBuilder.String()
	} else {
		replyMsg = bot.HandleMediaMsg(msg)
	}

	return
}