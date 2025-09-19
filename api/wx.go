// File: wx.go

package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pwh-pwh/aiwechat-vercel/chat"
	"github.com/pwh-pwh/aiwechat-vercel/config"
	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	offConfig "github.com/silenceper/wechat/v2/officialaccount/config"
	"github.com/silenceper/wechat/v2/officialaccount/message"
)

// Wx 处理微信公众号请求
func Wx(rw http.ResponseWriter, req *http.Request) {
	wc := wechat.NewWechat()
	memory := cache.NewMemory()
	cfg := &offConfig.Config{
		AppID:     config.GetWxAppId(),
		AppSecret: config.GetWxAppSecret(),
		Token:     config.GetWxToken(),
		Cache:     memory,
	}
	officialAccount := wc.GetOfficialAccount(cfg)

	// 传入request和responseWriter
	server := officialAccount.GetServer(req, rw)
	server.SkipValidate(true)

	// 设置接收消息的处理方法
	server.SetMessageHandler(func(msg *message.MixMessage) *message.Reply {
		// 回复消息：由 handleWxMessage 生成最终文本
		replyMsg := handleWxMessage(msg, officialAccount)

		// debug: 打印即将回复的纯文本长度，便于检查是否过长
		fmt.Printf("Will reply to user %s, reply length=%d\n", string(msg.FromUserName), len(replyMsg))

		// 构造文本回复
		text := message.NewText(replyMsg)
		return &message.Reply{MsgType: message.MsgTypeText, MsgData: text}
	})

	// 处理消息接收以及回复
	if err := server.Serve(); err != nil {
		fmt.Println("server.Serve error:", err)
		return
	}

	// 发送回复
	if err := server.Send(); err != nil {
		// Send 出错也打印出来，便于排查
		fmt.Println("server.Send error:", err)
	}

	// —— 调试用：输出最终发送给微信的完整 XML —— //
	// silenceper/wechat 的 Server 结构会把最终的 raw xml 放到 ResponseRawXMLMsg 字段
	if len(server.ResponseRawXMLMsg) > 0 {
		fmt.Printf("Final response XML:\n%s\n", string(server.ResponseRawXMLMsg))
	} else if server.ResponseMsg != nil {
		// 如果没有 raw xml，至少打印出 ResponseMsg 的结构，便于分析
		fmt.Printf("ResponseMsg (structure): %#v\n", server.ResponseMsg)
	} else {
		fmt.Println("No response captured: ResponseRawXMLMsg is empty and ResponseMsg is nil")
	}
}

// handleWxMessage 保持你原先的逻辑
// NOTE: 增加了对 officialAccount 参数的传递，以便在函数内获取 access token
func handleWxMessage(msg *message.MixMessage, oa *wechat.OfficialAccount) (replyMsg string) {
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

	switch msgType {
	case message.MsgTypeText:
		replyMsg = bot.Chat(userId, msgContent)
	case message.MsgTypeImage:
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
		geminiBot := chat.GetGeminiChatBot()
		geminiReply := geminiBot.Chat(userId, "", msg.PicURL)
		replyBuilder := strings.Builder{}
		replyBuilder.WriteString("Gemini 图片解读：\n")
		replyBuilder.WriteString(geminiReply)
		replyMsg = replyBuilder.String()
	case message.MsgTypeVoice:
		// NOTE: 新增代码，用于处理语音消息
		botType := config.GetUserBotType(userId)
		if botType != config.Bot_Type_Gemini {
			replyMsg = fmt.Sprintf("您当前的 %s 机器人只支持文本输入。如需语音解读，请使用 /gemini 切换到 Gemini 机器人。", botType)
			return
		}
		
		// 获取 access token
		accessToken, err := oa.GetAccessToken()
		if err != nil {
			replyMsg = fmt.Sprintf("获取微信 access token 失败: %v", err)
			return
		}

		// 下载语音文件
		voiceData, err := downloadWxMedia(accessToken, msg.MediaID)
		if err != nil {
			replyMsg = fmt.Sprintf("下载语音文件失败: %v", err)
			return
		}

		// 调用 Gemini 模型进行语音解读
		geminiBot := chat.GetGeminiChatBot()
		geminiReply := geminiBot.ChatWithVoice(userId, voiceData)
		replyBuilder := strings.Builder{}
		replyBuilder.WriteString("Gemini 语音解读：\n")
		replyBuilder.WriteString(geminiReply)
		replyMsg = replyBuilder.String()
	default:
		replyMsg = bot.HandleMediaMsg(msg)
	}

	return
}

// NOTE: 新增函数，用于从微信服务器下载临时素材
func downloadWxMedia(accessToken, mediaID string) ([]byte, error) {
	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/media/get?access_token=%s&media_id=%s", accessToken, mediaID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("下载微信媒体文件失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载微信媒体文件失败，状态码: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取微信媒体数据失败: %w", err)
	}

	return data, nil
}