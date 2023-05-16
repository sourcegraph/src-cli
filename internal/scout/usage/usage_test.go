package usage

import "testing"

func TestGetPercentage(t *testing.T) {
	cases := []struct {
		name        string
		x           float64
		y           float64
		want        float64
		shouldError bool
	}{
		{
			name:        "should return 0 if x is 0",
			x:           0.00,
			y:           1.00,
			want:        0.00,
			shouldError: false,
		},
		{
			name:        "should return correct percentage",
			x:           36.00,
			y:           72.00,
			want:        50.00,
			shouldError: false,
		},
		{
			name:        "should return correct percentage",
			x:           75.00,
			y:           100.00,
			want:        75.00,
			shouldError: false,
		},
		{
			name:        "should return throw an error if y is 0",
			x:           75.00,
			y:           0.00,
			want:        -1.00,
			shouldError: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := getPercentage(tc.x, tc.y)
			if err != nil && !tc.shouldError {
				t.Errorf("threw an error, but it shouldn't have")
			}

			if got != tc.want {
				t.Errorf("got %.2f want %.2f", tc.want, got)
			}
		})
	}
}
