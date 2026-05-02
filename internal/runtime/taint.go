package runtime

import (
	"context"

	"bridgekeeper/internal/redact"
)

type taintContextKey struct{}

// WithTaintTracker attaches a taint tracker to ctx for one agent session.
func WithTaintTracker(ctx context.Context, tracker *redact.TaintTracker) context.Context {
	if tracker == nil {
		tracker = redact.NewTaintTracker()
	}
	return context.WithValue(ctx, taintContextKey{}, tracker)
}

// WithNewTaintTracker attaches a fresh taint tracker to ctx.
func WithNewTaintTracker(ctx context.Context) context.Context {
	return WithTaintTracker(ctx, redact.NewTaintTracker())
}

// TaintTrackerFromContext returns the taint tracker carried by ctx.
func TaintTrackerFromContext(ctx context.Context) (*redact.TaintTracker, bool) {
	tracker, ok := ctx.Value(taintContextKey{}).(*redact.TaintTracker)
	return tracker, ok
}
