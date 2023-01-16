package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidatePod(t *testing.T) {
	cases := []struct {
		name string
		pod  func(pod *corev1.Pod)
		err  string
	}{
		{
			name: "valid pod",
		},
		{
			name: "invalid pod: image is not set",
			pod: func(pod *corev1.Pod) {
				pod.Spec.Containers[0].Image = ""
			},
			err: "container.Ports is not set",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pod := testPod()
			if tc.pod != nil {
				tc.pod(pod)
			}
			err := validatePod(pod)

			// test should error
			if tc.err != "" {
				if err == nil {
					t.Fatal("validate should error, but got non-nil error")
					return
				}
				if err.Error() != tc.err {
					t.Errorf("err msg\nwant: %q\n got: %q", tc.err, err.Error())
				}
				return
			}

			// test should not error
			if err != nil {
				t.Fatalf("ValidatePod error: %s", err)
			}
		})
	}
}

// helper test function to return a valid pod
func testPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "pod-123",
			Annotations: map[string]string{
				"ready": "ensure that this annotation is set",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "some-container",
					Image: "fatih/foo:test",
					Command: []string{
						"./foo",
						"--port=8800",
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 8800,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
			},
		},
	}
}
