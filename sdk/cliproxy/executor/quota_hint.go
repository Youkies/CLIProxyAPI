package executor

import (
	"context"
	"strings"
	"time"
)

// FreeQuotaCooldownHintMap is a per-family snapshot of auth IDs whose free
// quota is currently in cooldown.
type FreeQuotaCooldownHintMap map[string]map[string]time.Time

const (
	// FamilyQuotaClaude is the quota family covering claude models.
	FamilyQuotaClaude = "claude"
	// FamilyQuotaGemini is the quota family covering gemini models and the
	// default bucket for unknown non-claude model names.
	FamilyQuotaGemini = "gemini"
)

type freeQuotaCooldownHintKey struct{}

// WithFreeQuotaCooldownHint attaches a per-family snapshot of free-quota
// cooled-down auth IDs to ctx.
func WithFreeQuotaCooldownHint(ctx context.Context, hint FreeQuotaCooldownHintMap) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, freeQuotaCooldownHintKey{}, hint)
}

// FreeQuotaCooldownHint returns the per-family free-quota cooldown snapshot.
func FreeQuotaCooldownHint(ctx context.Context) FreeQuotaCooldownHintMap {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(freeQuotaCooldownHintKey{}).(FreeQuotaCooldownHintMap)
	return v
}

// ModelQuotaFamilyHint maps a model name to its quota family.
func ModelQuotaFamilyHint(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(m, "claude") {
		return FamilyQuotaClaude
	}
	return FamilyQuotaGemini
}

type noCreditCooldownHintKey struct{}

// WithNoCreditCooldownHint attaches a snapshot of auth IDs that currently have
// no usable paid credit.
func WithNoCreditCooldownHint(ctx context.Context, ids map[string]struct{}) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, noCreditCooldownHintKey{}, ids)
}

// NoCreditCooldownHint returns the no-credit snapshot, or nil if unset.
func NoCreditCooldownHint(ctx context.Context) map[string]struct{} {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(noCreditCooldownHintKey{}).(map[string]struct{})
	return v
}
