package executor

import (
	"strings"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

func antigravityModelFamily(model string) (string, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(m, "claude") {
		return cliproxyexecutor.FamilyQuotaClaude, true
	}
	return cliproxyexecutor.FamilyQuotaGemini, true
}
