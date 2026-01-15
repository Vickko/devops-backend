package auth

import (
	"context"
	"errors"
)

const (
	// UserContextKey is the context key for authenticated user
	UserContextKey = "authenticated_user"
)

var (
	// ErrNoUserInContext is returned when no user is found in context
	ErrNoUserInContext = errors.New("no authenticated user in context")
)

// GetUserFromContext extracts authenticated user from request context
func GetUserFromContext(ctx context.Context) (*UserInfo, error) {
	val := ctx.Value(UserContextKey)
	if val == nil {
		return nil, ErrNoUserInContext
	}

	user, ok := val.(*UserInfo)
	if !ok {
		return nil, ErrNoUserInContext
	}

	return user, nil
}

// MustGetUserFromContext panics if no user in context (use after auth middleware)
func MustGetUserFromContext(ctx context.Context) *UserInfo {
	user, err := GetUserFromContext(ctx)
	if err != nil {
		panic("expected authenticated user in context")
	}
	return user
}
