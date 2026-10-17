package payments

import "testing"

func TestValidEmail(t *testing.T) {
	tests := []struct {
		email string
		want  bool
	}{
		{"buyer@example.com", true},
		{"a@b", false},
		{"@example.com", false},
		{"buyer@", false},
		{"buyer @example.com", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := validEmail(tt.email); got != tt.want {
			t.Errorf("validEmail(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}
