package obcache

import (
	"context"
	"sort"
)

// Hook defines a cache operation hook with optional priority and condition
type Hook struct {
	// Priority determines execution order (higher values execute first)
	// Default: 0 (execution order not guaranteed for hooks with same priority)
	Priority int

	// Condition optionally filters hook execution
	// If nil, hook always executes
	// If returns false, hook is skipped
	Condition func(ctx context.Context, key string) bool

	// Handler is the actual hook function
	// Set exactly one of: OnHit, OnMiss, OnEvict, OnInvalidate
	OnHit        func(ctx context.Context, key string, value any)
	OnMiss       func(ctx context.Context, key string)
	OnEvict      func(ctx context.Context, key string, value any, reason EvictReason)
	OnInvalidate func(ctx context.Context, key string)
}

// Hooks contains all registered cache event hooks
type Hooks struct {
	onHit        []Hook
	onMiss       []Hook
	onEvict      []Hook
	onInvalidate []Hook
}

// NewHooks creates a new Hooks instance
func NewHooks() *Hooks {
	return &Hooks{}
}

// EvictReason indicates why a cache entry was evicted
type EvictReason int

const (
	// EvictReasonLRU indicates the entry was evicted due to LRU policy
	EvictReasonLRU EvictReason = iota

	// EvictReasonTTL indicates the entry was evicted due to TTL expiration
	EvictReasonTTL

	// EvictReasonCapacity indicates the entry was evicted due to capacity limits
	EvictReasonCapacity
)

func (r EvictReason) String() string {
	switch r {
	case EvictReasonLRU:
		return "LRU"
	case EvictReasonTTL:
		return "TTL"
	case EvictReasonCapacity:
		return "Capacity"
	default:
		return "Unknown"
	}
}

// AddOnHit registers a hook that executes on cache hits
func (h *Hooks) AddOnHit(fn func(ctx context.Context, key string, value any), opts ...HookOption) {
	hook := Hook{OnHit: fn}
	for _, opt := range opts {
		opt(&hook)
	}
	h.onHit = append(h.onHit, hook)
}

// AddOnMiss registers a hook that executes on cache misses
func (h *Hooks) AddOnMiss(fn func(ctx context.Context, key string), opts ...HookOption) {
	hook := Hook{OnMiss: fn}
	for _, opt := range opts {
		opt(&hook)
	}
	h.onMiss = append(h.onMiss, hook)
}

// AddOnEvict registers a hook that executes when entries are evicted
func (h *Hooks) AddOnEvict(fn func(ctx context.Context, key string, value any, reason EvictReason), opts ...HookOption) {
	hook := Hook{OnEvict: fn}
	for _, opt := range opts {
		opt(&hook)
	}
	h.onEvict = append(h.onEvict, hook)
}

// AddOnInvalidate registers a hook that executes when entries are invalidated
func (h *Hooks) AddOnInvalidate(fn func(ctx context.Context, key string), opts ...HookOption) {
	hook := Hook{OnInvalidate: fn}
	for _, opt := range opts {
		opt(&hook)
	}
	h.onInvalidate = append(h.onInvalidate, hook)
}

// HookOption configures a hook
type HookOption func(*Hook)

// WithPriority sets the hook execution priority (higher values execute first)
func WithPriority(priority int) HookOption {
	return func(h *Hook) {
		h.Priority = priority
	}
}

// WithCondition sets a condition that must be true for the hook to execute
func WithCondition(condition func(ctx context.Context, key string) bool) HookOption {
	return func(h *Hook) {
		h.Condition = condition
	}
}

// invokeOnHitWithCtx calls all OnHit hooks with context
func (h *Hooks) invokeOnHitWithCtx(ctx context.Context, key string, value any, _ []any) {
	h.invokeHooks(h.onHit, func(hook Hook) {
		if hook.Condition == nil || hook.Condition(ctx, key) {
			hook.OnHit(ctx, key, value)
		}
	})
}

// invokeOnMissWithCtx calls all OnMiss hooks with context
func (h *Hooks) invokeOnMissWithCtx(ctx context.Context, key string, _ []any) {
	h.invokeHooks(h.onMiss, func(hook Hook) {
		if hook.Condition == nil || hook.Condition(ctx, key) {
			hook.OnMiss(ctx, key)
		}
	})
}

// invokeOnEvict calls all OnEvict hooks
func (h *Hooks) invokeOnEvict(key string, value any, reason EvictReason) {
	h.invokeOnEvictWithCtx(context.Background(), key, value, reason, nil)
}

// invokeOnEvictWithCtx calls all OnEvict hooks with context
func (h *Hooks) invokeOnEvictWithCtx(ctx context.Context, key string, value any, reason EvictReason, _ []any) {
	h.invokeHooks(h.onEvict, func(hook Hook) {
		if hook.Condition == nil || hook.Condition(ctx, key) {
			hook.OnEvict(ctx, key, value, reason)
		}
	})
}

// invokeOnInvalidateWithCtx calls all OnInvalidate hooks with context
func (h *Hooks) invokeOnInvalidateWithCtx(ctx context.Context, key string, _ []any) {
	h.invokeHooks(h.onInvalidate, func(hook Hook) {
		if hook.Condition == nil || hook.Condition(ctx, key) {
			hook.OnInvalidate(ctx, key)
		}
	})
}

// invokeHooks executes hooks in priority order (highest priority first)
func (h *Hooks) invokeHooks(hooks []Hook, execute func(Hook)) {
	if len(hooks) == 0 {
		return
	}

	// Sort by priority (highest first) if needed
	if len(hooks) > 1 {
		sorted := make([]Hook, len(hooks))
		copy(sorted, hooks)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Priority > sorted[j].Priority
		})
		hooks = sorted
	}

	// Execute hooks
	for _, hook := range hooks {
		execute(hook)
	}
}
