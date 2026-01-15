package biz

import (
	"errors"
	"time"

	"github.com/cloudwego/eino/schema"
)

var ErrSessionNotFound = errors.New("session not found")
var ErrTreeNotFound = errors.New("session tree not found")

// Session 会话消息集合（完整对话链）
type Session []*ChatResponse

// SessionTreeInfo 会话树元信息（对外展示）
type SessionTreeInfo struct {
	ID                  string    // tree_id
	Title               string    // 第一条用户消息前15字
	LastActiveSessionID string    // 最后活跃的 session
	LastMessage         string    // 最新消息内容
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// SessionRepo 会话仓库接口
type SessionRepo interface {
	// NewConversation 创建新对话（tree + 首个 session）
	NewConversation() (treeID, sessionID string)
	// CreateBranchWithMessage 创建分支并追加首条消息（同一 tree 下新建 session）
	CreateBranchWithMessage(parentMsgID int64, msg *schema.Message) (sessionID string, msgID int64, err error)
	// SessionExists 检查 session 是否存在
	SessionExists(sessionID string) bool
	// GetTreeID 获取 session 所属的 tree_id
	GetTreeID(sessionID string) (string, error)
	// GetSessionMessages 获取 session 的完整消息链（包含祖先消息）
	GetSessionMessages(sessionID string) Session
	// AppendMessage 追加消息到 session，返回新消息 ID
	// model: 使用的模型名，用户消息传空字符串
	AppendMessage(sessionID string, msg *schema.Message, model string) (int64, error)
	// DeleteTree 删除整个对话树（级联删除 sessions 和 messages）
	DeleteTree(treeID string)
	// ListTrees 列出所有对话树
	ListTrees() ([]SessionTreeInfo, error)
	// Close 关闭仓库连接
	Close() error
}
