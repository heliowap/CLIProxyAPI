package openai

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

type catalogMergePatchExecutor struct{ auth.ProviderExecutor }

func (catalogMergePatchExecutor) Identifier() string       { return "codex" }
func (catalogMergePatchExecutor) SupportsApplyPatch() bool { return true }

func TestCodexClientModelsResponsePreservesPlanLabelWithToolCapabilities(t *testing.T) {
	const modelID = "codex-merge-plan-model"
	const clientID = "codex-merge-plan-client"
	modelRegistry := registry.GetGlobalRegistry()
	modelRegistry.RegisterClient(clientID, "codex", []*registry.ModelInfo{{
		ID:              modelID,
		MetadataModelID: "gpt-6.1-sol",
		DisplayName:     "CPA GPT 6.1 Sol",
	}})
	t.Cleanup(func() { modelRegistry.UnregisterClient(clientID) })
	manager := auth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(catalogMergePatchExecutor{})
	if _, errRegister := manager.Register(t.Context(), &auth.Auth{
		ID:         clientID,
		Provider:   "codex",
		Attributes: map[string]string{"plan_type": " pro "},
	}); errRegister != nil {
		t.Fatalf("register Codex auth: %v", errRegister)
	}
	cfg := &config.SDKConfig{}
	handler := NewOpenAIAPIHandler(handlers.NewBaseAPIHandlers(cfg, manager))
	for _, enabled := range []bool{false, true} {
		cfg.Client.Codex.EnableApplyPatch = enabled
		cfg.Client.Codex.OptimizeMultiAgentV2 = enabled
		response := handler.codexClientModelsResponse("0.153.4")
		entries, ok := response["models"].([]map[string]any)
		if !ok {
			t.Fatalf("models type = %T, want []map[string]any", response["models"])
		}
		var entry map[string]any
		for _, candidate := range entries {
			if candidate["slug"] == modelID {
				entry = candidate
				break
			}
		}
		if entry == nil {
			t.Fatalf("missing model %q", modelID)
		}
		if got := entry["display_name"]; got != "GPT 6.1 Sol (Pro)" {
			t.Fatalf("enable-apply-patch=%v: display_name = %#v, want GPT 6.1 Sol (Pro)", enabled, got)
		}
		if got := entry["apply_patch_tool_type"]; got != "freeform" {
			t.Fatalf("enable-apply-patch=%v: apply_patch_tool_type = %#v, want freeform", enabled, got)
		}
		if enabled && entry["multi_agent_version"] != "v2" {
			t.Fatalf("multi_agent_version = %#v, want v2", entry["multi_agent_version"])
		}
		tiers, ok := entry["service_tiers"].([]any)
		if !ok || len(tiers) != 1 || tiers[0].(map[string]any)["id"] != "priority" {
			t.Fatalf("service_tiers = %#v, want priority Fast mode", entry["service_tiers"])
		}
	}
}
