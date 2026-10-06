package openai

import (
	"sort"
	"strings"

	codexauth "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/codex"
	codexmodels "github.com/router-for-me/CLIProxyAPI/v8/internal/client/codex/models"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func (h *OpenAIAPIHandler) codexClientModelsResponse(clientVersion ...string) map[string]any {
	version := ""
	if len(clientVersion) > 0 {
		version = clientVersion[0]
	}
	optimizeMultiAgentV2 := h != nil && h.Cfg != nil && h.Cfg.Client.Codex.OptimizeMultiAgentV2
	var applyPatchCapabilityForModel codexmodels.ApplyPatchCapabilityForModelFunc
	if h != nil && h.Cfg != nil && h.Cfg.Client.Codex.EnableApplyPatch {
		applyPatchCapabilityForModel = h.SupportsApplyPatchModel
	}
	modelRegistry := registry.GetGlobalRegistry()
	return codexmodels.BuildResponseForClientWithToolCapabilities(
		h.Models(),
		modelRegistry.GetModelProviders,
		modelRegistry.GetResponsesWebSearchCapability,
		applyPatchCapabilityForModel,
		optimizeMultiAgentV2,
		version,
		codexOAuthPlanLabel(h),
	)
}

func codexOAuthPlanLabel(h *OpenAIAPIHandler) string {
	if h == nil || h.AuthManager == nil {
		return ""
	}
	plans := make(map[string]struct{})
	for _, auth := range h.AuthManager.List() {
		if auth == nil || auth.Disabled {
			continue
		}
		plan := codexOAuthAuthPlan(auth)
		isCodexOAuth := strings.Contains(strings.ToLower(strings.TrimSpace(auth.Provider)), "codex") || strings.Contains(strings.ToLower(auth.ID+" "+auth.FileName), "codex")
		if !isCodexOAuth || plan == "" {
			continue
		}
		plans[strings.ToUpper(plan[:1])+plan[1:]] = struct{}{}
	}
	if len(plans) == 0 {
		return ""
	}
	labels := make([]string, 0, len(plans))
	for plan := range plans {
		labels = append(labels, plan)
	}
	sort.Strings(labels)
	return strings.Join(labels, "/")
}

func codexOAuthAuthPlan(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Attributes != nil {
		if plan := strings.TrimSpace(auth.Attributes["plan_type"]); plan != "" {
			return strings.ToLower(plan)
		}
	}
	if auth.Metadata != nil {
		if plan, ok := auth.Metadata["plan_type"].(string); ok && strings.TrimSpace(plan) != "" {
			return strings.ToLower(strings.TrimSpace(plan))
		}
		if token, ok := auth.Metadata["id_token"].(string); ok && token != "" {
			if claims, err := codexauth.ParseJWTToken(token); err == nil && claims != nil {
				return strings.ToLower(strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType))
			}
		}
	}
	return ""
}

// CodexClientModelsResponse builds a Codex client model response.
func CodexClientModelsResponse(models []map[string]any) map[string]any {
	return codexmodels.BuildResponse(models, nil, false)
}

// CodexClientModelsResponseWithMultiAgentV2 builds a Codex client model response
// and advertises multi-agent v2 for synthesized models when enabled.
func CodexClientModelsResponseWithMultiAgentV2(models []map[string]any, enabled bool) map[string]any {
	return codexmodels.BuildResponse(models, nil, enabled)
}

// CodexClientModelsResponseForClient builds a Codex client model response
// tailored for a specific client version.
func CodexClientModelsResponseForClient(models []map[string]any, clientVersion string, enabled bool) map[string]any {
	return codexmodels.BuildResponseForClient(models, nil, enabled, clientVersion)
}
