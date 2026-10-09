package main

import (
	"strings"
	"testing"
)

// assertClosedLoopInspectable follows the live run's dispatched intent from
// its episode to the verification of its command, through the CLI alone.
func assertClosedLoopInspectable(t *testing.T, dbPath string) {
	t.Helper()
	var intentID, episodeID string
	if err := openReadOnly(t, dbPath).QueryRowContext(t.Context(), `
		SELECT i.intent_id, d.episode_id FROM intents i JOIN decisions d ON d.decision_id = i.decision_id
		JOIN commands c ON c.intent_id = i.intent_id LIMIT 1`).Scan(&intentID, &episodeID); err != nil {
		t.Fatalf("the live run dispatched no intent: %v", err)
	}
	var episode episodeInspection
	decodeJSON(t, runCLI(t, newEpisodeCommand(), "show", episodeID, "--db", dbPath, "--json"), &episode)
	if len(episode.Episode.Attempts) == 0 || len(episode.Decisions) == 0 || len(episode.Decisions[0].Intents) == 0 {
		t.Fatalf("episode %s does not lead to its intent: %+v", episodeID, episode)
	}
	var intent intentInspection
	decodeJSON(t, runCLI(t, newIntentCommand(), "show", intentID, "--db", dbPath, "--json"), &intent)
	if len(intent.Intent.Evaluations) == 0 || len(intent.Commands) == 0 || len(intent.Commands[0].Command.Outcomes) == 0 || len(intent.Commands[0].Command.Verifications) == 0 {
		t.Fatalf("intent %s does not lead to its command, outcome and verification: %+v", intentID, intent)
	}
	assertInspectionText(t, dbPath, episodeID, intentID)
}

func assertInspectionText(t *testing.T, dbPath, episodeID, intentID string) {
	t.Helper()
	episode := runCLI(t, newEpisodeCommand(), "show", episodeID, "--db", dbPath)
	for _, want := range []string{episodeID, "attempt ", "decision ", "  intent " + intentID, " risk=R1 policy=approved"} {
		if !strings.Contains(episode, want) {
			t.Errorf("episode show = %q, want it to contain %q", episode, want)
		}
	}
	intent := runCLI(t, newIntentCommand(), "show", intentID, "--db", dbPath)
	for _, want := range []string{intentID + " set_indicator risk=R1 policy=approved", "policy ", ": approved (", "command ", "  outcome ", "  verification "} {
		if !strings.Contains(intent, want) {
			t.Errorf("intent show = %q, want it to contain %q", intent, want)
		}
	}
}
