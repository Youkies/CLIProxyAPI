package executor

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

// RequestedModelMetadataKey stores the client-requested model name in Options.Metadata.
const RequestedModelMetadataKey = "requested_model"

// RequestPathMetadataKey stores the inbound HTTP request path (e.g. "/v1/images/generations") in Options.Metadata.
// It is optional and may be absent for non-HTTP executions.
const RequestPathMetadataKey = "request_path"

// DisallowFreeAuthMetadataKey instructs auth selection to skip known free-tier credentials.
const DisallowFreeAuthMetadataKey = "disallow_free_auth"

// ReasoningEffortMetadataKey stores the client-requested reasoning effort for usage logs.
const ReasoningEffortMetadataKey = "reasoning_effort"

const (
	// PinnedAuthMetadataKey locks execution to a specific auth ID.
	PinnedAuthMetadataKey = "pinned_auth_id"
	// SelectedAuthMetadataKey stores the auth ID selected by the scheduler.
	SelectedAuthMetadataKey = "selected_auth_id"
	// SelectedAuthCallbackMetadataKey carries an optional callback invoked with the selected auth ID.
	SelectedAuthCallbackMetadataKey = "selected_auth_callback"
	// ExecutionSessionMetadataKey identifies a long-lived downstream execution session.
	ExecutionSessionMetadataKey = "execution_session_id"
)

// Request encapsulates the translated payload that will be sent to a provider executor.
type Request struct {
	// Model is the upstream model identifier after translation.
	Model string
	// Payload is the provider specific JSON payload.
	Payload []byte
	// Format represents the provider payload schema.
	Format sdktranslator.Format
	// Metadata carries optional provider specific execution hints.
	Metadata map[string]any
}

// Options controls execution behavior for both streaming and non-streaming calls.
type Options struct {
	// Stream toggles streaming mode.
	Stream bool
	// Alt carries optional alternate format hint (e.g. SSE JSON key).
	Alt string
	// Headers are forwarded to the provider request builder.
	Headers http.Header
	// Query contains optional query string parameters.
	Query url.Values
	// OriginalRequest preserves the inbound request bytes prior to translation.
	OriginalRequest []byte
	// SourceFormat identifies the inbound schema.
	SourceFormat sdktranslator.Format
	// Metadata carries extra execution hints shared across selection and executors.
	Metadata map[string]any
}

// Response wraps either a full provider response or metadata for streaming flows.
type Response struct {
	// Payload is the provider response in the executor format.
	Payload []byte
	// Metadata exposes optional structured data for translators.
	Metadata map[string]any
	// Headers carries upstream HTTP response headers for passthrough to clients.
	Headers http.Header
}

// StreamChunk represents a single streaming payload unit emitted by provider executors.
type StreamChunk struct {
	// Payload is the raw provider chunk payload.
	Payload []byte
	// Err reports any terminal error encountered while producing chunks.
	Err error
}

// StreamResult wraps the streaming response, providing both the chunk channel
// and the upstream HTTP response headers captured before streaming begins.
type StreamResult struct {
	// Headers carries upstream HTTP response headers from the initial connection.
	Headers http.Header
	// Chunks is the channel of streaming payload units.
	Chunks <-chan StreamChunk
}

// StatusError represents an error that carries an HTTP-like status code.
// Provider executors should implement this when possible to enable
// better auth state updates on failures (e.g., 401/402/429).
type StatusError interface {
	error
	StatusCode() int
}

type freeQuotaOnlyKey struct{}

// WithFreeQuotaOnly returns a context signalling that the executor should only
// attempt the free quota channel for the current request.
func WithFreeQuotaOnly(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, freeQuotaOnlyKey{}, true)
}

// IsFreeQuotaOnly reports whether the context is in free-quota-only mode.
func IsFreeQuotaOnly(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(freeQuotaOnlyKey{}).(bool)
	return v
}

// FreeQuotaDeferralError is returned by executors when an auth's free quota
// should be deferred to a later credit phase rather than marked as failed.
type FreeQuotaDeferralError interface {
	error
	IsFreeQuotaDeferral() bool
}

// FreeQuotaDeferralInfo carries the cooldown window observed from an upstream
// quota error so callers can persist or seed runtime cooldown state.
type FreeQuotaDeferralInfo interface {
	FreeQuotaDeferralError
	FreeQuotaAuthID() string
	FreeQuotaFamily() string
	FreeQuotaCooldownUntil() time.Time
	FreeQuotaMessage() string
}

// IsFreeQuotaDeferralErr reports whether err is a FreeQuotaDeferralError.
func IsFreeQuotaDeferralErr(err error) bool {
	var d FreeQuotaDeferralError
	return errors.As(err, &d) && d != nil && d.IsFreeQuotaDeferral()
}

// GetFreeQuotaDeferralInfo extracts optional cooldown details from err.
func GetFreeQuotaDeferralInfo(err error) (FreeQuotaDeferralInfo, bool) {
	var d FreeQuotaDeferralInfo
	if errors.As(err, &d) && d != nil && d.IsFreeQuotaDeferral() {
		return d, true
	}
	return nil, false
}

// RateLimitSwitchError is returned by executors when a transient per-credential
// QPM limit is hit and the conductor should try another credential.
type RateLimitSwitchError interface {
	error
	IsRateLimitSwitch() bool
}

// IsRateLimitSwitchErr reports whether err is a RateLimitSwitchError.
func IsRateLimitSwitchErr(err error) bool {
	var r RateLimitSwitchError
	return errors.As(err, &r) && r != nil && r.IsRateLimitSwitch()
}
