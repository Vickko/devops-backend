package data

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"devops-backend/internal/biz"

	"github.com/cloudwego/eino/schema"
	_ "modernc.org/sqlite"
)

// sqliteSessionRepo SQLite 实现的会话仓库（三层模型）
type sqliteSessionRepo struct {
	db *sql.DB
}

// NewSQLiteSessionRepo 创建 SQLite 会话仓库
func NewSQLiteSessionRepo(dbPath string) (biz.SessionRepo, error) {
	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 启用外键约束
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// 创建 session_trees 表
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS session_trees (
			id TEXT PRIMARY KEY,
			title TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create session_trees table: %w", err)
	}

	// 创建 sessions 表
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			tree_id TEXT NOT NULL,
			message_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (tree_id) REFERENCES session_trees(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create sessions table: %w", err)
	}

	// 创建 messages 表
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			parent_id INTEGER,
			role TEXT NOT NULL,
			model TEXT,
			message_data TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create messages table: %w", err)
	}

	// 兼容旧库：老的 messages 表可能没有 model 列
	if err := ensureMessagesModelColumn(db); err != nil {
		db.Close()
		return nil, err
	}

	// 创建索引
	db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_tree_id ON sessions(tree_id)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_messages_parent_id ON messages(parent_id)")

	return &sqliteSessionRepo{db: db}, nil
}

func ensureMessagesModelColumn(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(messages)")
	if err != nil {
		return fmt.Errorf("failed to query messages schema: %w", err)
	}
	defer rows.Close()

	// PRAGMA table_info 返回列：cid, name, type, notnull, dflt_value, pk
	var (
		cid       int
		name      string
		colType   string
		notNull   int
		dfltValue sql.NullString
		pk        int
	)
	hasModel := false
	for rows.Next() {
		if scanErr := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); scanErr != nil {
			continue
		}
		if name == "model" {
			hasModel = true
			break
		}
	}
	if hasModel {
		return nil
	}

	// 给旧表补一列，避免 SELECT/INSERT 直接报错
	if _, err := db.Exec("ALTER TABLE messages ADD COLUMN model TEXT"); err != nil {
		return fmt.Errorf("failed to migrate messages table (add model column): %w", err)
	}
	return nil
}

// NewConversation 创建新对话（tree + 首个 session）
func (r *sqliteSessionRepo) NewConversation() (treeID, sessionID string) {
	treeID = r.generateID("tree_")
	sessionID = r.generateID("session_")

	// 创建 tree
	r.db.Exec("INSERT INTO session_trees (id) VALUES (?)", treeID)
	// 创建首个 session
	r.db.Exec("INSERT INTO sessions (id, tree_id) VALUES (?, ?)", sessionID, treeID)

	return treeID, sessionID
}

// CreateBranchWithMessage 创建分支并追加首条消息
func (r *sqliteSessionRepo) CreateBranchWithMessage(parentMsgID int64, msg *schema.Message) (sessionID string, msgID int64, err error) {
	// 找到 parent 消息所属的 tree
	var treeID string
	err = r.db.QueryRow(`
		SELECT s.tree_id FROM messages m
		JOIN sessions s ON m.session_id = s.id
		WHERE m.id = ?
	`, parentMsgID).Scan(&treeID)
	if err != nil {
		return "", 0, fmt.Errorf("parent message not found: %w", err)
	}

	sessionID = r.generateID("session_")
	_, err = r.db.Exec(
		"INSERT INTO sessions (id, tree_id) VALUES (?, ?)",
		sessionID, treeID,
	)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create branch: %w", err)
	}

	// 序列化消息
	messageData, err := json.Marshal(msg)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal message: %w", err)
	}

	// 插入首条消息，parent_id 指向分支点
	result, err := r.db.Exec(
		"INSERT INTO messages (session_id, parent_id, role, model, message_data) VALUES (?, ?, ?, ?, ?)",
		sessionID, parentMsgID, string(msg.Role), "", string(messageData),
	)
	if err != nil {
		return "", 0, fmt.Errorf("failed to insert message: %w", err)
	}

	msgID, _ = result.LastInsertId()

	// 更新元数据
	r.updateMetadataAfterAppend(sessionID, msg)

	return sessionID, msgID, nil
}

// SessionExists 检查 session 是否存在
func (r *sqliteSessionRepo) SessionExists(sessionID string) bool {
	var exists int
	err := r.db.QueryRow("SELECT 1 FROM sessions WHERE id = ?", sessionID).Scan(&exists)
	return err == nil
}

// GetTreeID 获取 session 所属的 tree_id
func (r *sqliteSessionRepo) GetTreeID(sessionID string) (string, error) {
	var treeID string
	err := r.db.QueryRow("SELECT tree_id FROM sessions WHERE id = ?", sessionID).Scan(&treeID)
	if err != nil {
		return "", fmt.Errorf("%w: %s", biz.ErrSessionNotFound, sessionID)
	}
	return treeID, nil
}

// GetSessionMessages 获取 session 的完整消息链
func (r *sqliteSessionRepo) GetSessionMessages(sessionID string) biz.Session {
	// 一次查询获取该 session 所属 tree 的所有消息
	rows, err := r.db.Query(`
		SELECT m.id, m.session_id, m.parent_id, m.model, m.message_data
		FROM messages m
		JOIN sessions s ON m.session_id = s.id
		WHERE s.tree_id = (SELECT tree_id FROM sessions WHERE id = ?)
		ORDER BY m.id
	`, sessionID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	// 构建消息 map 和当前 session 消息列表
	type msgNode struct {
		id        int64
		sessionID string
		parentID  sql.NullInt64
		model     sql.NullString
		data      string
	}
	msgMap := make(map[int64]*msgNode)
	var currentSessionMsgs []*msgNode

	for rows.Next() {
		node := &msgNode{}
		if err := rows.Scan(&node.id, &node.sessionID, &node.parentID, &node.model, &node.data); err != nil {
			continue
		}
		msgMap[node.id] = node
		if node.sessionID == sessionID {
			currentSessionMsgs = append(currentSessionMsgs, node)
		}
	}

	// 如果 session 没有消息，返回空
	if len(currentSessionMsgs) == 0 {
		return nil
	}

	// 应用层回溯祖先：从第一条消息的 parent_id 沿 map 查找
	var ancestors []*biz.ChatResponse
	if currentSessionMsgs[0].parentID.Valid {
		parentID := currentSessionMsgs[0].parentID.Int64
		for parentID > 0 {
			node, ok := msgMap[parentID]
			if !ok {
				break
			}
			var msg schema.Message
			if json.Unmarshal([]byte(node.data), &msg) == nil {
				ancestors = append([]*biz.ChatResponse{{Message: msg, Model: node.model.String}}, ancestors...)
			}
			if node.parentID.Valid {
				parentID = node.parentID.Int64
			} else {
				break
			}
		}
	}

	// 解析当前 session 的消息
	var currentMsgs []*biz.ChatResponse
	for _, node := range currentSessionMsgs {
		var msg schema.Message
		if json.Unmarshal([]byte(node.data), &msg) == nil {
			currentMsgs = append(currentMsgs, &biz.ChatResponse{Message: msg, Model: node.model.String})
		}
	}

	// 合并：祖先 + 当前
	return append(ancestors, currentMsgs...)
}

// AppendMessage 追加消息到 session
func (r *sqliteSessionRepo) AppendMessage(sessionID string, msg *schema.Message, model string) (int64, error) {
	// 检查 session 是否存在
	if !r.SessionExists(sessionID) {
		return 0, fmt.Errorf("%w: %s", biz.ErrSessionNotFound, sessionID)
	}

	// 获取该 session 最后一条消息作为 parent
	var parentID sql.NullInt64
	r.db.QueryRow("SELECT MAX(id) FROM messages WHERE session_id = ?", sessionID).Scan(&parentID)

	// 序列化消息
	messageData, err := json.Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal message: %w", err)
	}

	// 插入消息
	var result sql.Result
	if parentID.Valid {
		result, err = r.db.Exec(
			"INSERT INTO messages (session_id, parent_id, role, model, message_data) VALUES (?, ?, ?, ?, ?)",
			sessionID, parentID.Int64, string(msg.Role), model, string(messageData),
		)
	} else {
		result, err = r.db.Exec(
			"INSERT INTO messages (session_id, role, model, message_data) VALUES (?, ?, ?, ?)",
			sessionID, string(msg.Role), model, string(messageData),
		)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to insert message: %w", err)
	}

	newMsgID, _ := result.LastInsertId()

	// 更新元数据
	r.updateMetadataAfterAppend(sessionID, msg)

	return newMsgID, nil
}

// updateMetadataAfterAppend 追加消息后更新元数据
func (r *sqliteSessionRepo) updateMetadataAfterAppend(sessionID string, msg *schema.Message) {
	// 获取 tree_id
	var treeID string
	r.db.QueryRow("SELECT tree_id FROM sessions WHERE id = ?", sessionID).Scan(&treeID)

	// 更新 session 的 message_count
	r.db.Exec("UPDATE sessions SET message_count = message_count + 1 WHERE id = ?", sessionID)

	// 获取当前 tree 的 title
	var currentTitle sql.NullString
	r.db.QueryRow("SELECT title FROM session_trees WHERE id = ?", treeID).Scan(&currentTitle)

	// 如果 title 为空且是用户消息，设置 title
	if (!currentTitle.Valid || currentTitle.String == "") && msg.Role == schema.User && msg.Content != "" {
		runes := []rune(msg.Content)
		var newTitle string
		if len(runes) > 15 {
			newTitle = string(runes[:15]) + "..."
		} else {
			newTitle = msg.Content
		}
		r.db.Exec("UPDATE session_trees SET title = ? WHERE id = ?", newTitle, treeID)
	}

	// 更新 updated_at（message_count 和其他字段通过 ListTrees 联合查询获取）
	r.db.Exec("UPDATE session_trees SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", treeID)
}

// DeleteTree 删除整个对话树
func (r *sqliteSessionRepo) DeleteTree(treeID string) {
	// CASCADE 会自动删除关联的 sessions 和 messages
	r.db.Exec("DELETE FROM session_trees WHERE id = ?", treeID)
}

// ListTrees 列出所有对话树（通过联合查询获取最新消息信息）
func (r *sqliteSessionRepo) ListTrees() ([]biz.SessionTreeInfo, error) {
	rows, err := r.db.Query(`
		SELECT
			st.id, st.title, st.created_at, st.updated_at,
			latest.session_id AS last_active_session_id,
			json_extract(latest.message_data, '$.content') AS last_message_content
		FROM session_trees st
		LEFT JOIN (
			SELECT s.tree_id, m.session_id, m.message_data
			FROM messages m
			JOIN sessions s ON m.session_id = s.id
			WHERE m.id IN (
				SELECT MAX(m2.id)
				FROM messages m2
				JOIN sessions s2 ON m2.session_id = s2.id
				GROUP BY s2.tree_id
			)
		) latest ON st.id = latest.tree_id
		ORDER BY st.updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query session trees: %w", err)
	}
	defer rows.Close()

	var trees []biz.SessionTreeInfo
	for rows.Next() {
		var id string
		var title, lastActiveSessionID, lastMsgContent sql.NullString
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &title, &createdAt, &updatedAt,
			&lastActiveSessionID, &lastMsgContent); err != nil {
			return nil, fmt.Errorf("failed to scan session tree: %w", err)
		}

		trees = append(trees, biz.SessionTreeInfo{
			ID:                  id,
			Title:               title.String,
			LastActiveSessionID: lastActiveSessionID.String,
			LastMessage:         lastMsgContent.String,
			CreatedAt:           createdAt,
			UpdatedAt:           updatedAt,
		})
	}

	return trees, nil
}

// Close 关闭数据库连接
func (r *sqliteSessionRepo) Close() error {
	return r.db.Close()
}

// generateID 生成唯一 ID
func (r *sqliteSessionRepo) generateID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(b)
}
