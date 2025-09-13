package db

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/go-redis/redis/v8"
)

var (
	ChatDbInstance ChatDb        = nil
	RedisClient    *redis.Client = nil
	Cache          sync.Map
)

const (
	PROMPT_KEY = "prompt"
	MSG_KEY    = "msg"
	MODEL_KEY  = "model"
	TODO_KEY   = "todo"
	KEYWORD_REPLY_KEY = "keyword"
	LAST_AI_BOT_KEY = "lastAIBot" // 新增用于存储上次使用的AI模型
)

type KeywordReply struct {
    Keyword string `json:"keyword"`
    Reply   string `json:"reply"`
}

func init() {
	db, err := GetChatDb()
	if err != nil {
		fmt.Println(err)
		return
	}
	ChatDbInstance = db
}

// ContentPart represents a part of a message, which can be text or an image.
type ContentPart struct {
	Type     string `json:"type"` // "text" or "image"
	Data     string `json:"data"` // The text content or image URL
	MimeType string `json:"mime_type,omitempty"` // MIME type for image data
}

// Msg represents a message in a conversation.
type Msg struct {
	Role  string `json:"role"`
	Parts []ContentPart `json:"parts"`
}

type ChatDb interface {
	GetMsgList(botType string, userId string) ([]Msg, error)
	SetMsgList(botType string, userId string, msgList []Msg)
}

type RedisChatDb struct {
	client *redis.Client
}

func NewRedisChatDb(url string) (*RedisChatDb, error) {
	options, err := redis.ParseURL(url)
	if err != nil {
		fmt.Println(err)
		return nil, errors.New("redis url error")
	}
	options.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	client := redis.NewClient(options)
	RedisClient = client
	return &RedisChatDb{
		client: client,
	}, nil
}

func (r *RedisChatDb) GetMsgList(botType string, userId string) ([]Msg, error) {
	result, err := r.client.Get(context.Background(), fmt.Sprintf("%v:%v:%v", MSG_KEY, botType, userId)).Result()
	if err != nil {
		return nil, err
	}
	var msgList []Msg
	err = sonic.Unmarshal([]byte(result), &msgList)
	if err != nil {
		return nil, err
	}
	return msgList, nil
}

func (r *RedisChatDb) SetMsgList(botType string, userId string, msgList []Msg) {
	res, err := sonic.Marshal(msgList)
	if err != nil {
		fmt.Println(err)
		return
	}
	msgTime := os.Getenv("MSG_TIME")
	var expires time.Duration
	//转换为数字
	msgT, err := strconv.Atoi(msgTime)
	if err != nil || msgT < 0 {
		msgT = 30
		expires = time.Minute * time.Duration(msgT)
	} else if msgT == 0 {
		expires = 0 // 设置为0, 永不过期
	} else {
		expires = time.Minute * time.Duration(msgT)
	}

	r.client.Set(context.Background(), fmt.Sprintf("%v:%v:%v", MSG_KEY, botType, userId), res, expires)
}

func GetChatDb() (ChatDb, error) {
	kvUrl := os.Getenv("KV_URL")
	if kvUrl == "" {
		return nil, errors.New("请配置KV_URL")
	} else {
		db, err := NewRedisChatDb(kvUrl)
		if err != nil {
			return nil, err
		}
		return db, nil
	}
}

func GetValueWithMemory(key string) (string, bool) {
	value, ok := Cache.Load(key)
	if ok {
		return value.(string), ok
	}
	return "", false
}

func SetValueWithMemory(key string, val any) {
	Cache.Store(key, val)
}

func DeleteKeyWithMemory(key string) {
	Cache.Delete(key)
}

func GetValue(key string) (val string, err error) {
	val, flag := GetValueWithMemory(key)
	if !flag {
		if RedisClient == nil {
			return "", errors.New("redis client is nil")
		}
		val, err = RedisClient.Get(context.Background(), key).Result()
		if err == redis.Nil {
			return "", nil
		}
		SetValueWithMemory(key, val)
		return
	}
	return
}

func SetValue(key string, val any, expires time.Duration) (err error) {
	SetValueWithMemory(key, val)

	if RedisClient == nil {
		return errors.New("redis client is nil")
	}
	
	err = RedisClient.Set(context.Background(), key, val, expires).Err()

	return
}

func DeleteKey(key string) {
	DeleteKeyWithMemory(key)
	if RedisClient == nil {
		return
	}
	RedisClient.Del(context.Background(), key)
}

func DeleteMsgList(botType string, userId string) {
	RedisClient.Del(context.Background(), fmt.Sprintf("%v:%v:%v", MSG_KEY, botType, userId))
}

func SetPrompt(userId, botType, prompt string) {
	SetValue(fmt.Sprintf("%s:%s:%s", PROMPT_KEY, userId, botType), prompt, 0)
}

func GetPrompt(userId, botType string) (string, error) {
	return GetValue(fmt.Sprintf("%s:%s:%s", PROMPT_KEY, userId, botType))
}

func RemovePrompt(userId, botType string) {
	DeleteKey(fmt.Sprintf("%s:%s:%s", PROMPT_KEY, userId, botType))
}

// todolist format: "todo1|todo2|todo3"
func GetTodoList(userId string) (string, error) {
	tListStr, err := GetValue(fmt.Sprintf("%s:%s", TODO_KEY, userId))
	if err != nil && RedisClient == nil {
		return "", err
	}
	if tListStr == "" {
		return "todolist为空", nil
	}
	split := strings.Split(tListStr, "|")
	var sb strings.Builder
	for index, todo := range split {
		sb.WriteString(fmt.Sprintf("%d. %s\n", index+1, todo))
	}
	return sb.String(), nil
}

func AddTodoList(userId string, todo string) error {
	todoList, err := GetValue(fmt.Sprintf("%s:%s", TODO_KEY, userId))
	if err != nil && RedisClient == nil {
		return err
	}
	if todoList == "" {
		todoList = todo
	} else {
		todoList = fmt.Sprintf("%s|%s", todoList, todo)
	}
	return SetValue(fmt.Sprintf("%s:%s", TODO_KEY, userId), todoList, 0)
}

func DelTodoList(userId string, todoIndex int) error {
	todoList, err := GetValue(fmt.Sprintf("%s:%s", TODO_KEY, userId))
	if err != nil && RedisClient == nil {
		return err
	}
	todoList = strings.Split(todoList, "|")[todoIndex-1]
	return SetValue(fmt.Sprintf("%s:%s", TODO_KEY, userId), todoList, 0)
}

func SetModel(userId, botType, model string) error {
	if model == "" {
		DeleteKey(fmt.Sprintf("%s:%s:%s", MODEL_KEY, userId, botType))
		return nil
	} else {
		return SetValue(fmt.Sprintf("%s:%s:%s", MODEL_KEY, userId, botType), model, 0)
	}
}

func GetModel(userId, botType string) (string, error) {
	return GetValue(fmt.Sprintf("%s:%s:%s", MODEL_KEY, userId, botType))
}

// SetKeywordReply adds or updates a keyword reply.
func SetKeywordReply(keyword, reply string) error {
	replies, err := GetKeywordReplies()
	if err != nil && err != redis.Nil {
		return err
	}

	found := false
	for i, kr := range replies {
		if kr.Keyword == keyword {
			replies[i].Reply = reply
			found = true
			break
		}
	}
	if !found {
		replies = append(replies, KeywordReply{Keyword: keyword, Reply: reply})
	}

	res, err := sonic.Marshal(replies)
	if err != nil {
		return err
	}

	return SetValue(KEYWORD_REPLY_KEY, res, 0)
}

// GetKeywordReplies retrieves all keyword replies.
func GetKeywordReplies() ([]KeywordReply, error) {
	val, err := GetValue(KEYWORD_REPLY_KEY)
	if err != nil {
		if err == redis.Nil {
			return []KeywordReply{}, nil
		}
		return nil, err
	}

	// Handle empty string case gracefully
	if val == "" {
		return []KeywordReply{}, nil
	}

	var replies []KeywordReply
	err = sonic.Unmarshal([]byte(val), &replies)
	if err != nil {
		return nil, err
	}
	return replies, nil
}

// RemoveKeyword removes a keyword reply.
func RemoveKeyword(keyword string) error {
	replies, err := GetKeywordReplies()
	if err != nil {
		return err
	}

	var newReplies []KeywordReply
	for _, kr := range replies {
		if kr.Keyword != keyword {
			newReplies = append(newReplies, kr)
		}
	}

	if len(newReplies) == 0 {
		DeleteKey(KEYWORD_REPLY_KEY)
		return nil
	}

	res, err := sonic.Marshal(newReplies)
	if err != nil {
		return err
	}

	return SetValue(KEYWORD_REPLY_KEY, res, 0)
}

// SetLastAIBot adds or updates the last used AI bot type.
func SetLastAIBot(userId, botType string) error {
	return SetValue(fmt.Sprintf("%s:%s", LAST_AI_BOT_KEY, userId), botType, 0)
}

// GetLastAIBot retrieves the last used AI bot type.
func GetLastAIBot(userId string) (string, error) {
	return GetValue(fmt.Sprintf("%s:%s", LAST_AI_BOT_KEY, userId))
}