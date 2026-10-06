package domain

import (
	"testing"
	"time"
)

func TestLocationValidate(t *testing.T) {
	now := time.Now().Unix()
	tests := []struct {
		name    string
		loc     Location
		wantErr bool
	}{
		// --- valid plates ---
		{"B1234XYZ", Location{"B1234XYZ", -6.2, 106.8, now}, false},
		{"D5678AB", Location{"D5678AB", -6.2, 106.8, now}, false},
		{"B1X", Location{"B1X", -6.2, 106.8, now}, false},
		{"AB1234CDE", Location{"AB1234CDE", -6.2, 106.8, now}, false},

		// --- invalid plates ---
		{"lowercase", Location{"b1234xyz", -6.2, 106.8, now}, true},
		{"with space", Location{"B 1234 XYZ", -6.2, 106.8, now}, true},
		{"with dash", Location{"B-1234-XYZ", -6.2, 106.8, now}, true},
		{"empty", Location{"", -6.2, 106.8, now}, true},
		{"too long", Location{"ABCDE123456789", -6.2, 106.8, now}, true},
		{"digits only", Location{"123456", -6.2, 106.8, now}, true},

		// --- lat/lon ---
		{"lat too high", Location{"B1234XYZ", 91, 106.8, now}, true},
		{"lat too low", Location{"B1234XYZ", -91, 106.8, now}, true},
		{"lon too high", Location{"B1234XYZ", -6.2, 181, now}, true},
		{"lon too low", Location{"B1234XYZ", -6.2, -181, now}, true},

		// --- timestamp ---
		{"zero ts", Location{"B1234XYZ", -6.2, 106.8, 0}, true},
		{"negative ts", Location{"B1234XYZ", -6.2, 106.8, -1}, true},
		{"future ts", Location{"B1234XYZ", -6.2, 106.8, now + 1000}, true},
		{"too old ts", Location{"B1234XYZ", -6.2, 106.8, now - 31*86400}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.loc.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
