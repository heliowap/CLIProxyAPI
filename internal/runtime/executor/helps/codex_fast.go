package helps

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/thinking"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

const codexFastServiceTier = "priority"

// ApplyCodexFastServiceTier sets service_tier to "priority" when the model
// suffix includes fast (alone or combined with effort, e.g. fast-high) and the
// base model supports ChatGPT Fast mode. Official Fast maps to the request
// value "priority" and is independent of reasoning effort. Mini and Spark are
// not Fast-capable.
func ApplyCodexFastServiceTier(body []byte, model string) []byte {
	suffix := thinking.ParseSuffix(model)
	if !suffix.HasSuffix || !suffixHasCodexFast(suffix.RawSuffix) {
		return body
	}
	if !supportsCodexFastMode(suffix.ModelName) {
		return body
	}
	return SetStringIfDifferent(body, "service_tier", codexFastServiceTier)
}

// CodexModelWithoutFastSuffix removes the fast token from a model suffix so
// thinking can apply the remaining effort. gpt-5.6-sol(fast-high) becomes
// gpt-5.6-sol(high); gpt-5.6-sol(fast) becomes gpt-5.6-sol.
func CodexModelWithoutFastSuffix(model string) string {
	suffix := thinking.ParseSuffix(model)
	if !suffix.HasSuffix {
		return model
	}
	tokens := splitCodexSuffixTokens(suffix.RawSuffix)
	kept := make([]string, 0, len(tokens))
	removedFast := false
	for _, token := range tokens {
		if strings.EqualFold(token, "fast") {
			removedFast = true
			continue
		}
		kept = append(kept, token)
	}
	if !removedFast {
		return model
	}
	if len(kept) == 0 {
		return suffix.ModelName
	}
	return suffix.ModelName + "(" + strings.Join(kept, "-") + ")"
}

// CodexThinkingRequest returns a shallow copy of req whose model suffix has
// fast removed so ApplyRequestThinking can see the effort level.
func CodexThinkingRequest(req cliproxyexecutor.Request) cliproxyexecutor.Request {
	req.Model = CodexModelWithoutFastSuffix(req.Model)
	return req
}

func suffixHasCodexFast(rawSuffix string) bool {
	for _, token := range splitCodexSuffixTokens(rawSuffix) {
		if strings.EqualFold(token, "fast") {
			return true
		}
	}
	return false
}

func splitCodexSuffixTokens(rawSuffix string) []string {
	rawSuffix = strings.TrimSpace(rawSuffix)
	if rawSuffix == "" {
		return nil
	}
	parts := strings.Split(rawSuffix, "-")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if token := strings.TrimSpace(part); token != "" {
			out = append(out, token)
		}
	}
	return out
}

func supportsCodexFastMode(baseModel string) bool {
	switch strings.ToLower(strings.TrimSpace(baseModel)) {
	case "gpt-5.4", "gpt-5.5", "gpt-6-astra":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(strings.TrimSpace(baseModel)), "gpt-5.6")
	}
}
