package server

import (
	"context"
	"net/http"

	"github.com/cloudwego/eino-ext/devops"
)

// InitDevops initializes the eino devops server with an optional router.
// The router will be mounted at /api prefix.
func InitDevops(ctx context.Context, router http.Handler) error {
	if router == nil {
		return devops.Init(ctx)
	}
	return devops.Init(ctx, devops.WithHandler("/api", router))
}
