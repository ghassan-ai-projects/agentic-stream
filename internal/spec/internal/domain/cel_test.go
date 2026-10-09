package domain_test

import (
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

func TestCELBoolAcceptsOnlyBooleans(t *testing.T) {
	t.Parallel()
	env, err := domain.NewCELEnv()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		expression string
		want       bool
		wantErr    string
	}{
		{expression: "1 < 2", want: true},
		{expression: "1 > 2"},
		{expression: "1 + 2", wantErr: "cel result not bool"},
		{expression: `"text"`, wantErr: "cel result not bool"},
	}
	for _, tt := range tests {
		t.Run(tt.expression, func(t *testing.T) {
			t.Parallel()
			ast, issues := env.Compile(tt.expression)
			if issues != nil && issues.Err() != nil {
				t.Fatal(issues.Err())
			}
			program, err := env.Program(ast)
			if err != nil {
				t.Fatal(err)
			}
			out, _, err := program.Eval(map[string]any{})
			if err != nil {
				t.Fatal(err)
			}
			got, err := domain.CELBool(out)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("CELBool(%s) error = %v, want %q", tt.expression, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("CELBool(%s) = %t, %v, want %t", tt.expression, got, err, tt.want)
			}
		})
	}
}

func TestCELEnvDeclaresOnlyTheSpecVariables(t *testing.T) {
	t.Parallel()
	env, err := domain.NewCELEnv()
	if err != nil {
		t.Fatal(err)
	}
	for _, accepted := range []string{`features.x > 1.0`, `situation.phase == "alert"`, `delta.phase_changed`, `event_time < watermark`} {
		if _, issues := env.Compile(accepted); issues != nil && issues.Err() != nil {
			t.Errorf("%q rejected: %v", accepted, issues.Err())
		}
	}
	for _, rejected := range []string{`now > 1`, `undeclared.x`, `rand() > 1`} {
		if _, issues := env.Compile(rejected); issues == nil || issues.Err() == nil {
			t.Errorf("%q compiled; the environment must expose only the declared variables", rejected)
		}
	}
}
