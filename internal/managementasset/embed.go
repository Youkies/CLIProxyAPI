package managementasset

import (
	"bytes"
	_ "embed"
	"sync"
)

//go:embed bundled/management.html
var bundledManagementHTML []byte

//go:embed bundled/credits_overlay.js
var bundledCreditsOverlayJS []byte

var (
	bundledHTMLOnce sync.Once
	bundledHTML     []byte
)

// BundledHTML returns the control panel HTML embedded into the server binary.
func BundledHTML() []byte {
	bundledHTMLOnce.Do(func() {
		bundledHTML = injectCreditsOverlay(bundledManagementHTML, bundledCreditsOverlayJS)
	})
	return bundledHTML
}

func injectCreditsOverlay(html []byte, js []byte) []byte {
	if len(html) == 0 || len(js) == 0 {
		return append([]byte(nil), html...)
	}
	if bytes.Contains(html, []byte("data-cliproxy-ai-credits-overlay")) {
		return append([]byte(nil), html...)
	}
	script := make([]byte, 0, len(js)+96)
	script = append(script, []byte("\n<script data-cliproxy-ai-credits-overlay>\n")...)
	script = append(script, js...)
	script = append(script, []byte("\n</script>\n")...)
	idx := bytes.LastIndex(bytes.ToLower(html), []byte("</body>"))
	if idx < 0 {
		out := make([]byte, 0, len(html)+len(script))
		out = append(out, html...)
		out = append(out, script...)
		return out
	}
	out := make([]byte, 0, len(html)+len(script))
	out = append(out, html[:idx]...)
	out = append(out, script...)
	out = append(out, html[idx:]...)
	return out
}
