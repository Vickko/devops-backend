package biz

import (
	"github.com/cloudwego/eino/schema"
)

// SessionUsecase handles session lifecycle: CRUD, listing, and message history.
type SessionUsecase struct {
	repo SessionRepo
}

// NewSessionUsecase creates a SessionUsecase.
func NewSessionUsecase(repo SessionRepo) *SessionUsecase {
	return &SessionUsecase{repo: repo}
}

// ResolveSession validates or creates a session.
// Returns the tree ID, resolved session ID, and whether a new conversation was created.
func (uc *SessionUsecase) ResolveSession(sessionID string) (treeID, resolvedID string, isNew bool, err error) {
	if sessionID == "" {
		treeID, resolvedID = uc.repo.NewConversation()
		return treeID, resolvedID, true, nil
	}
	resolvedID = sessionID
	treeID, err = uc.repo.GetTreeID(sessionID)
	if err != nil {
		return "", "", false, err
	}
	return treeID, resolvedID, false, nil
}

// AppendMessage appends a message to the session.
func (uc *SessionUsecase) AppendMessage(sessionID string, msg *schema.Message, model string) (int64, error) {
	return uc.repo.AppendMessage(sessionID, msg, model)
}

// GetHistory returns the message list for a session.
func (uc *SessionUsecase) GetHistory(sessionID string) ([]*schema.Message, error) {
	session := uc.repo.GetSessionMessages(sessionID)
	if session == nil {
		return nil, ErrSessionNotFound
	}
	return extractMessages(session), nil
}

// GetSession returns the full session (with model info per message).
func (uc *SessionUsecase) GetSession(sessionID string) (Session, error) {
	session := uc.repo.GetSessionMessages(sessionID)
	if session == nil {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// ListSessions lists all conversation trees.
func (uc *SessionUsecase) ListSessions() ([]SessionTreeInfo, error) {
	return uc.repo.ListTrees()
}

// extractMessages converts a Session into a slice of schema.Message pointers.
func extractMessages(session Session) []*schema.Message {
	msgs := make([]*schema.Message, len(session))
	for i, cr := range session {
		msgs[i] = &cr.Message
	}
	return msgs
}
