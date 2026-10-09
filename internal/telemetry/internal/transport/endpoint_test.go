package transport

import "testing"

func TestTracesEndpointURLDefaultsOnlyABarePath(t *testing.T) {
	t.Parallel()
	tests := []struct{ endpoint, want string }{
		{"http://collector:4318", "http://collector:4318/v1/traces"},
		{"http://collector:4318/", "http://collector:4318/v1/traces"},
		{"https://collector.example/v1/traces", "https://collector.example/v1/traces"},
		{"https://collector.example/otlp/trace", "https://collector.example/otlp/trace"},
	}
	for _, tc := range tests {
		t.Run(tc.endpoint, func(t *testing.T) {
			t.Parallel()
			got, err := tracesEndpointURL(tc.endpoint)
			if err != nil || got != tc.want {
				t.Fatalf("tracesEndpointURL(%q) = %q, %v; want %q", tc.endpoint, got, err, tc.want)
			}
		})
	}
}

func TestTracesEndpointURLRejectsAnUnparsableEndpoint(t *testing.T) {
	t.Parallel()
	if got, err := tracesEndpointURL("http://[::1"); err == nil {
		t.Fatalf("an unparsable endpoint was accepted as %q", got)
	}
}
