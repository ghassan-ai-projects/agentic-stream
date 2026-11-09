package main

import (
	"cmp"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"
)

const (
	twinRowsVariable = "CONVEYOR_TWIN_ROWS"
	plantLimp        = "the plant's limp feed"
)

type storyTable struct {
	SafeLimpFeedPct map[string]float64 `json:"safe_limp_feed_pct"`
	Stories         []tableStory       `json:"stories"`
}

type tableStory struct {
	ID           string             `json:"id"`
	Plant        string             `json:"plant"`
	Disturbances []tableDisturbance `json:"disturbances"`
	Rows         []map[string]any   `json:"rows"`
}

type tableDisturbance struct {
	Instance  string   `json:"instance"`
	Fault     string   `json:"fault"`
	At        string   `json:"at"`
	AtS       float64  `json:"at_s"`
	Severity  any      `json:"severity"`
	DurationS *float64 `json:"duration_s"`
}

func (c conveyorTwin) storyTable(t *testing.T) storyTable {
	t.Helper()
	var table storyTable
	decodeFile(t, filepath.Join(c.examples(), "belt-conveyor", "stories.json"), &table)
	return table
}

func (s storyTable) story(t *testing.T, id string) tableStory {
	t.Helper()
	at := slices.IndexFunc(s.Stories, func(story tableStory) bool { return story.ID == id })
	if at < 0 {
		t.Fatalf("stories.json has no story %s", id)
	}
	return s.Stories[at]
}

func twinRows(t *testing.T) int {
	t.Helper()
	rows, err := strconv.Atoi(cmp.Or(os.Getenv(twinRowsVariable), "1"))
	if err != nil || rows < 1 {
		t.Fatalf("%s=%q: want a positive number of seed rows (%v)", twinRowsVariable, os.Getenv(twinRowsVariable), err)
	}
	return rows
}

func (s tableStory) firstRows(n int) []map[string]any { return s.Rows[:min(n, len(s.Rows))] }

func (s tableStory) planOf(row map[string]any) func(plan map[string]any) {
	return func(plan map[string]any) {
		delete(plan, "pace")
		plan["seed"], plan["disturbances"] = row["seed"], s.disturbancesOf(row)
	}
}

func (s tableStory) disturbancesOf(row map[string]any) []any {
	var disturbances []map[string]any
	for _, d := range s.Disturbances {
		disturbances = append(disturbances, d.injectionsOf(row)...)
	}
	slices.SortStableFunc(disturbances, func(a, b map[string]any) int { return cmp.Compare(a["at_s"].(float64), b["at_s"].(float64)) })
	planned := make([]any, len(disturbances))
	for i, disturbance := range disturbances {
		planned[i] = disturbance
	}
	return planned
}

func (d tableDisturbance) injectionsOf(row map[string]any) []map[string]any {
	times := []float64{d.AtS}
	if d.At != "" {
		times = rowNumbers(row, d.At)
	}
	injections := make([]map[string]any, len(times))
	for i, at := range times {
		injections[i] = map[string]any{"at_s": at, "instance": d.Instance, "fault": d.Fault}
		if d.DurationS != nil {
			injections[i]["duration_s"] = *d.DurationS
		}
		switch severity := d.Severity.(type) {
		case float64:
			injections[i]["severity"] = severity
		case string:
			injections[i]["severity"] = rowNumbers(row, severity)[i]
		}
	}
	return injections
}

func rowNumbers(row map[string]any, field string) []float64 {
	if value, single := row[field].(float64); single {
		return []float64{value}
	}
	values, _ := row[field].([]any)
	numbers := make([]float64, len(values))
	for i, value := range values {
		numbers[i], _ = value.(float64)
	}
	return numbers
}

func (c conveyorTwin) limpSetpoint(t *testing.T, table storyTable, plant string) string {
	t.Helper()
	setpoint := readConveyorRoutes(t, plant)[limpFeed.Type].Presets[limpFeed.Parameters["level"].(string)]["feed_setpoint"]
	if setpoint != table.SafeLimpFeedPct[plant] {
		t.Fatalf("the catalog's limp feed of plant %s is %v %%, the story table's safe limp feed %v %%", plant, setpoint, table.SafeLimpFeedPct[plant])
	}
	return strconv.FormatFloat(setpoint, 'f', -1, 64)
}

func TestTheTwinBuildsEachStorysCommittedSessionPlanFromItsFirstRow(t *testing.T) {
	t.Parallel()
	binary, repository := streamsimNode(t)
	twin := conveyorTwin{binary: binary, repository: repository}
	table := twin.storyTable(t)
	for _, name := range slices.Sorted(maps.Keys(twinStories)) {
		story := table.story(t, name)
		committed := readJSONFile(t, filepath.Join(twin.examples(), "belt-conveyor", "sessions", name+".session-plan.json"))
		built := readJSONFile(t, twin.sessionPlan(t, name, "host.sock", "policy", story.planOf(story.Rows[0])))
		if !reflect.DeepEqual(built["disturbances"], committed["disturbances"]) || built["seed"] != committed["seed"] {
			t.Errorf("%s row 1: seed %v disturbances %v, want the committed plan's %v %v", name, built["seed"], built["disturbances"],
				committed["seed"], committed["disturbances"])
		}
	}
}
