package episodes

import (
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"strings"
)

// executorDocument is the spec-derived executor section of every request,
// with the provenance digests the worker and validator verify.
func (a *Assembler) executorDocument() (map[string]any, error) {
	executorDocument := map[string]any{
		"name":              a.spec.Cognition.Executor.Name,
		"model_policy":      a.spec.Cognition.Executor.ModelPolicy,
		"prompt_version":    a.spec.Cognition.Executor.PromptVersion,
		"prompt":            a.spec.Cognition.Executor.Prompt,
		"objective":         a.spec.Cognition.Executor.Objective,
		"decision_schema":   a.spec.Cognition.Executor.DecisionSchema,
		"diagnosis_catalog": a.spec.Cognition.Executor.DiagnosisCatalog,
	}
	promptDigest, err := canonicaljson.Digest(canonicaljson.DomainPrompt, map[string]any{
		"version": a.spec.Cognition.Executor.PromptVersion,
		"text":    a.spec.Cognition.Executor.Prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("digest prompt provenance: %w", err)
	}
	objectiveDigest, err := canonicaljson.Digest(canonicaljson.DomainObjective, map[string]any{"text": a.spec.Cognition.Executor.Objective})
	if err != nil {
		return nil, fmt.Errorf("digest objective provenance: %w", err)
	}
	// P1: the diagnosis catalog digest binds the catalog document the Ruby
	// worker verifies (shared situation-runtime/diagnosis-catalog domain).
	// The digest is over the PARSED catalog (the array shape), matching
	// DiagnosisCatalog.verify_wire — a wrapped-string shape would digest
	// differently and every Go-driven episode would fail closed in the Ruby
	// worker. An invalid catalog document fails compilation.
	var catalogValue any = []any{}
	if strings.TrimSpace(a.spec.Cognition.Executor.DiagnosisCatalog) != "" {
		if err := json.Unmarshal([]byte(a.spec.Cognition.Executor.DiagnosisCatalog), &catalogValue); err != nil {
			return nil, fmt.Errorf("diagnosis catalog is not valid JSON: %w", err)
		}
	}
	catalogDigest, err := canonicaljson.Digest(canonicaljson.DomainDiagnosisCatalog, catalogValue)
	if err != nil {
		return nil, fmt.Errorf("digest diagnosis catalog provenance: %w", err)
	}
	executorDocument["prompt_sha256"] = promptDigest
	executorDocument["objective_sha256"] = objectiveDigest
	executorDocument["diagnosis_catalog_sha256"] = catalogDigest

	// P4: the intent catalog is compiled from the spec and embedded with its
	// shared-domain digest — the Ruby worker verifies it via
	// IntentCatalog.verify_wire before any model call, and the Go validator
	// verifies it again independently (B10). A missing, empty, duplicate, or
	// structurally invalid catalog fails compilation.
	intentCatalog, intentCatalogDigest, err := CompileIntentCatalog(a.spec.Actions.Intents)
	if err != nil {
		return nil, fmt.Errorf("compile intent catalog: %w", err)
	}
	executorDocument["intent_catalog"] = intentCatalog
	executorDocument["intent_catalog_sha256"] = intentCatalogDigest

	// P5: the digest-pinned skill refs flow to the worker, which resolves the
	// text only from the operator-approved directory and requires the tree
	// digest to match (unknown name or mismatch fails before a model call).
	executorDocument["skill_refs"] = a.spec.Cognition.Executor.Skills
	return executorDocument, nil
}

func (a *Assembler) buildTools() []map[string]any {
	tools := make([]map[string]any, 0, len(a.spec.Cognition.Executor.Tools))
	for _, name := range a.spec.Cognition.Executor.Tools {
		tools = append(tools, map[string]any{
			"name":        name,
			"description": "bounded read-only evidence capability",
			"schema":      map[string]any{"type": "object", "additionalProperties": false},
		})
	}
	return tools
}

func (a *Assembler) allowedIntentTypes() map[string]bool {
	configured := make(map[string]bool)
	for _, intent := range a.spec.Actions.Intents {
		configured[intent.Type] = true
	}
	return configured
}

func (a *Assembler) allowedIntentTypeList() []string {
	allowed := a.allowedIntentTypes()
	result := make([]string, 0, len(allowed))
	seen := make(map[string]struct{}, len(allowed))
	for _, intent := range a.spec.Actions.Intents {
		if allowed[intent.Type] {
			if _, ok := seen[intent.Type]; ok {
				continue
			}
			seen[intent.Type] = struct{}{}
			result = append(result, intent.Type)
		}
	}
	return result
}

func (a *Assembler) effectiveRiskCeiling() string {
	ceiling := a.spec.Cognition.Executor.RiskCeiling
	if ceiling == "" {
		return "R1"
	}
	return ceiling
}

func (a *Assembler) budgetMap() map[string]any {
	b := a.spec.Cognition.Executor.Budget
	return map[string]any{
		"wall_time":               b.WallTime,
		"model_calls":             b.ModelCalls,
		"input_tokens":            b.InputTokens,
		"output_tokens":           b.OutputTokens,
		"tool_calls":              b.ToolCalls,
		"tool_result_bytes":       b.ToolResultBytes,
		"total_tool_result_bytes": b.TotalToolResultBytes,
		"provider_retries":        b.ProviderRetries,
		"cost_microunits":         b.CostMicrounits,
	}
}
