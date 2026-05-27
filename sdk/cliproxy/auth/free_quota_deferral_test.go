package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type testFreeQuotaDeferralErr struct{ authID string }

func (e testFreeQuotaDeferralErr) Error() string             { return "free quota deferred: " + e.authID }
func (e testFreeQuotaDeferralErr) IsFreeQuotaDeferral() bool { return true }

type testRateLimitSwitchErr struct{ authID string }

func (e testRateLimitSwitchErr) Error() string           { return "rate_limit_switch: " + e.authID }
func (e testRateLimitSwitchErr) IsRateLimitSwitch() bool { return true }
func (e testRateLimitSwitchErr) StatusCode() int         { return http.StatusTooManyRequests }

type deferralTestExecutor struct {
	providerName string
	deferFirst   int32
	callCount    atomic.Int32
	callLog      []deferralCallRecord
}

type deferralCallRecord struct {
	authID        string
	freeQuotaOnly bool
}

func (e *deferralTestExecutor) Identifier() string { return e.providerName }

func (e *deferralTestExecutor) Execute(ctx context.Context, auth *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	freeQuotaOnly := cliproxyexecutor.IsFreeQuotaOnly(ctx)
	e.callLog = append(e.callLog, deferralCallRecord{authID: auth.ID, freeQuotaOnly: freeQuotaOnly})
	n := e.callCount.Add(1)
	if int(n) <= int(e.deferFirst) {
		return cliproxyexecutor.Response{}, testFreeQuotaDeferralErr{authID: auth.ID}
	}
	return cliproxyexecutor.Response{Payload: []byte(`{"ok":true}`)}, nil
}

func (e *deferralTestExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, fmt.Errorf("not used in deferral tests")
}

func (e *deferralTestExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) {
	return auth, nil
}

func (e *deferralTestExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (e *deferralTestExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("not used in deferral tests")
}

type rateLimitSwitchTestExecutor struct {
	providerName string
	callCount    atomic.Int32
}

func (e *rateLimitSwitchTestExecutor) Identifier() string { return e.providerName }

func (e *rateLimitSwitchTestExecutor) Execute(_ context.Context, auth *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	e.callCount.Add(1)
	return cliproxyexecutor.Response{}, testRateLimitSwitchErr{authID: auth.ID}
}

func (e *rateLimitSwitchTestExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, fmt.Errorf("not used in rate limit switch tests")
}

func (e *rateLimitSwitchTestExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) {
	return auth, nil
}

func (e *rateLimitSwitchTestExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (e *rateLimitSwitchTestExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("not used in rate limit switch tests")
}

func buildDeferralManager(t *testing.T, n int, provider string, exec ProviderExecutor) *Manager {
	t.Helper()
	mgr := NewManager(nil, &RoundRobinSelector{}, nil)
	mgr.RegisterExecutor(exec)
	ctx := context.Background()
	for i := 0; i < n; i++ {
		_, err := mgr.Register(ctx, &Auth{
			ID:       fmt.Sprintf("%s-auth-%d", provider, i+1),
			Provider: provider,
			Status:   StatusActive,
		})
		if err != nil {
			t.Fatalf("register auth %d: %v", i+1, err)
		}
	}
	return mgr
}

func TestFreeQuotaDeferral_FansOutBeforeCreditPhase(t *testing.T) {
	const provider = "antigravity"
	exec := &deferralTestExecutor{
		providerName: provider,
		deferFirst:   2,
	}
	mgr := buildDeferralManager(t, 3, provider, exec)

	resp, err := mgr.Execute(context.Background(), []string{provider}, cliproxyexecutor.Request{}, cliproxyexecutor.Options{Metadata: map[string]any{}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(resp.Payload) != `{"ok":true}` {
		t.Fatalf("payload = %s, want success", resp.Payload)
	}
	if got := exec.callCount.Load(); got != 3 {
		t.Fatalf("call count = %d, want 3", got)
	}
	for i, rec := range exec.callLog {
		if !rec.freeQuotaOnly {
			t.Fatalf("call[%d] auth=%s freeQuotaOnly=false, want true", i, rec.authID)
		}
	}
}

func TestFreeQuotaDeferral_AllFreeDeferredThenCreditPhase(t *testing.T) {
	const provider = "antigravity"
	exec := &deferralTestExecutor{
		providerName: provider,
		deferFirst:   3,
	}
	mgr := buildDeferralManager(t, 3, provider, exec)

	resp, err := mgr.Execute(context.Background(), []string{provider}, cliproxyexecutor.Request{}, cliproxyexecutor.Options{Metadata: map[string]any{}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(resp.Payload) != `{"ok":true}` {
		t.Fatalf("payload = %s, want success", resp.Payload)
	}
	if got := exec.callCount.Load(); got != 4 {
		t.Fatalf("call count = %d, want 4", got)
	}
	for i := 0; i < 3; i++ {
		if !exec.callLog[i].freeQuotaOnly {
			t.Fatalf("call[%d] freeQuotaOnly=false, want true", i)
		}
	}
	if exec.callLog[3].freeQuotaOnly {
		t.Fatal("phase 2 call freeQuotaOnly=true, want false")
	}
}

func TestRateLimitSwitch_DoesNotLeakToCallerWhenAllCandidatesSwitch(t *testing.T) {
	const provider = "antigravity"
	exec := &rateLimitSwitchTestExecutor{providerName: provider}
	mgr := buildDeferralManager(t, 2, provider, exec)

	_, err := mgr.Execute(context.Background(), []string{provider}, cliproxyexecutor.Request{}, cliproxyexecutor.Options{Metadata: map[string]any{}})
	if err == nil {
		t.Fatal("Execute() error = nil, want public auth error")
	}
	if cliproxyexecutor.IsRateLimitSwitchErr(err) {
		t.Fatalf("Execute() leaked rate-limit switch error: %v", err)
	}
	if strings.Contains(err.Error(), "rate_limit_switch") {
		t.Fatalf("Execute() error leaked internal message: %v", err)
	}
	if got := exec.callCount.Load(); got != 2 {
		t.Fatalf("call count = %d, want 2", got)
	}
}

func TestFreeQuotaDeferral_ManagerHintSkipsColdFreeProbe(t *testing.T) {
	const provider = "antigravity"
	const authID = "ag-hinted-free-quota"
	const model = "claude-sonnet-4-6"

	exec := &deferralTestExecutor{providerName: provider}
	mgr := NewManager(nil, &RoundRobinSelector{}, nil)
	mgr.RegisterExecutor(exec)

	reg := registry.GetGlobalRegistry()
	reg.RegisterClient(authID, provider, []*registry.ModelInfo{{ID: model}})
	t.Cleanup(func() { reg.UnregisterClient(authID) })

	now := time.Now()
	resetAt := now.Add(2 * time.Hour).UTC().Format(time.RFC3339)
	statusMessage := fmt.Sprintf(`{"error":{"code":429,"message":"Individual quota reached. Contact your administrator to enable overages.","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"QUOTA_EXHAUSTED","domain":"cloudcode-pa.googleapis.com","metadata":{"model":"%s","quotaResetTimestamp":"%s","quotaResetDelay":"2h"}},{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"7200s"}]}}`, model, resetAt)
	_, err := mgr.Register(context.Background(), &Auth{
		ID:       authID,
		Provider: provider,
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			model: {
				Status:        StatusError,
				StatusMessage: statusMessage,
				UpdatedAt:     now,
			},
		},
	})
	if err != nil {
		t.Fatalf("register auth: %v", err)
	}

	resp, err := mgr.Execute(context.Background(), []string{provider}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{Metadata: map[string]any{}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(resp.Payload) != `{"ok":true}` {
		t.Fatalf("payload = %s, want success", resp.Payload)
	}
	if got := exec.callCount.Load(); got != 1 {
		t.Fatalf("call count = %d, want 1", got)
	}
	if len(exec.callLog) != 1 {
		t.Fatalf("call log length = %d, want 1", len(exec.callLog))
	}
	if exec.callLog[0].freeQuotaOnly {
		t.Fatal("first call freeQuotaOnly=true, want direct credit phase")
	}

	resp, err = mgr.Execute(context.Background(), []string{provider}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{Metadata: map[string]any{}})
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if string(resp.Payload) != `{"ok":true}` {
		t.Fatalf("second payload = %s, want success", resp.Payload)
	}
	if got := exec.callCount.Load(); got != 2 {
		t.Fatalf("call count after second request = %d, want 2", got)
	}
	if exec.callLog[1].freeQuotaOnly {
		t.Fatal("second call freeQuotaOnly=true, want direct credit phase")
	}

	updated, ok := mgr.GetByID(authID)
	if !ok || updated == nil {
		t.Fatal("expected updated auth")
	}
	state := updated.ModelStates[model]
	if state == nil || !strings.Contains(state.StatusMessage, "quotaResetTimestamp") {
		t.Fatalf("expected free quota cooldown state to survive credit success, got %#v", state)
	}
}

func TestAntigravityCreditPhaseCandidate_RoundRobinsCoolingAuths(t *testing.T) {
	const provider = "antigravity"
	const model = "claude-sonnet-4-6"

	exec := &deferralTestExecutor{providerName: provider}
	mgr := NewManager(nil, &RoundRobinSelector{}, nil)
	mgr.RegisterExecutor(exec)

	reg := registry.GetGlobalRegistry()
	now := time.Now()
	resetAt := now.Add(2 * time.Hour).UTC().Format(time.RFC3339)
	statusMessage := fmt.Sprintf(`{"error":{"code":429,"message":"Individual quota reached.","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"QUOTA_EXHAUSTED","domain":"cloudcode-pa.googleapis.com","metadata":{"model":"%s","quotaResetTimestamp":"%s","quotaResetDelay":"2h"}},{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"7200s"}]}}`, model, resetAt)

	allowed := make(map[string]struct{})
	for i := 1; i <= 3; i++ {
		authID := fmt.Sprintf("ag-credit-rr-%d", i)
		reg.RegisterClient(authID, provider, []*registry.ModelInfo{{ID: model}})
		t.Cleanup(func() { reg.UnregisterClient(authID) })
		SetAntigravityCreditsHint(authID, AntigravityCreditsHint{
			Known:     true,
			Available: true,
			UpdatedAt: now,
		})

		_, err := mgr.Register(context.Background(), &Auth{
			ID:       authID,
			Provider: provider,
			Status:   StatusActive,
			ModelStates: map[string]*ModelState{
				model: {
					Status:         StatusError,
					Unavailable:    true,
					StatusMessage:  statusMessage,
					NextRetryAfter: now.Add(2 * time.Hour),
					Quota: QuotaState{
						Exceeded:      true,
						NextRecoverAt: now.Add(2 * time.Hour),
					},
					UpdatedAt: now,
				},
			},
		})
		if err != nil {
			t.Fatalf("register auth %s: %v", authID, err)
		}
		allowed[authID] = struct{}{}
	}

	var got []string
	for i := 0; i < 4; i++ {
		auth, _, _, ok := mgr.pickAntigravityCreditPhaseCandidate(context.Background(), model, cliproxyexecutor.Options{}, nil, allowed)
		if !ok || auth == nil {
			t.Fatalf("pick %d returned no auth", i)
		}
		got = append(got, auth.ID)
	}

	want := []string{"ag-credit-rr-1", "ag-credit-rr-2", "ag-credit-rr-3", "ag-credit-rr-1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("picked auths = %v, want %v", got, want)
	}
}
