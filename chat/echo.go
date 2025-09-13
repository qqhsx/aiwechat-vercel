package chat

import "github.com/silenceper/wechat/v2/officialaccount/message"

type Echo struct{}

func (e *Echo) HandleMediaMsg(msg *message.MixMessage) string {
	return "不支持的消息类型"
}

func (e *Echo) Chat(userId string, msg string, imageURL ...string) string {
	return msg
}