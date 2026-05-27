package executor

import (
	"bytes"
	"net/http"
	"sync"
	"time"
)

const (
	noCreditCooldownDuration = 2 * time.Hour
	noCreditCooldownJitter   = 0.25
)

var (
	agNoCreditCacheMu sync.Mutex
	agNoCreditCache   = make(map[string]time.Time)
)

func agRecordNoCreditBalance(authID string) {
	if authID == "" {
		return
	}
	agNoCreditCacheMu.Lock()
	defer agNoCreditCacheMu.Unlock()
	agNoCreditCache[authID] = time.Now().Add(withJitterPct(noCreditCooldownDuration, noCreditCooldownJitter))
}

func agClearNoCredit(authID string) {
	if authID == "" {
		return
	}
	agNoCreditCacheMu.Lock()
	defer agNoCreditCacheMu.Unlock()
	delete(agNoCreditCache, authID)
}

// AgNoCreditCooldownSet returns a copy of auth IDs parked with no usable G1 credit.
func AgNoCreditCooldownSet() map[string]struct{} {
	agNoCreditCacheMu.Lock()
	defer agNoCreditCacheMu.Unlock()
	now := time.Now()
	result := make(map[string]struct{})
	for id, until := range agNoCreditCache {
		if now.Before(until) {
			result[id] = struct{}{}
		}
	}
	return result
}

func isInsufficientG1Credits(statusCode int, body []byte) bool {
	if statusCode != http.StatusTooManyRequests && statusCode != http.StatusPaymentRequired {
		return false
	}
	return bytes.Contains(body, []byte("INSUFFICIENT_G1_CREDITS_BALANCE"))
}
