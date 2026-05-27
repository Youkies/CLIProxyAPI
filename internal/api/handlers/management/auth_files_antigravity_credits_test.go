package management

import (
	"context"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestListAuthFiles_IncludesAntigravityCreditsHint(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	const authID = "antigravity-credits-list-auth"
	manager := coreauth.NewManager(nil, nil, nil)
	record := &coreauth.Auth{
		ID:       authID,
		FileName: "antigravity-credits-list-auth.json",
		Provider: "antigravity",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"runtime_only": "true",
		},
	}
	if _, errRegister := manager.Register(context.Background(), record); errRegister != nil {
		t.Fatalf("failed to register auth record: %v", errRegister)
	}

	updatedAt := time.Now().UTC().Truncate(time.Second)
	coreauth.SetAntigravityCreditsHint(authID, coreauth.AntigravityCreditsHint{
		Known:           true,
		Available:       true,
		CreditAmount:    24092,
		MinCreditAmount: 50,
		PaidTierID:      "g1-pro-tier",
		UpdatedAt:       updatedAt,
	})

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)
	h.tokenStore = &memoryAuthStore{}

	entry := firstAuthFileEntry(t, h)
	creditsRaw, ok := entry["ai_credits"].(map[string]any)
	if !ok {
		t.Fatalf("expected ai_credits object, got %#v", entry["ai_credits"])
	}
	if got := creditsRaw["available"]; got != true {
		t.Fatalf("expected available=true, got %#v", got)
	}
	if got := creditsRaw["credit_amount"]; got != float64(24092) {
		t.Fatalf("expected credit_amount=24092, got %#v", got)
	}
	if got := creditsRaw["min_credit_amount"]; got != float64(50) {
		t.Fatalf("expected min_credit_amount=50, got %#v", got)
	}
	if got := creditsRaw["paid_tier_id"]; got != "g1-pro-tier" {
		t.Fatalf("expected paid_tier_id=%q, got %#v", "g1-pro-tier", got)
	}
	if _, ok := entry["antigravity_credits"].(map[string]any); !ok {
		t.Fatalf("expected antigravity_credits object, got %#v", entry["antigravity_credits"])
	}
}
