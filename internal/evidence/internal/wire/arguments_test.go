package wire

import "testing"

func TestDecodeEvidenceGetArgumentsAcceptsOnlyTheClosedV1Schema(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "entity only", raw: `{"entity_id":"motor-1"}`, want: "motor-1"},
		{name: "empty object", raw: `{}`, wantErr: true},
		{name: "empty entity", raw: `{"entity_id":""}`, wantErr: true},
		{name: "entity of the wrong type", raw: `{"entity_id":1}`, wantErr: true},
		{name: "unknown field", raw: `{"entity_id":"x","extra":true}`, wantErr: true},
		{name: "query dimension smuggled in", raw: `{"entity_id":"x","tenant_id":"other"}`, wantErr: true},
		{name: "trailing document", raw: `{"entity_id":"x"}{}`, wantErr: true},
		{name: "trailing garbage", raw: `{"entity_id":"x"} garbage`, wantErr: true},
		{name: "not an object", raw: `["motor-1"]`, wantErr: true},
		{name: "not json", raw: `motor-1`, wantErr: true},
		{name: "empty input", raw: ``, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecodeEvidenceGetArguments([]byte(test.raw))
			if test.wantErr {
				requireError(t, err, test.raw)
				return
			}
			if err != nil || got.EntityID != test.want {
				t.Fatalf("arguments = %+v, err %v, want entity %q", got, err, test.want)
			}
		})
	}
}
