package domain

import (
	"math"
	"strings"
)

type Expectation struct {
	Metric    string
	Value     any
	Tolerance float64
}

type output struct {
	name      string
	tolerance float64
}

func (i Inventory) Outputs(kind string) []string {
	var names []string
	for _, metric := range i.kinds[strings.ToLower(kind)].outputs() {
		names = append(names, metric.name)
	}
	return names
}

func (i Inventory) Expectations(kind string, args map[string]any) []Expectation {
	var expected []Expectation
	for _, metric := range i.kinds[strings.ToLower(kind)].outputs() {
		if value, commanded := args[metric.name]; commanded {
			expected = append(expected, Expectation{Metric: metric.name, Value: value, Tolerance: metric.tolerance})
		}
	}
	return expected
}

func (k Kind) outputs() []output {
	metrics, _ := k["metrics"].([]any)
	var writable []output
	for _, metric := range metrics {
		fields, _ := metric.(map[string]any)
		if name, _ := fields["name"].(string); fields["writable"] == true && name != "" {
			resolution, _ := numberOf(fields["resolution"])
			writable = append(writable, output{name: name, tolerance: resolution})
		}
	}
	return writable
}

func (e Expectation) Matches(value any) bool {
	if word, isWord := e.Value.(string); isWord {
		return value == word
	}
	want, wanted := numberOf(e.Value)
	got, read := numberOf(value)
	return wanted && read && math.Abs(got-want) <= e.Tolerance
}
