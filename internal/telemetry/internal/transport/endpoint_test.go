package transport

import "testing"

func TestTracesEndpointURLDefaultsOnlyABarePath(t *testing.T) {
	t.Parallel()

	for endpoint, want := range map[string]string{
		"http://collector:4318":                "http://collector:4318/v1/traces",
		"http://collector:4318/":               "http://collector:4318/v1/traces",
		"https://collector.example/v1/traces":  "https://collector.example/v1/traces",
		"https://collector.example/otlp/trace": "https://collector.example/otlp/trace",
	} {
		got, err := tracesEndpointURL(endpoint)
		if err != nil || got != want {
			t.Errorf("tracesEndpointURL(%q) = %q, %v; want %q", endpoint, got, err, want)
		}
	}
	if _, err := tracesEndpointURL("http://[::1"); err == nil {
		t.Error("an unparsable endpoint was accepted")
	}
}
