package campaign

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const schemaVersion = "0.1"

var (
	errUnknownArm = errors.New("unknown campaign arm")
	errNotJSON    = errors.New("the scorer printed no JSON document")
)

// Campaign is a streams-simulator campaign-v0.1: the ordered runs a consumer
// is driven through. It says nothing about what a run injects.
type Campaign struct {
	Dir    string `json:"-"`
	Domain string `json:"domain"`
	Seed   int64  `json:"seed"`
	Runs   []Run  `json:"runs"`
}

// Run is one run of a campaign.
type Run struct {
	Index       int     `json:"index"`
	Phase       string  `json:"phase"`
	Domain      string  `json:"domain"`
	Seed        int64   `json:"seed"`
	DurationS   float64 `json:"duration_s"`
	SessionPlan string  `json:"session_plan"`
}

// Duration is how long the run lasts in simulated time.
func (r Run) Duration() time.Duration { return time.Duration(r.DurationS * float64(time.Second)) }

// Load reads campaign.json from a campaign folder.
func Load(dir string) (Campaign, error) {
	data, err := readIn(dir, "campaign.json")
	if err != nil {
		return Campaign{}, fmt.Errorf("read the campaign: %w", err)
	}
	return decodeCampaign(dir, data)
}

func decodeCampaign(dir string, data []byte) (Campaign, error) {
	var document struct {
		SchemaVersion string `json:"schema_version"`
		Campaign
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return Campaign{}, fmt.Errorf("decode the campaign: %w", err)
	}
	if document.SchemaVersion != schemaVersion || len(document.Runs) == 0 {
		return Campaign{}, fmt.Errorf("campaign %s: schema %q with %d runs, want %s with runs", dir, document.SchemaVersion, len(document.Runs), schemaVersion)
	}
	document.Dir = dir
	return document.Campaign, nil
}

// Arm is one arm of the campaign: how the playbook is kept and who approves.
type Arm struct {
	Name     string
	Static   bool
	Learning bool
	Approver Approver
}

// ArmNamed returns the arm of the plan: 0 a static hand-written playbook, A no
// playbook, B learning with the careful approver, C learning with approve-all.
func ArmNamed(name string) (Arm, error) {
	switch name {
	case "0":
		return Arm{Name: name, Static: true, Approver: ApproveAll{}}, nil
	case "A":
		return Arm{Name: name, Approver: ApproveAll{}}, nil
	case "B":
		return Arm{Name: name, Learning: true, Approver: Careful{Coverage: 3, After: 4 * time.Hour}}, nil
	case "C":
		return Arm{Name: name, Learning: true, Approver: ApproveAll{}}, nil
	default:
		return Arm{}, fmt.Errorf("%w %q: want 0, A, B or C", errUnknownArm, name)
	}
}

func readIn(dir, name string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read %s in %s: %w", name, dir, err)
	}
	return data, nil
}
