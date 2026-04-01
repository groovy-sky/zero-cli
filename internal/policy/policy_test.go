package policy

import "testing"

func TestIsURLAllowed(t *testing.T) {
	p := Policy{
		Net: Net{
			Enabled: true,
			HTTPAllow: []string{
				"https://api.github.com/",
				"https://pypi.org/",
			},
		},
	}

	cases := []struct {
		url  string
		want bool
	}{
		{"https://api.github.com/zen", true},
		{"https://api.github.com/", true},
		{"https://pypi.org/simple/", true},
		{"https://example.com/", false},
		{"http://api.github.com/zen", false}, // scheme mismatch due to prefix rules
	}

	for _, tc := range cases {
		got := p.IsURLAllowed(tc.url)
		if got != tc.want {
			t.Fatalf("IsURLAllowed(%q)=%v, want %v", tc.url, got, tc.want)
		}
	}
}