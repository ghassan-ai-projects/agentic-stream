package campaign

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const learnPhase = "learn"

const unsettledLearningQuery = `SELECT
	(SELECT COUNT(*) FROM rule_drafts WHERE status = 'requested')
	+ (SELECT COUNT(*) FROM rule_proposals WHERE status IN ('pending', 'approved'))
	+ (SELECT COUNT(*) FROM (SELECT diagnosis_code, kind_digest, intent, preset FROM case_verifications
		WHERE verdict = 'verified' GROUP BY diagnosis_code, kind_digest, intent, preset HAVING COUNT(*) >= ?1) ready
		WHERE NOT EXISTS (SELECT 1 FROM rule_drafts d WHERE d.diagnosis_code = ready.diagnosis_code
			AND d.kind_digest = ready.kind_digest AND d.intent = ready.intent AND d.preset = ready.preset))`

func (c Campaign) lastLearnRun() int {
	last := 0
	for _, run := range c.Runs {
		if run.Phase == learnPhase {
			last = run.Index
		}
	}
	return last
}

type playbookModes struct {
	Rules []struct {
		Mode string `yaml:"mode"`
	} `yaml:"rules"`
}

// LearningSettled reports whether learning has nothing left to do: no draft
// or proposal in flight, no key with draftAfter verified cases still waiting
// for a draft, and every rule of the playbook active, at least one of them.
func LearningSettled(playbookPath string, draftAfter int) func(context.Context, *sql.DB) (bool, error) {
	return func(ctx context.Context, db *sql.DB) (bool, error) {
		var unsettled int
		if err := db.QueryRowContext(ctx, unsettledLearningQuery, draftAfter).Scan(&unsettled); err != nil {
			return false, fmt.Errorf("read the learning still in flight: %w", err)
		}
		if unsettled > 0 {
			return false, nil
		}
		return everyRuleActive(playbookPath)
	}
}

func everyRuleActive(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read the playbook: %w", err)
	}
	var playbook playbookModes
	if err := yaml.Unmarshal(data, &playbook); err != nil {
		return false, fmt.Errorf("decode the playbook %s: %w", path, err)
	}
	for _, rule := range playbook.Rules {
		if rule.Mode != "active" {
			return false, nil
		}
	}
	return len(playbook.Rules) > 0, nil
}
