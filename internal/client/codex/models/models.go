// Package models builds model catalogs for official Codex clients.
package models

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
)

type codexClientModelsPayload struct {
	Models []map[string]any `json:"models"`
}

// ProvidersForModelFunc returns the providers registered for a model.
type ProvidersForModelFunc func(string) []string

// WebSearchCapabilityForModelFunc returns explicit conservative capability
// metadata for an exact public model ID. nil means unknown.
type WebSearchCapabilityForModelFunc func(string) *bool

// ToolSearchCapabilityForModelFunc reports whether every credential serving a
// public model ID can receive Codex tool_search as Anthropic native tool search.
type ToolSearchCapabilityForModelFunc func(id string) bool

var (
	codexClientModelTemplatesMu       sync.Mutex
	codexClientModelTemplatesLoaded   bool
	codexClientModelTemplatesRevision uint64
	codexClientModelTemplates         map[string]map[string]any
	codexClientDefaultTemplate        map[string]any
	codexClientModelTemplatesErr      error
)

var (
	codexGPTVersionPattern      = regexp.MustCompile(`(?i)gpt-(\d+)(?:\.(\d+))?`)
	codexGeminiPattern          = regexp.MustCompile(`(?i)(?:^|[/_-])gemini-`)
	codexGemini38Pattern        = regexp.MustCompile(`(?i)(?:^|[/_-])gemini-3[.-]8(?:$|[(_-])`)
	codexClaudePattern          = regexp.MustCompile(`(?i)(?:^|[/_-])claude-`)
	codexAllowedClaudePattern   = regexp.MustCompile(`(?i)(?:^|[/_-])claude-(?:fable-5[.-]1|opus-5[.-]5|sonnet-5[.-]5)(?:$|\()`)
	codexAllowedGLMPattern      = regexp.MustCompile(`(?i)(?:^|[/_-])glm-5[.-]3(?:-flash)?(?:$|\()`)
	codexAllowedKimiPattern     = regexp.MustCompile(`(?i)(?:^|[/_-])kimi-k3(?:$|\()`)
	codexAllowedDeepSeekPattern = regexp.MustCompile(`(?i)(?:^|[/_-])deepseek-v4[.-]1-flash(?:$|\()`)
)

var codexClientAllowedReasoningLevels = map[string]struct{}{
	"none":    {},
	"minimal": {},
	"low":     {},
	"medium":  {},
	"high":    {},
	"xhigh":   {},
	"max":     {},
	"ultra":   {},
}

var codexClientLegacyAllowedReasoningLevels = map[string]struct{}{
	"none":    {},
	"minimal": {},
	"low":     {},
	"medium":  {},
	"high":    {},
	"xhigh":   {},
}

// BuildResponse builds a Codex client model response from available models.
func BuildResponse(availableModels []map[string]any, providersForModel ProvidersForModelFunc, optimizeMultiAgentV2 bool) map[string]any {
	return BuildResponseForClient(availableModels, providersForModel, optimizeMultiAgentV2, "")
}

// BuildResponseForClient builds a Codex client model response from available models
// tailored for a specific client version.
func BuildResponseForClient(availableModels []map[string]any, providersForModel ProvidersForModelFunc, optimizeMultiAgentV2 bool, clientVersion string) map[string]any {
	return BuildResponseForClientWithCPACapabilities(availableModels, providersForModel, registry.GetGlobalRegistry().GetResponsesWebSearchCapability, optimizeMultiAgentV2, clientVersion)
}

// BuildResponseForClientWithCPACapabilities builds a client response while
// allowing Home to supply capability metadata independent of the local registry.
func BuildResponseForClientWithCPACapabilities(availableModels []map[string]any, providersForModel ProvidersForModelFunc, webSearchCapabilityForModel WebSearchCapabilityForModelFunc, optimizeMultiAgentV2 bool, clientVersion string, openAIPlanLabel ...string) map[string]any {
	return BuildResponseForClientWithToolCapabilities(availableModels, providersForModel, webSearchCapabilityForModel, nil, nil, optimizeMultiAgentV2, clientVersion, openAIPlanLabel...)
}

// BuildResponseForClientWithToolCapabilities adds executor-backed tool support
// independently of template metadata and legacy provider restrictions.
func BuildResponseForClientWithToolCapabilities(availableModels []map[string]any, providersForModel ProvidersForModelFunc, webSearchCapabilityForModel WebSearchCapabilityForModelFunc, applyPatchCapabilityForModel ApplyPatchCapabilityForModelFunc, toolSearchCapabilityForModel ToolSearchCapabilityForModelFunc, optimizeMultiAgentV2 bool, clientVersion string, openAIPlanLabel ...string) map[string]any {
	return BuildResponseForClientWithSearchToolRoutes(availableModels, providersForModel, nil, webSearchCapabilityForModel, applyPatchCapabilityForModel, toolSearchCapabilityForModel, optimizeMultiAgentV2, clientVersion, openAIPlanLabel...)
}

// BuildResponseForClientWithSearchToolRoutes lets Home supply routing evidence
// for supports_search_tool without the other provider-dependent metadata.
// A nil searchToolProvidersForModel uses providersForModel. A nil
// toolSearchCapabilityForModel keeps Claude routes from advertising search.
func BuildResponseForClientWithSearchToolRoutes(availableModels []map[string]any, providersForModel, searchToolProvidersForModel ProvidersForModelFunc, webSearchCapabilityForModel WebSearchCapabilityForModelFunc, applyPatchCapabilityForModel ApplyPatchCapabilityForModelFunc, toolSearchCapabilityForModel ToolSearchCapabilityForModelFunc, optimizeMultiAgentV2 bool, clientVersion string, openAIPlanLabel ...string) map[string]any {
	planLabel := ""
	if len(openAIPlanLabel) > 0 {
		planLabel = strings.TrimSpace(openAIPlanLabel[0])
	}
	return map[string]any{
		"models": buildCodexClientModelsWithToolCapabilities(availableModels, providersForModel, searchToolProvidersForModel, webSearchCapabilityForModel, applyPatchCapabilityForModel, toolSearchCapabilityForModel, optimizeMultiAgentV2, clientVersion, planLabel),
	}
}

// MarshalCompact serializes a Codex client catalog as a single JSON line.
// HTML escaping is disabled so instruction text is not expanded into \u003c sequences.
func MarshalCompact(payload any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if errEncode := encoder.Encode(payload); errEncode != nil {
		return nil, errEncode
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func buildCodexClientModels(models []map[string]any, providersForModel ProvidersForModelFunc, webSearchCapabilityForModel WebSearchCapabilityForModelFunc, optimizeMultiAgentV2 bool, clientVersion string) []map[string]any {
	return buildCodexClientModelsWithToolCapabilities(models, providersForModel, nil, webSearchCapabilityForModel, nil, nil, optimizeMultiAgentV2, clientVersion, "")
}

func buildCodexClientModelsWithToolCapabilities(models []map[string]any, providersForModel, searchToolProvidersForModel ProvidersForModelFunc, webSearchCapabilityForModel WebSearchCapabilityForModelFunc, applyPatchCapabilityForModel ApplyPatchCapabilityForModelFunc, toolSearchCapabilityForModel ToolSearchCapabilityForModelFunc, optimizeMultiAgentV2 bool, clientVersion, openAIPlanLabel string) []map[string]any {
	templates, defaultTemplate, errLoadCodexClientModelTemplates := loadCodexClientModelTemplates()
	if errLoadCodexClientModelTemplates != nil || defaultTemplate == nil {
		return nil
	}
	if searchToolProvidersForModel == nil {
		searchToolProvidersForModel = providersForModel
	}

	modelIDs := make(map[string]struct{}, len(models))
	for _, model := range models {
		if id := strings.TrimSpace(stringModelValue(model, "id")); id != "" {
			modelIDs[id] = struct{}{}
		}
	}

	result := make([]map[string]any, 0, len(models))
	for _, model := range models {
		id := strings.TrimSpace(stringModelValue(model, "id"))
		if id == "" {
			continue
		}

		metadataID := codexClientMetadataModelID(id)
		hideFromPicker := isCodexGPTBelow56(id)
		if hideCodexModel(id, modelIDs) && !hideFromPicker {
			continue
		}

		if template, ok := templates[metadataID]; ok {
			entry := cloneCodexClientModelMap(template)
			entry["slug"] = id
			info := registry.LookupModelInfo(id)
			applyCodexClientModelCapabilities(entry, id, metadataID, info, providersForModel, clientVersion)
			applyCodexClientDisplayName(entry, model, id, providersForModel, openAIPlanLabel)
			applyCodexClientDescription(entry, model)
			applyCodexClientBaseInstructions(entry, model)
			applyCodexClientMaxContextLengthOverride(entry, model)
			applyCodexClientMaxTokens(entry, model)
			if thinkingSupport := codexClientThinkingSupport(model); thinkingSupport != nil {
				applyCodexClientThinkingMetadata(entry, thinkingSupport, clientVersion)
			}
			applyCodexClientProviderCapabilities(entry, id, providersForModel)
			applyCodexClientSearchToolSupport(entry, id, true, searchToolProvidersForModel, toolSearchCapabilityForModel)
			applyCPAWebSearchCapability(entry, id, webSearchCapabilityForModel, clientVersion)
			sanitizeCodexClientReasoningMetadata(entry, clientVersion)
			applyCodexClientVisibilityOverride(entry, id)
			if hideFromPicker {
				entry["visibility"] = "hide"
			}
			if optimizeMultiAgentV2 {
				entry["multi_agent_version"] = "v2"
			}
			applyCodexClientDevinDisplayName(entry, id, model, providersForModel)
			applyCodexClientApplyPatchCapability(entry, id, applyPatchCapabilityForModel)
			result = append(result, entry)
			continue
		}

		entry := cloneCodexClientModelMap(defaultTemplate)
		applyCodexClientModelMetadata(entry, id, model, optimizeMultiAgentV2, clientVersion)
		applyCodexClientDisplayName(entry, model, id, providersForModel, openAIPlanLabel)
		applyCodexClientMaxTokens(entry, model)
		applyCodexClientSearchToolSupport(entry, id, false, searchToolProvidersForModel, toolSearchCapabilityForModel)
		applyCPAWebSearchCapability(entry, id, webSearchCapabilityForModel, clientVersion)
		sanitizeCodexClientReasoningMetadata(entry, clientVersion)
		applyCodexClientVisibilityOverride(entry, id)
		if hideFromPicker {
			entry["visibility"] = "hide"
		}
		applyCodexClientDevinDisplayName(entry, id, model, providersForModel)
		applyCodexClientApplyPatchCapability(entry, id, applyPatchCapabilityForModel)
		result = append(result, entry)
	}

	applyCodexClientNonTemplatePriorities(result, templates)

	sort.SliceStable(result, func(i, j int) bool {
		return codexClientModelPriority(result[i]) < codexClientModelPriority(result[j])
	})

	return result
}

func maxCodexClientTemplatePriority(templates map[string]map[string]any) int {
	maxPriority := 0
	for _, template := range templates {
		priority := codexClientModelPriority(template)
		if priority > maxPriority {
			maxPriority = priority
		}
	}
	return maxPriority
}

func applyCodexClientNonTemplatePriorities(result []map[string]any, templates map[string]map[string]any) {
	if len(result) == 0 {
		return
	}

	basePriority := maxCodexClientTemplatePriority(templates)
	type nonTemplateEntry struct {
		index       int
		displayName string
		slug        string
	}

	pending := make([]nonTemplateEntry, 0)
	for index, entry := range result {
		slug := stringModelValue(entry, "slug")
		if _, ok := templates[codexClientMetadataModelID(slug)]; ok {
			continue
		}
		displayName := stringModelValue(entry, "display_name")
		if displayName == "" {
			displayName = slug
		}
		pending = append(pending, nonTemplateEntry{
			index:       index,
			displayName: displayName,
			slug:        slug,
		})
	}

	sort.SliceStable(pending, func(i, j int) bool {
		leftPinned := pending[i].slug == "devin/swe-2"
		rightPinned := pending[j].slug == "devin/swe-2"
		if leftPinned != rightPinned {
			return leftPinned
		}
		left := strings.ToLower(pending[i].displayName)
		right := strings.ToLower(pending[j].displayName)
		if left == right {
			return pending[i].slug < pending[j].slug
		}
		return left < right
	})

	for rank, entry := range pending {
		result[entry.index]["priority"] = basePriority + 100*(rank+1)
	}
}

func loadCodexClientModelTemplates() (map[string]map[string]any, map[string]any, error) {
	raw, revision := registry.GetCodexClientModelsSnapshot()
	return loadCodexClientModelTemplatesSnapshot(raw, revision)
}

func loadCodexClientModelTemplatesSnapshot(raw []byte, revision uint64) (map[string]map[string]any, map[string]any, error) {
	codexClientModelTemplatesMu.Lock()
	defer codexClientModelTemplatesMu.Unlock()
	if codexClientModelTemplatesLoaded && codexClientModelTemplatesRevision == revision {
		return codexClientModelTemplates, codexClientDefaultTemplate, codexClientModelTemplatesErr
	}

	var payload codexClientModelsPayload
	err := json.Unmarshal(raw, &payload)
	var templates map[string]map[string]any
	var defaultTemplate map[string]any
	if err == nil {
		templates = make(map[string]map[string]any, len(payload.Models))
		for _, model := range payload.Models {
			slug := strings.TrimSpace(stringModelValue(model, "slug"))
			if slug == "" {
				continue
			}
			templates[slug] = cloneCodexClientModelMap(model)
			if defaultTemplate == nil || slug == "gpt-5.5" {
				defaultTemplate = cloneCodexClientModelMap(model)
			}
		}
	}

	codexClientModelTemplatesLoaded = true
	codexClientModelTemplatesRevision = revision
	codexClientModelTemplates = templates
	codexClientDefaultTemplate = defaultTemplate
	codexClientModelTemplatesErr = err
	return codexClientModelTemplates, codexClientDefaultTemplate, codexClientModelTemplatesErr
}

func codexClientMetadataModelID(id string) string {
	id = strings.TrimSpace(id)
	if info := registry.LookupModelInfo(id); info != nil {
		if metadataID := strings.TrimSpace(info.MetadataModelID); metadataID != "" {
			return metadataID
		}
	}
	if idx := strings.Index(id, "/"); idx != -1 {
		base := strings.TrimSpace(id[idx+1:])
		if info := registry.LookupModelInfo(base); info != nil && strings.TrimSpace(info.MetadataModelID) != "" {
			return strings.TrimSpace(info.MetadataModelID)
		}
		return base
	}
	return id
}

func applyCodexClientDisplayName(entry map[string]any, model map[string]any, id string, providersForModel ProvidersForModelFunc, openAIPlanLabel string) {
	displayName := stringModelValue(model, "display_name")
	if displayName == "" {
		displayName = stringModelValue(entry, "display_name")
	}
	displayName = cleanCodexClientDisplayName(displayName)
	if displayName == "" {
		displayName = id
	}
	provider := codexClientProviderLabel(id, model, providersForModel, openAIPlanLabel)
	if provider != "" && !strings.HasSuffix(strings.ToLower(displayName), "("+strings.ToLower(provider)+")") {
		displayName += " (" + provider + ")"
	}
	entry["display_name"] = displayName
}

func cleanCodexClientDisplayName(displayName string) string {
	displayName = strings.TrimSpace(displayName)
	for _, prefix := range []string{"CPA OCGo ", "CPA OpenCode Go ", "CPA OpenRouter ", "CPA Baseten ", "CPA Fireworks ", "CPA Antigravity ", "CPA "} {
		if strings.HasPrefix(displayName, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(displayName, prefix))
		}
	}
	return displayName
}

func codexClientProviderLabel(id string, model map[string]any, providersForModel ProvidersForModelFunc, openAIPlanLabel string) string {
	idLower := strings.ToLower(strings.TrimSpace(id))
	if isCodexClientDevinModel(id, model, nil, providersForModel) {
		return "Devin"
	}
	switch {
	case strings.HasPrefix(idLower, "cpa-ocgo-"):
		return "OpenCode Go"
	case strings.HasPrefix(idLower, "cpa-or-"):
		return "OpenRouter"
	case strings.HasPrefix(idLower, "cpa-bt-"):
		return "Baseten"
	case strings.HasPrefix(idLower, "cpa-fw-"):
		return "Fireworks"
	case strings.HasPrefix(idLower, "cpa-ag-"):
		return "Antigravity"
	case strings.HasPrefix(idLower, "devin/"):
		return "Devin"
	}

	var providers []string
	if providersForModel != nil {
		providers = providersForModel(id)
		if len(providers) == 0 && strings.Contains(id, "/") {
			providers = providersForModel(strings.TrimSpace(id[strings.Index(id, "/")+1:]))
		}
	}
	for _, rawProvider := range providers {
		provider := strings.ToLower(strings.TrimSpace(rawProvider))
		switch provider {
		case "codex":
			if openAIPlanLabel != "" {
				return openAIPlanLabel
			}
			return "OpenAI OAuth"
		case "antigravity":
			return "Antigravity"
		case "devin":
			return "Devin"
		case "gemini":
			return "Gemini API"
		case "claude":
			return "Claude"
		case "xai":
			return "xAI"
		case "openai":
			if openAIPlanLabel != "" {
				return openAIPlanLabel
			}
			return "OpenAI"
		case "openai-compatibility":
			return "OpenAI-compatible"
		}
		if provider != "" {
			return titleCodexProvider(provider)
		}
	}

	ownedBy := strings.ToLower(strings.TrimSpace(stringModelValue(model, "owned_by")))
	if ownedBy == "openai" && codexGPTVersionPattern.MatchString(id) {
		if openAIPlanLabel != "" {
			return openAIPlanLabel
		}
		return "OpenAI OAuth"
	}
	return ""
}

func titleCodexProvider(provider string) string {
	parts := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(provider))
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func hideCodexModel(id string, modelIDs map[string]struct{}) bool {
	idLower := strings.ToLower(strings.TrimSpace(id))
	if isCodexGPTBelow56(id) {
		return true
	}
	if codexGeminiPattern.MatchString(idLower) && !codexGemini38Pattern.MatchString(idLower) {
		return true
	}
	if codexClaudePattern.MatchString(idLower) && !codexAllowedClaudePattern.MatchString(idLower) {
		return true
	}
	if strings.HasPrefix(idLower, "cpa-ag-") && !codexGemini38Pattern.MatchString(idLower) {
		return true
	}
	if strings.HasPrefix(idLower, "devin/") && idLower != "devin/swe-2" && idLower != "devin/deepseek-v4-1-flash" {
		return true
	}
	if strings.Contains(idLower, "nemotron") || strings.Contains(idLower, "muse-spark-1.2") || strings.Contains(idLower, "qwen") {
		return true
	}
	switch idLower {
	case "cpa-ocgo-longcat-2.0", "cpa-ocgo-mimo-v2.5", "cpa-ocgo-mimo-v2.5-pro", "cpa-ocgo-minimax-m2.5", "cpa-ocgo-minimax-m3":
		return true
	}
	if strings.Contains(idLower, "glm-") && !codexAllowedGLMPattern.MatchString(idLower) {
		return true
	}
	if strings.Contains(idLower, "kimi-") && !codexAllowedKimiPattern.MatchString(idLower) {
		return true
	}
	if strings.HasPrefix(idLower, "cpa-ocgo-gpt-5.6-luna") {
		return true
	}
	if strings.Contains(idLower, "deepseek-") && !codexAllowedDeepSeekPattern.MatchString(idLower) {
		return true
	}
	if strings.HasPrefix(idLower, "cpa-codex-") {
		base := strings.TrimPrefix(id, "cpa-codex-")
		if _, exists := modelIDs[base]; exists {
			return true
		}
		if open := strings.LastIndex(base, "("); open > 0 && strings.HasSuffix(base, ")") {
			base = strings.TrimSpace(base[:open])
			if _, exists := modelIDs[base]; exists {
				return true
			}
		}
	}
	if strings.HasPrefix(idLower, "cpa-claude-") {
		base := strings.TrimPrefix(idLower, "cpa-")
		if _, exists := modelIDs[base]; exists {
			return true
		}
		normalizedBase := strings.ReplaceAll(base, ".", "-")
		for modelID := range modelIDs {
			if strings.ReplaceAll(strings.ToLower(modelID), ".", "-") == normalizedBase {
				return true
			}
		}
	}
	if strings.HasPrefix(idLower, "gemini-") {
		if _, exists := modelIDs["cpa-ag-"+id]; exists {
			return true
		}
	}
	if strings.HasPrefix(idLower, "cpa-ag-gemini-") && strings.HasSuffix(idLower, "-high") {
		if _, exists := modelIDs[strings.TrimSuffix(id, "-high")]; exists {
			return true
		}
	}

	open := strings.LastIndex(id, "(")
	if open < 1 || !strings.HasSuffix(id, ")") {
		return false
	}
	base := strings.TrimSpace(id[:open])
	suffix := strings.ToLower(strings.TrimSpace(id[open+1 : len(id)-1]))
	if _, hasBase := modelIDs[base]; hasBase && isReasoningEffortAlias(suffix) {
		return true
	}
	if strings.HasPrefix(suffix, "fast-") {
		if _, hasFastBase := modelIDs[base+"(fast)"]; hasFastBase {
			return true
		}
	}
	return false
}

func isReasoningEffortAlias(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return true
	default:
		return false
	}
}

func isCodexGPTBelow56(modelID string) bool {
	match := codexGPTVersionPattern.FindStringSubmatch(modelID)
	if len(match) < 2 {
		return false
	}
	major, errMajor := strconv.Atoi(match[1])
	if errMajor != nil {
		return false
	}
	minor := 0
	if len(match) > 2 && match[2] != "" {
		minor, _ = strconv.Atoi(match[2])
	}
	return major < 5 || major == 5 && minor < 6
}

func applyCodexClientDevinDisplayName(entry map[string]any, id string, model map[string]any, providersForModel ProvidersForModelFunc) {
	if !isCodexClientDevinModel(id, model, entry, providersForModel) {
		return
	}
	displayName := stringModelValue(entry, "display_name")
	if displayName == "" {
		displayName = id
	}
	trimmed := strings.TrimSpace(displayName)
	if strings.HasSuffix(trimmed, " (Devin)") {
		return
	}
	if strings.HasSuffix(strings.ToLower(trimmed), " (devin)") {
		entry["display_name"] = trimmed[:len(trimmed)-len(" (devin)")] + " (Devin)"
		return
	}
	if strings.HasSuffix(strings.ToLower(trimmed), "(devin)") {
		entry["display_name"] = strings.TrimSpace(trimmed[:len(trimmed)-len("(devin)")]) + " (Devin)"
		return
	}
	entry["display_name"] = trimmed + " (Devin)"
}

func isCodexClientDevinModel(id string, model map[string]any, entry map[string]any, providersForModel ProvidersForModelFunc) bool {
	idLower := strings.ToLower(strings.TrimSpace(id))
	if strings.HasPrefix(idLower, "devin/") {
		return true
	}
	if idx := strings.Index(idLower, "/"); idx != -1 {
		rest := idLower[idx+1:]
		if strings.HasPrefix(rest, "devin/") {
			return true
		}
	}
	if entry != nil {
		slugLower := strings.ToLower(strings.TrimSpace(stringModelValue(entry, "slug")))
		if strings.HasPrefix(slugLower, "devin/") {
			return true
		}
		if idx := strings.Index(slugLower, "/"); idx != -1 {
			rest := slugLower[idx+1:]
			if strings.HasPrefix(rest, "devin/") {
				return true
			}
		}
		if strings.EqualFold(strings.TrimSpace(stringModelValue(entry, "type")), "devin") {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(stringModelValue(entry, "owned_by")), "cognition") {
			return true
		}
	}
	if model != nil {
		if strings.EqualFold(strings.TrimSpace(stringModelValue(model, "type")), "devin") {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(stringModelValue(model, "owned_by")), "cognition") {
			return true
		}
	}
	if info := registry.LookupModelInfo(id); info != nil {
		if strings.EqualFold(info.Type, "devin") || strings.EqualFold(info.OwnedBy, "cognition") || strings.HasPrefix(strings.ToLower(info.ID), "devin/") {
			return true
		}
	} else if idx := strings.Index(id, "/"); idx != -1 {
		base := strings.TrimSpace(id[idx+1:])
		if info := registry.LookupModelInfo(base); info != nil {
			if strings.EqualFold(info.Type, "devin") || strings.EqualFold(info.OwnedBy, "cognition") || strings.HasPrefix(strings.ToLower(info.ID), "devin/") {
				return true
			}
		}
	}
	if providersForModel != nil {
		providers := providersForModel(id)
		if len(providers) == 0 && strings.Contains(id, "/") {
			idx := strings.Index(id, "/")
			base := strings.TrimSpace(id[idx+1:])
			providers = providersForModel(base)
		}
		for _, p := range providers {
			if strings.EqualFold(strings.TrimSpace(p), "devin") {
				return true
			}
		}
	}
	return false
}

func applyCodexClientDescription(entry map[string]any, model map[string]any) {
	if description := stringModelValue(model, "description"); description != "" {
		entry["description"] = description
	}
}

func applyCodexClientBaseInstructions(entry map[string]any, model map[string]any) {
	if baseInstructions := stringModelValue(model, "base_instructions"); baseInstructions != "" {
		entry["base_instructions"] = baseInstructions
	}
}

func applyCodexClientModelCapabilities(entry map[string]any, id, metadataID string, info *registry.ModelInfo, providersForModel ProvidersForModelFunc, clientVersion string) {
	if info != nil && info.Type == registry.OpenAIImageModelType {
		entry["visibility"] = "hide"
		delete(entry, "input_modalities")
		delete(entry, "supports_image_detail_original")
		return
	}

	var providers []string
	if providersForModel != nil {
		providers = providersForModel(id)
		if len(providers) == 0 && strings.Contains(id, "/") {
			idx := strings.Index(id, "/")
			base := strings.TrimSpace(id[idx+1:])
			providers = providersForModel(base)
		}
	}

	isAlias := metadataID != "" && !strings.EqualFold(id, metadataID)

	var constrainedModalities []string
	hasModalitiesConstraint := false
	for _, p := range providers {
		pInfo := registry.LookupModelInfo(id, p)
		if pInfo == nil && strings.Contains(id, "/") {
			idx := strings.Index(id, "/")
			pInfo = registry.LookupModelInfo(strings.TrimSpace(id[idx+1:]), p)
		}
		if pInfo == nil {
			continue
		}
		isCodex := strings.EqualFold(strings.TrimSpace(p), "codex")
		if (!isCodex && isAlias) || pInfo.ExplicitInputModalities {
			mods := pInfo.SupportedInputModalities
			if mods == nil {
				mods = []string{}
			}
			if !hasModalitiesConstraint {
				constrainedModalities = append([]string(nil), mods...)
				hasModalitiesConstraint = true
			} else {
				constrainedModalities = intersectStringSlices(constrainedModalities, mods)
			}
		}
	}
	if !hasModalitiesConstraint && info != nil && info.ExplicitInputModalities {
		mods := info.SupportedInputModalities
		if mods == nil {
			mods = []string{}
		}
		constrainedModalities = append([]string(nil), mods...)
		hasModalitiesConstraint = true
	}
	if hasModalitiesConstraint {
		codexModalities := filterCodexInputModalities(constrainedModalities)
		entry["input_modalities"] = codexModalities
		if hasImageModality(codexModalities) {
			entry["supports_image_detail_original"] = true
		} else {
			delete(entry, "supports_image_detail_original")
		}
	}

	var constrainedThinking *registry.ThinkingSupport
	hasThinkingConstraint := false
	for _, p := range providers {
		pInfo := registry.LookupModelInfo(id, p)
		if pInfo == nil && strings.Contains(id, "/") {
			idx := strings.Index(id, "/")
			pInfo = registry.LookupModelInfo(strings.TrimSpace(id[idx+1:]), p)
		}
		if pInfo == nil {
			continue
		}
		isCodex := strings.EqualFold(strings.TrimSpace(p), "codex")
		if (!isCodex && isAlias) || pInfo.ExplicitThinking {
			hasThinkingConstraint = true
			pThinking := pInfo.Thinking
			if pThinking == nil {
				pThinking = &registry.ThinkingSupport{Levels: []string{}}
			}
			if constrainedThinking == nil {
				constrainedThinking = pThinking
			} else {
				constrainedThinking = intersectThinkingSupport(constrainedThinking, pThinking)
			}
		}
	}
	if !hasThinkingConstraint && info != nil && info.ExplicitThinking {
		pThinking := info.Thinking
		if pThinking == nil {
			pThinking = &registry.ThinkingSupport{Levels: []string{}}
		}
		constrainedThinking = pThinking
		hasThinkingConstraint = true
	}
	if hasThinkingConstraint && constrainedThinking != nil {
		applyCodexClientThinkingMetadata(entry, constrainedThinking, clientVersion)
	}
}

func intersectThinkingSupport(a, b *registry.ThinkingSupport) *registry.ThinkingSupport {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	levels := intersectStringSlices(a.Levels, b.Levels)
	min := a.Min
	if b.Min > min {
		min = b.Min
	}
	max := a.Max
	if b.Max > 0 && (max == 0 || b.Max < max) {
		max = b.Max
	}
	return &registry.ThinkingSupport{
		Min:            min,
		Max:            max,
		ZeroAllowed:    a.ZeroAllowed && b.ZeroAllowed,
		DynamicAllowed: a.DynamicAllowed && b.DynamicAllowed,
		Levels:         levels,
	}
}

func intersectStringSlices(a, b []string) []string {
	if len(a) == 0 || len(b) == 0 {
		return []string{}
	}
	bMap := make(map[string]struct{}, len(b))
	for _, item := range b {
		bMap[strings.ToLower(strings.TrimSpace(item))] = struct{}{}
	}
	out := make([]string, 0, len(a))
	seen := make(map[string]struct{}, len(a))
	for _, item := range a {
		key := strings.ToLower(strings.TrimSpace(item))
		if _, inB := bMap[key]; inB {
			if _, inSeen := seen[key]; !inSeen {
				seen[key] = struct{}{}
				out = append(out, item)
			}
		}
	}
	return out
}

func applyCodexClientMaxContextLengthOverride(entry map[string]any, model map[string]any) {
	if maxContextLength := intModelValue(model, "max_context_length"); maxContextLength > 0 {
		entry["context_window"] = maxContextLength
		entry["max_context_window"] = maxContextLength
	}
}

func applyCodexClientMaxTokens(entry map[string]any, model map[string]any) {
	if maxCompletionTokens := intModelValue(model, "max_completion_tokens"); maxCompletionTokens > 0 {
		entry["max_tokens"] = maxCompletionTokens
	}
}

func applyCPAWebSearchCapability(entry map[string]any, id string, capabilityForModel WebSearchCapabilityForModelFunc, clientVersion string) {
	// Templates must not supply runtime capability claims or leak CPA-only fields.
	delete(entry, "cpa_capabilities")
	if clientVersion != "cpa" || capabilityForModel == nil {
		return
	}
	if supported := capabilityForModel(strings.TrimSpace(id)); supported != nil {
		entry["cpa_capabilities"] = map[string]any{"web_search": *supported}
	}
}

func applyCodexClientProviderCapabilities(entry map[string]any, id string, providersForModel ProvidersForModelFunc) {
	if providersForModel != nil && !isPureCodexProvider(id, providersForModel) {
		entry["prefer_websockets"] = false
		entry["service_tiers"] = []any{}
		nullCodexClientRequiredOptions(entry)
	}
}

// nullCodexClientRequiredOptions clears capabilities that non-Codex models must not advertise.
// Codex requires these keys to be present; omitting them rejects the entire catalog.
func nullCodexClientRequiredOptions(entry map[string]any) {
	entry["apply_patch_tool_type"] = nil
	entry["tool_mode"] = "direct"
	entry["use_responses_lite"] = false
	entry["supports_reasoning_effort_updates"] = false
	entry["upgrade"] = nil
	entry["availability_nux"] = nil
}

const codexClientFallbackInstructions = "You are Codex, a coding agent. You and the user share one workspace."

// useCompactCodexClientInstructions replaces cloned template prompts with a short literal.
// Both the legacy and canonical instruction fields must be present for catalog decoding.
func useCompactCodexClientInstructions(entry map[string]any) {
	entry["base_instructions"] = codexClientFallbackInstructions
	entry["model_messages"] = map[string]any{
		"instructions_template":  codexClientFallbackInstructions,
		"instructions_variables": nil,
		"approvals":              nil,
		"collaboration_modes":    nil,
		"auto_review":            nil,
		"permissions":            nil,
		"multi_agent":            nil,
	}
}

func isPureCodexProvider(id string, providersForModel ProvidersForModelFunc) bool {
	if providersForModel == nil {
		return true
	}
	return allCodexClientProvidersAre(codexClientModelProviders(id, providersForModel), "codex")
}

// codexClientModelProviders resolves the providers serving a model, falling
// back to the base ID for prefixed aliases.
func codexClientModelProviders(id string, providersForModel ProvidersForModelFunc) []string {
	providers := providersForModel(id)
	if len(providers) == 0 && strings.Contains(id, "/") {
		idx := strings.Index(id, "/")
		base := strings.TrimSpace(id[idx+1:])
		providers = providersForModel(base)
	}
	return providers
}

// allCodexClientProvidersAre reports whether providers is non-empty and every
// entry is the named provider.
func allCodexClientProvidersAre(providers []string, name string) bool {
	if len(providers) == 0 {
		return false
	}
	for _, provider := range providers {
		if !strings.EqualFold(strings.TrimSpace(provider), name) {
			return false
		}
	}
	return true
}

// applyCodexClientSearchToolSupport keeps supports_search_tool only when every
// route serving the model can close Codex's client tool_search loop: pure Codex
// template models, or pure Claude models whose every credential supports
// Anthropic native tool search. Mixed and unknown routes fail closed.
func applyCodexClientSearchToolSupport(entry map[string]any, id string, templateModel bool, providersForModel ProvidersForModelFunc, toolSearchCapabilityForModel ToolSearchCapabilityForModelFunc) {
	supportsSearch, _ := entry["supports_search_tool"].(bool)
	if !supportsSearch {
		return
	}

	if providersForModel == nil {
		if !templateModel {
			entry["supports_search_tool"] = false
		}
		return
	}

	providers := codexClientModelProviders(id, providersForModel)
	if allCodexClientProvidersAre(providers, "claude") && toolSearchCapabilityForModel != nil && toolSearchCapabilityForModel(id) {
		return
	}
	if !templateModel || !allCodexClientProvidersAre(providers, "codex") {
		entry["supports_search_tool"] = false
	}
}

func applyCodexClientModelMetadata(entry map[string]any, id string, model map[string]any, optimizeMultiAgentV2 bool, clientVersion string) {
	info := registry.LookupModelInfo(id)

	displayName := stringModelValue(model, "display_name")
	description := stringModelValue(model, "description")
	contextWindow := intModelValue(model, "context_length")
	thinkingSupport := codexClientThinkingSupport(model)

	if info != nil {
		if info.DisplayName != "" {
			displayName = info.DisplayName
		}
		if info.Description != "" {
			description = info.Description
		}
		if contextWindow <= 0 && info.ContextLength > 0 {
			contextWindow = info.ContextLength
		}
		if info.Type == registry.OpenAIImageModelType {
			entry["visibility"] = "hide"
			delete(entry, "input_modalities")
			delete(entry, "supports_image_detail_original")
		} else {
			applyCodexClientInputModalitiesMetadata(entry, info.SupportedInputModalities)
		}
		if thinkingSupport == nil {
			thinkingSupport = info.Thinking
		}
	}
	applyCodexClientThinkingMetadata(entry, thinkingSupport, clientVersion)

	if maxContextWindow := intModelValue(model, "max_context_length"); maxContextWindow > 0 {
		contextWindow = maxContextWindow
	}

	if displayName == "" {
		displayName = id
	}
	if description == "" {
		description = id
	}

	entry["slug"] = id
	entry["display_name"] = displayName
	entry["description"] = description
	entry["prefer_websockets"] = false
	if optimizeMultiAgentV2 {
		entry["multi_agent_version"] = "v2"
	}
	entry["service_tiers"] = []any{}
	nullCodexClientRequiredOptions(entry)

	if contextWindow > 0 {
		entry["context_window"] = contextWindow
		entry["max_context_window"] = contextWindow
	}

	if plans, ok := model["available_in_plans"]; ok {
		entry["available_in_plans"] = cloneCodexClientModelValue(plans)
	}
	// Codex 0.156+ caps an explicit model_catalog_url body at 1MiB. Cloning the
	// full template instructions onto every non-template model exceeds that limit
	// and the client keeps its bundled catalog.
	useCompactCodexClientInstructions(entry)
}

func codexClientThinkingSupport(model map[string]any) *registry.ThinkingSupport {
	raw, ok := model["thinking"]
	if !ok || raw == nil {
		return nil
	}
	switch thinking := raw.(type) {
	case *registry.ThinkingSupport:
		return thinking
	case registry.ThinkingSupport:
		return &thinking
	}
	data, errMarshal := json.Marshal(raw)
	if errMarshal != nil {
		return nil
	}
	var thinking registry.ThinkingSupport
	if errUnmarshal := json.Unmarshal(data, &thinking); errUnmarshal != nil {
		return nil
	}
	return &thinking
}

func applyCodexClientVisibilityOverride(entry map[string]any, id string) {
	if isCodexClientImageOrVideoModel(id) {
		entry["visibility"] = "hide"
	}
}

// isCodexClientImageOrVideoModel reports catalog IDs that are not chat models.
// Speech IDs are included so the Codex client does not offer them as text models.
func isCodexClientImageOrVideoModel(id string) bool {
	target := strings.TrimSpace(id)
	if idx := strings.Index(target, "/"); idx != -1 {
		target = strings.TrimSpace(target[idx+1:])
	}
	switch target {
	case "grok-imagine-image-quality", "gpt-image-1.5", "gpt-image-2", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst", "gpt-image-2.5", "grok-imagine-image", "grok-imagine-image-2.0", "grok-imagine-video", "grok-imagine-video-1.5", "grok-imagine-video-1.5-preview", "grok-tts", "grok-voice-tts-1.0":
		return true
	default:
		return false
	}
}

func applyCodexClientInputModalitiesMetadata(entry map[string]any, modalities []string) {
	if len(modalities) == 0 {
		return
	}
	codexModalities := filterCodexInputModalities(modalities)
	if len(codexModalities) == 0 {
		return
	}
	entry["input_modalities"] = codexModalities
	if hasImageModality(codexModalities) {
		entry["supports_image_detail_original"] = true
	} else {
		delete(entry, "supports_image_detail_original")
	}
}

func filterCodexInputModalities(modalities []string) []any {
	if len(modalities) == 0 {
		return []any{}
	}
	codexModalities := make([]any, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, raw := range modalities {
		switch modality := strings.ToLower(strings.TrimSpace(raw)); modality {
		case "text", "image":
			if _, ok := seen[modality]; ok {
				continue
			}
			seen[modality] = struct{}{}
			codexModalities = append(codexModalities, modality)
		}
	}
	return codexModalities
}

func hasImageModality(modalities []any) bool {
	for _, m := range modalities {
		if s, ok := m.(string); ok && s == "image" {
			return true
		}
	}
	return false
}

func applyCodexClientThinkingMetadata(entry map[string]any, thinking *registry.ThinkingSupport, clientVersion string) {
	if thinking == nil {
		return
	}

	levels := make([]any, 0, len(thinking.Levels))
	defaultLevel := ""
	firstLevel := ""
	for _, rawLevel := range thinking.Levels {
		level := normalizeCodexClientReasoningLevel(rawLevel, clientVersion)
		if level == "" {
			continue
		}
		if firstLevel == "" {
			firstLevel = level
		}
		if (defaultLevel == "" && level != "none") || level == "medium" {
			defaultLevel = level
		}
		levels = append(levels, map[string]any{
			"effort":      level,
			"description": codexClientReasoningDescription(level),
		})
	}
	if len(levels) == 0 {
		entry["supported_reasoning_levels"] = levels
		delete(entry, "default_reasoning_level")
		return
	}
	if defaultLevel == "" {
		defaultLevel = firstLevel
	}

	entry["supported_reasoning_levels"] = levels
	entry["default_reasoning_level"] = defaultLevel
}

func sanitizeCodexClientReasoningMetadata(entry map[string]any, clientVersion string) {
	rawLevels, ok := entry["supported_reasoning_levels"].([]any)
	if !ok {
		return
	}

	levels := make([]any, 0, len(rawLevels))
	allowedDefaults := make(map[string]struct{}, len(rawLevels))
	for _, rawLevelEntry := range rawLevels {
		levelEntry, ok := rawLevelEntry.(map[string]any)
		if !ok {
			continue
		}
		level := normalizeCodexClientReasoningLevel(stringModelValue(levelEntry, "effort"), clientVersion)
		if level == "" {
			continue
		}
		clonedEntry := cloneCodexClientModelMap(levelEntry)
		clonedEntry["effort"] = level
		levels = append(levels, clonedEntry)
		allowedDefaults[level] = struct{}{}
	}

	if len(levels) == 0 {
		entry["supported_reasoning_levels"] = levels
		delete(entry, "default_reasoning_level")
		return
	}

	defaultLevel := normalizeCodexClientReasoningLevel(stringModelValue(entry, "default_reasoning_level"), clientVersion)
	if _, ok := allowedDefaults[defaultLevel]; !ok {
		defaultLevel = stringModelValue(levels[0].(map[string]any), "effort")
	}

	entry["supported_reasoning_levels"] = levels
	entry["default_reasoning_level"] = defaultLevel
}

func normalizeCodexClientReasoningLevel(rawLevel string, clientVersion string) string {
	level := strings.ToLower(strings.TrimSpace(rawLevel))
	if !supportsExtendedReasoningLevels(clientVersion) {
		if _, ok := codexClientLegacyAllowedReasoningLevels[level]; !ok {
			return ""
		}
		return level
	}
	if _, ok := codexClientAllowedReasoningLevels[level]; !ok {
		return ""
	}
	return level
}

// supportsExtendedReasoningLevels reports whether the given clientVersion supports
// extended reasoning effort levels ("max" and "ultra"), which require Codex CLI >= 0.144.0.
// If the version is empty or unparseable, it returns true to preserve modern features.
func supportsExtendedReasoningLevels(clientVersion string) bool {
	clientVersion = strings.TrimSpace(clientVersion)
	if clientVersion == "" {
		return true
	}
	cmp, ok := compareDottedVersions(clientVersion, "0.144.0")
	if !ok {
		return true
	}
	return cmp >= 0
}

func parseDottedVersion(version string) []int64 {
	version = strings.TrimSpace(version)
	if strings.HasPrefix(version, "v") || strings.HasPrefix(version, "V") {
		version = version[1:]
	}
	if idx := strings.IndexAny(version, "-+"); idx != -1 {
		version = version[:idx]
	}
	parts := strings.Split(version, ".")
	nums := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		num, errParse := strconv.ParseInt(part, 10, 64)
		if errParse != nil || num < 0 {
			return nil
		}
		nums = append(nums, num)
	}
	return nums
}

func compareDottedVersions(a, b string) (int, bool) {
	numsA := parseDottedVersion(a)
	numsB := parseDottedVersion(b)
	if len(numsA) == 0 || len(numsB) == 0 {
		return 0, false
	}
	maxLen := len(numsA)
	if len(numsB) > maxLen {
		maxLen = len(numsB)
	}
	for i := 0; i < maxLen; i++ {
		var valA, valB int64
		if i < len(numsA) {
			valA = numsA[i]
		}
		if i < len(numsB) {
			valB = numsB[i]
		}
		if valA < valB {
			return -1, true
		}
		if valA > valB {
			return 1, true
		}
	}
	return 0, true
}

func codexClientReasoningDescription(level string) string {
	switch level {
	case "none":
		return "No reasoning"
	case "minimal":
		return "Fastest responses with minimal reasoning"
	case "low":
		return "Fast responses with lighter reasoning"
	case "medium":
		return "Balances speed and reasoning depth for everyday tasks"
	case "high":
		return "Greater reasoning depth for complex problems"
	case "xhigh":
		return "Extra high reasoning depth for complex problems"
	case "max":
		return "Maximum available reasoning depth for complex problems"
	default:
		return level
	}
}

func codexClientModelPriority(model map[string]any) int {
	if priority, ok := model["priority"].(int); ok {
		return priority
	}
	if priority, ok := model["priority"].(float64); ok {
		return int(priority)
	}
	return 100
}

func stringModelValue(model map[string]any, key string) string {
	if model == nil {
		return ""
	}
	value, ok := model[key]
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func intModelValue(model map[string]any, key string) int {
	if model == nil {
		return 0
	}
	switch value := model[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func cloneCodexClientModelMap(model map[string]any) map[string]any {
	if model == nil {
		return nil
	}
	cloned := make(map[string]any, len(model))
	for key, value := range model {
		cloned[key] = cloneCodexClientModelValue(value)
	}
	return cloned
}

func cloneCodexClientModelValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneCodexClientModelMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, entry := range typed {
			cloned[i] = cloneCodexClientModelValue(entry)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
