package advise

import "testing"

func TestCheckUsage(t *testing.T) {
	cases := []struct {
		name         string
		usage        float64
		resourceType string
		container    string
		want         string
	}{
		{
			name:         "should return correct message for usage over 100",
			usage:        110,
			resourceType: "cpu",
			container:    "gitserver-0",
			want:         "\t🚨 gitserver-0: Your cpu usage is over 100% (110.00%). Add more cpu.",
		},
		{
			name:         "should return correct message for usage over 80 and under 100",
			usage:        87,
			resourceType: "memory",
			container:    "gitserver-0",
			want:         "\t⚠️  gitserver-0: Your memory usage is over 80% (87.00%). Consider raising limits.",
		},
		{
			name:         "should return correct message for usage over 40 and under 80",
			usage:        63.4,
			resourceType: "memory",
			container:    "gitserver-0",
			want:         "\t✅ gitserver-0: Your memory usage is under 80% (63.40%). Keep memory allocation the same.",
		},
		{
			name:         "should return correct message for usage under 40",
			usage:        22.33,
			resourceType: "memory",
			container:    "gitserver-0",
			want:         "\t⚠️  gitserver-0: Your memory usage is under 40% (22.33%). Consider lowering limits.",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := CheckUsage(tc.usage, tc.resourceType, tc.container)

			if got != tc.want {
				t.Errorf("got: '%s' want '%s'", got, tc.want)
			}
		})
	}
}
