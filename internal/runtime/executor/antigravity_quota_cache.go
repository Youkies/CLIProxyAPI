package executor

import (
	"net/http"
	"strings"
	"sync"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
)

type ag429Action int

const (
	ag429Unrelated ag429Action = iota
	ag429RateLimit
	ag429QuotaGone
	ag429AmbiguousTransient
)

const (
	agShortRateLimitThreshold    = 60 * time.Second
	agQuotaLongResetCap          = 24 * time.Hour
	agQuotaUnknownDelayDefault   = 2 * time.Hour
	agQuotaAmbiguousDelayDefault = 5 * time.Minute
	agQuotaAmbiguousCap          = 15 * time.Minute
	agQuotaCooldownJitterPct     = 0.25
	agProbeInterval              = 10 * time.Second
	agQPMCooldown                = 2 * time.Second
)

type agAuthQuotaState struct {
	freeQuotaCooldownUntil time.Time
	creditCooldownUntil    time.Time
	qpmCooldownUntil       time.Time
	lastProbeTime          time.Time
}

type agQuotaKey struct {
	authID string
	family string
}

var (
	agQuotaCache   = make(map[agQuotaKey]*agAuthQuotaState)
	agQuotaCacheMu sync.Mutex
)

func agGetOrCreateState(authID, family string) *agAuthQuotaState {
	key := agQuotaKey{authID: authID, family: family}
	s := agQuotaCache[key]
	if s == nil {
		s = &agAuthQuotaState{}
		agQuotaCache[key] = s
	}
	return s
}

func classify429(statusCode int, body []byte) (ag429Action, *time.Duration) {
	if statusCode != http.StatusTooManyRequests || len(body) == 0 {
		return ag429Unrelated, nil
	}
	reason := antigravityParseErrorReason(body)
	delay, _ := parseRetryDelay(body)

	switch reason {
	case "QUOTA_EXHAUSTED":
		return ag429QuotaGone, delay
	case "RATE_LIMIT_EXCEEDED":
		if delay != nil && *delay > agShortRateLimitThreshold {
			return ag429QuotaGone, delay
		}
		return ag429RateLimit, delay
	}

	if delay != nil && *delay > agShortRateLimitThreshold {
		return ag429QuotaGone, delay
	}
	msg := strings.ToLower(string(body))
	if strings.Contains(msg, "exhausted your capacity") || strings.Contains(msg, "quota_exhausted") {
		return ag429QuotaGone, delay
	}
	if isBareGRPCResourceExhausted(body, msg) {
		return ag429AmbiguousTransient, nil
	}
	if strings.Contains(msg, "resource_exhausted") || strings.Contains(msg, "resource has been exhausted") || strings.Contains(msg, "quota") {
		return ag429QuotaGone, delay
	}
	if delay != nil {
		return ag429RateLimit, delay
	}
	return ag429Unrelated, nil
}

func isBareGRPCResourceExhausted(body []byte, lowerMsg string) bool {
	if !strings.Contains(lowerMsg, "resource has been exhausted") {
		return false
	}
	if !strings.Contains(lowerMsg, "(e.g. check quota)") {
		return false
	}
	if antigravityParseErrorReason(body) != "" {
		return false
	}
	delay, _ := parseRetryDelay(body)
	return delay == nil
}

func agResolveQuotaParkDuration(hint *time.Duration) time.Duration {
	base := agQuotaUnknownDelayDefault
	if hint != nil {
		base = *hint
	}
	if base > agQuotaLongResetCap {
		base = agQuotaLongResetCap
	}
	if base < agShortRateLimitThreshold {
		base = agShortRateLimitThreshold
	}
	return withJitterPct(base, agQuotaCooldownJitterPct)
}

func agRecordFreeQuotaGone(authID, family string, hint *time.Duration) time.Time {
	if authID == "" {
		return time.Time{}
	}
	cooldown := agResolveQuotaParkDuration(hint)
	until := time.Now().Add(cooldown)
	agRecordFreeQuotaCooldownUntil(authID, family, until)
	return until
}

func agRecordFreeQuotaCooldownUntil(authID, family string, until time.Time) {
	if authID == "" || family == "" || until.IsZero() {
		return
	}
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agGetOrCreateState(authID, family)
	if until.After(s.freeQuotaCooldownUntil) {
		s.freeQuotaCooldownUntil = until
	}
}

func agResolveAmbiguousParkDuration() time.Duration {
	base := agQuotaAmbiguousDelayDefault
	if base > agQuotaAmbiguousCap {
		base = agQuotaAmbiguousCap
	}
	return withJitterPct(base, agQuotaCooldownJitterPct)
}

func agRecordAmbiguousFreeQuotaTransient(authID, family string) time.Time {
	if authID == "" {
		return time.Time{}
	}
	cooldown := agResolveAmbiguousParkDuration()
	until := time.Now().Add(cooldown)
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agGetOrCreateState(authID, family)
	s.freeQuotaCooldownUntil = until
	return until
}

func agRecordAmbiguousCreditTransient(authID, family string) {
	if authID == "" {
		return
	}
	cooldown := agResolveAmbiguousParkDuration()
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agGetOrCreateState(authID, family)
	s.creditCooldownUntil = time.Now().Add(cooldown)
}

func agRecordCreditGone(authID, family string, hint *time.Duration) {
	if authID == "" {
		return
	}
	cooldown := agResolveQuotaParkDuration(hint)
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agGetOrCreateState(authID, family)
	s.creditCooldownUntil = time.Now().Add(cooldown)
}

func agFreeQuotaInCooldown(authID, family string) bool {
	if authID == "" {
		return false
	}
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agQuotaCache[agQuotaKey{authID: authID, family: family}]
	return s != nil && time.Now().Before(s.freeQuotaCooldownUntil)
}

func agCreditInCooldown(authID, family string) bool {
	if authID == "" {
		return false
	}
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agQuotaCache[agQuotaKey{authID: authID, family: family}]
	return s != nil && time.Now().Before(s.creditCooldownUntil)
}

func agQPMInCooldown(authID, family string) bool {
	if authID == "" {
		return false
	}
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agQuotaCache[agQuotaKey{authID: authID, family: family}]
	return s != nil && time.Now().Before(s.qpmCooldownUntil)
}

func agRecordQPMRateLimit(authID, family string, retryAfter time.Duration) {
	if authID == "" {
		return
	}
	cooldown := agQPMCooldown
	if retryAfter > 0 {
		cooldown = retryAfter
		if cooldown < agQPMCooldown {
			cooldown = agQPMCooldown
		}
		if cooldown > 5*time.Second {
			cooldown = 5 * time.Second
		}
	}
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agGetOrCreateState(authID, family)
	s.qpmCooldownUntil = time.Now().Add(cooldown)
}

func agShouldProbe(authID, family string) bool {
	if authID == "" {
		return false
	}
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agGetOrCreateState(authID, family)
	now := time.Now()
	if now.Sub(s.lastProbeTime) >= agProbeInterval {
		s.lastProbeTime = now
		return true
	}
	return false
}

func agResetFreeQuota(authID, family string) {
	if authID == "" {
		return
	}
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agQuotaCache[agQuotaKey{authID: authID, family: family}]
	if s != nil {
		s.freeQuotaCooldownUntil = time.Time{}
	}
}

func agResetCredit(authID, family string) {
	if authID == "" {
		return
	}
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agQuotaCache[agQuotaKey{authID: authID, family: family}]
	if s != nil {
		s.creditCooldownUntil = time.Time{}
	}
}

// AgFreeQuotaCooldownSet returns a copy of auth IDs whose free quota is in cooldown.
func AgFreeQuotaCooldownSet() cliproxyexecutor.FreeQuotaCooldownHintMap {
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	now := time.Now()
	result := cliproxyexecutor.FreeQuotaCooldownHintMap{}
	for k, s := range agQuotaCache {
		if s == nil || !now.Before(s.freeQuotaCooldownUntil) {
			continue
		}
		sub := result[k.family]
		if sub == nil {
			sub = make(map[string]time.Time)
			result[k.family] = sub
		}
		sub[k.authID] = s.freeQuotaCooldownUntil
	}
	return result
}

func agQuotaStateForTest(authID, family string) (agAuthQuotaState, bool) {
	agQuotaCacheMu.Lock()
	defer agQuotaCacheMu.Unlock()
	s := agQuotaCache[agQuotaKey{authID: authID, family: family}]
	if s == nil {
		return agAuthQuotaState{}, false
	}
	return *s, true
}

func antigravityParseErrorReason(body []byte) string {
	details := gjson.GetBytes(body, "error.details")
	if details.Exists() && details.IsArray() {
		for _, d := range details.Array() {
			if d.Get("@type").String() == "type.googleapis.com/google.rpc.ErrorInfo" {
				return d.Get("reason").String()
			}
		}
	}
	return ""
}

func withJitterPct(base time.Duration, pct float64) time.Duration {
	if base <= 0 || pct <= 0 {
		return base
	}
	randSourceMutex.Lock()
	jitter := (randSource.Float64()*2 - 1) * pct
	randSourceMutex.Unlock()
	factor := 1 + jitter
	if factor <= 0 {
		return base
	}
	return time.Duration(float64(base) * factor)
}
