package usage

import (
	"testing"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

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
			x:           0,
			y:           1,
			want:        0,
			shouldError: false,
		},
		{
			name:        "should return correct percentage",
			x:           36,
			y:           72,
			want:        50,
			shouldError: false,
		},
		{
			name:        "should return correct percentage",
			x:           75,
			y:           100,
			want:        75,
			shouldError: false,
		},
		{
			name:        "should return correct percentages over 100%",
			x:           3800,
			y:           2000,
			want:        190,
			shouldError: false,
		},
		{
			name:        "should return x if y is 0",
			x:           75,
			y:           0,
			want:        75,
			shouldError: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := getPercentage(tc.x, tc.y)

			if got != tc.want {
				t.Errorf("got %.2f want %.2f", tc.want, got)
			}
		})
	}
}

func TestGetRawUsage(t *testing.T) {
	cases := []struct {
		name        string
		cpu         *resource.Quantity
		mem         *resource.Quantity
		targetKey   string
		want        float64
		shouldError bool
	}{
		{
			name:        "return cpu usage in nanocores",
			cpu:         resource.NewQuantity(2756053, resource.Format("BinarySI")),
			mem:         resource.NewQuantity(838374, resource.Format("BinarySI")),
			targetKey:   "cpu",
			want:        2756053,
			shouldError: false,
		},
		{
			name:      "return memory usage in KiB",
			cpu:       resource.NewQuantity(8926483, resource.Format("BinarySI")),
			mem:       resource.NewQuantity(2332343, resource.Format("BinarySI")),
			targetKey: "memory",
			want:      2332343,
			shouldError: false,
		},
        {
            name: "should error with non-existant targetKey",
			cpu:       resource.NewQuantity(8, resource.Format("BinarySI")),
			mem:       resource.NewQuantity(2, resource.Format("BinarySI")),
			targetKey: "mem",
			want:      0,
			shouldError: true,
        },
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resourceList := resourceListHelper(tc.cpu, tc.mem)
			got, err := getRawUsage(resourceList, tc.targetKey)
            if !tc.shouldError && err != nil {
                t.Errorf("unexpected error: %v", err)
            }
            
			want := tc.want
			if got != want {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

func resourceListHelper(cpu *resource.Quantity, mem *resource.Quantity) v1.ResourceList {
	return v1.ResourceList{
		v1.ResourceCPU:    *cpu,
		v1.ResourceMemory: *mem,
	}
}
