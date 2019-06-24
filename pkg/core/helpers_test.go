package core

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMergeLabels(t *testing.T) {
	labels1 := map[string]string{
		"foo": "bar",
	}

	labels2 := map[string]string{
		"foo": "baz",
	}

	merged := mergeLabels(labels1, labels2)
	require.Equal(t, labels2, merged)

	labels3 := map[string]string{
		"bar": "foo",
	}

	merged = mergeLabels(labels1, labels3)
	require.Equal(t, map[string]string{"foo": "bar", "bar": "foo"}, merged)
}

func TestGetStackGeneration(t *testing.T) {
	for _, tc := range []struct {
		name        string
		annotations map[string]string
		expected    int64
	}{
		{
			name:        "returns 0 without the annotation",
			annotations: nil,
			expected:    0,
		},
		{
			name:        "returns 0 with an invalid annotation",
			annotations: map[string]string{stackGenerationAnnotationKey: "foo"},
			expected:    0,
		},
		{
			name:        "returns parsed annotation value",
			annotations: map[string]string{stackGenerationAnnotationKey: "192"},
			expected:    192,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta := metav1.ObjectMeta{
				Name:        "foo",
				Annotations: tc.annotations,
			}
			require.Equal(t, tc.expected, getStackGeneration(meta))
		})
	}
}
