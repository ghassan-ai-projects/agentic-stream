package canonicaljson_test

import (
	"encoding/json"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestDigestsMatchTheFrozenRubyVectors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		domain canonicaljson.Domain
		input  string
		want   string
	}{
		{
			name:   "prompt",
			domain: canonicaljson.DomainPrompt,
			input:  `{"version":"1.0","text":"You are the pond supervisor's diagnostic assistant."}`,
			want:   "sha256:d288842cb90fe718a43e494a527b95cea1fd94e2a673bc65507e3bc3d93bf45a",
		},
		{
			name:   "diagnosis catalog is bound over the parsed array",
			domain: canonicaljson.DomainDiagnosisCatalog,
			input:  `[{"code":"unknown","description":"no confident diagnosis"}]`,
			want:   "sha256:50ef8a500d51c12bb1134738cb559bfb9541ef286ba43d0e0558963bf384e553",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var parsed any
			if err := json.Unmarshal([]byte(tt.input), &parsed); err != nil {
				t.Fatalf("decode input: %v", err)
			}
			got, err := canonicaljson.Digest(tt.domain, parsed)
			if err != nil || got != tt.want {
				t.Fatalf("Digest = %q, %v; want the Ruby-computed %q", got, err, tt.want)
			}
		})
	}
}
