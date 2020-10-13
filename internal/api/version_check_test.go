package api

import "testing"

func TestCheckSourcegraphVersion(t *testing.T) {
	for _, tc := range []struct {
		currentVersion, constraint, minDate string
		expected                            bool
	}{
		{
			currentVersion: "3.12.6",
			constraint:     ">= 3.12.6",
			minDate:        "2020-01-19",
			expected:       true,
		},
		{
			currentVersion: "3.12.6-rc.1",
			constraint:     ">= 3.12.6-0",
			minDate:        "2020-01-19",
			expected:       true,
		},
		{
			currentVersion: "3.12.6",
			constraint:     ">= 3.13",
			minDate:        "2020-01-19",
			expected:       false,
		},
		{
			currentVersion: "3.13.0",
			constraint:     ">= 3.13",
			minDate:        "2020-01-19",
			expected:       true,
		},
		{
			currentVersion: "dev",
			constraint:     ">= 3.13",
			minDate:        "2020-01-19",
			expected:       true,
		},
		{
			currentVersion: "0.0.0+dev",
			constraint:     ">= 3.13",
			minDate:        "2020-01-19",
			expected:       true,
		},
		{
			currentVersion: "54959_2020-01-29_9258595",
			minDate:        "2020-01-19",
			constraint:     ">= 999.13",
			expected:       true,
		},
		{
			currentVersion: "54959_2020-01-29_9258595",
			minDate:        "2020-01-30",
			constraint:     ">= 999.13",
			expected:       false,
		},
		{
			currentVersion: "54959_2020-01-29_9258595",
			minDate:        "2020-01-29",
			constraint:     ">= 0.0",
			expected:       true,
		},
	} {
		actual, err := CheckSourcegraphVersion(tc.currentVersion, tc.constraint, tc.minDate)
		if err != nil {
			t.Errorf("err: %s", err)
		}

		if actual != tc.expected {
			t.Errorf("wrong result. want=%t, got=%t (version=%q, constraint=%q)", tc.expected, actual, tc.currentVersion, tc.constraint)
		}
	}
}
