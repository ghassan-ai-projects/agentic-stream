package domain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// executorDocument is the spec-derived executor section of every request,
// with the provenance digests the worker and validator verify.
type executorEvidence struct {
	Document        map[string]any
	PromptSHA256    string
	ObjectiveSHA256 string
}

func (a requestAssembly) executorDocument() (executorEvidence, error) {
	evidence := executorEvidence{Document: a.executorConfiguration()}
	document := evidence.Document
	if err := a.bindExecutorProvenance(&evidence); err != nil {
		return executorEvidence{}, err
	}
	if err := a.bindIntentCatalog(document); err != nil {
		return executorEvidence{}, err
	}
	document["skill_refs"] = a.spec.Cognition.Executor.Skills
	return evidence, nil
}

func (a requestAssembly) executorConfiguration() map[string]any {
	return map[string]any{
		"name": a.spec.Cognition.Executor.Name, "model_policy": a.spec.Cognition.Executor.ModelPolicy,
		"prompt_version": a.spec.Cognition.Executor.PromptVersion, "prompt": a.spec.Cognition.Executor.Prompt,
		"objective": a.spec.Cognition.Executor.Objective, "decision_schema": a.spec.Cognition.Executor.DecisionSchema,
		"diagnosis_catalog": a.spec.Cognition.Executor.DiagnosisCatalog,
	}
}

func (a requestAssembly) bindExecutorProvenance(evidence *executorEvidence) error {
	prompt, objective, err := a.executorTextDigests()
	if err != nil {
		return err
	}
	catalog, err := a.diagnosisCatalogDigest()
	if err != nil {
		return err
	}
	evidence.PromptSHA256 = prompt
	evidence.Document["prompt_sha256"] = prompt
	evidence.ObjectiveSHA256 = objective
	evidence.Document["objective_sha256"] = objective
	evidence.Document["diagnosis_catalog_sha256"] = catalog
	return nil
}

func (a requestAssembly) executorTextDigests() (string, string, error) {
	promptDigest, err := canonicaljson.Digest(canonicaljson.DomainPrompt, map[string]any{
		"version": a.spec.Cognition.Executor.PromptVersion,
		"text":    a.spec.Cognition.Executor.Prompt,
	})
	if err != nil {
		return "", "", fmt.Errorf("digest prompt provenance: %w", err)
	}
	objectiveDigest, err := canonicaljson.Digest(canonicaljson.DomainObjective, map[string]any{"text": a.spec.Cognition.Executor.Objective})
	if err != nil {
		return "", "", fmt.Errorf("digest objective provenance: %w", err)
	}
	return promptDigest, objectiveDigest, nil
}

// diagnosisCatalogDigest hashes the parsed catalog array, matching the worker wire contract.
func (a requestAssembly) diagnosisCatalogDigest() (string, error) {
	var catalogValue any = []any{}
	if strings.TrimSpace(a.spec.Cognition.Executor.DiagnosisCatalog) != "" {
		if err := json.Unmarshal([]byte(a.spec.Cognition.Executor.DiagnosisCatalog), &catalogValue); err != nil {
			return "", fmt.Errorf("diagnosis catalog is not valid JSON: %w", err)
		}
	}
	catalogDigest, err := canonicaljson.Digest(canonicaljson.DomainDiagnosisCatalog, catalogValue)
	if err != nil {
		return "", fmt.Errorf("digest diagnosis catalog provenance: %w", err)
	}
	return catalogDigest, nil
}

func (a requestAssembly) bindIntentCatalog(executorDocument map[string]any) error {
	intentCatalog, intentCatalogDigest, err := CompileIntentCatalog(a.spec.Actions.Intents)
	if err != nil {
		return fmt.Errorf("compile intent catalog: %w", err)
	}
	executorDocument["intent_catalog"] = intentCatalog
	executorDocument["intent_catalog_sha256"] = intentCatalogDigest

	return nil
}

func (a requestAssembly) buildTools() []map[string]any {
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

func (a requestAssembly) allowedIntentTypeList() []string {
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

func (a requestAssembly) allowedIntentTypes() map[string]bool {
	configured := make(map[string]bool)
	for _, intent := range a.spec.Actions.Intents {
		configured[intent.Type] = true
	}
	return configured
}

func (a requestAssembly) effectiveRiskCeiling() string {
	ceiling := a.spec.Cognition.Executor.RiskCeiling
	if ceiling == "" {
		return "R1"
	}
	return ceiling
}

func (a requestAssembly) budgetMap() map[string]any {
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
