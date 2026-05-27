package incident

import "testing"

func TestValidateTransition(t *testing.T) {
	cases := []struct {
		from    State
		to      State
		wantErr bool
	}{
		{"", StateStarted, false},
		{"", StateSuggested, false},
		{StateSuggested, StateStarted, false},
		{StateSuggested, StateResolved, false},
		{StateStarted, StateAcknowledged, false},
		{StateStarted, StateResolved, false},
		{StateAcknowledged, StateResolved, false},
		{StateSuggested, StateAcknowledged, true},
		{StateResolved, StateStarted, true},
		{StateAcknowledged, StateStarted, true},
		{StateStarted, StateStarted, true},
	}
	for _, tc := range cases {
		err := ValidateTransition(tc.from, tc.to)
		if tc.wantErr && err == nil {
			t.Fatalf("expected error for %q -> %q", tc.from, tc.to)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("unexpected error for %q -> %q: %v", tc.from, tc.to, err)
		}
	}
}
