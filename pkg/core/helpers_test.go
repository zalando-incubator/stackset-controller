package core

import (
	"reflect"
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

func TestSyncAnnotations(t *testing.T) {
	for _, tc := range []struct {
		dest              map[string]string
		src               map[string]string
		annotationsToSync []string
		expected          map[string]string
	}{
		{
			dest:              map[string]string{"a": "1", "b": "2"},
			src:               map[string]string{"a": "1"},
			annotationsToSync: []string{"a"},
			expected:          map[string]string{"a": "1", "b": "2"},
		},
		{
			dest:              map[string]string{"a": "1", "b": "2"},
			src:               map[string]string{"a": "3"},
			annotationsToSync: []string{"a"},
			expected:          map[string]string{"a": "3", "b": "2"},
		},
		{
			dest:              map[string]string{"a": "1", "b": "2"},
			src:               map[string]string{},
			annotationsToSync: []string{"a"},
			expected:          map[string]string{"b": "2"},
		},
		{
			dest:              map[string]string{"a": "1", "b": "2"},
			src:               map[string]string{},
			annotationsToSync: []string{},
			expected:          map[string]string{"a": "1", "b": "2"},
		},
	} {
		res := syncAnnotations(tc.dest, tc.src, tc.annotationsToSync)
		if !reflect.DeepEqual(tc.expected, res) {
			t.Errorf("expected %v, got %v", tc.expected, res)
		}
	}
}

func TestGetKeyValues(t * testing.T) {
	for _, tc := range []struct {
		keys []string
		annotations map[string]string
		expected map[string]string
	}{
		{
			keys: []string{"a"},
			annotations: map[string]string{"a": "1", "b": "2"},
			expected: map[string]string{"a": "1"},
		},
		{
			keys: []string{"a", "b"},
			annotations: map[string]string{"a": "1", "b": "2"},
			expected: map[string]string{"a": "1", "b": "2"},
		},
		{
			keys: []string{},
			annotations: map[string]string{"a": "1", "b": "2"},
			expected: map[string]string{},
		},
		{
			keys: []string{"c"},
			annotations: map[string]string{"a": "1", "b": "2"},
			expected: map[string]string{},
		},
		{
			keys: []string{"a", "c"},
			annotations: map[string]string{"a": "1", "b": "2"},
			expected: map[string]string{"a": "1"},
		},
	} {
		res := getKeyValues(tc.keys, tc.annotations)
		if !reflect.DeepEqual(tc.expected, res) {
			t.Errorf("expected %v, got %v", tc.expected, res)
		}
	}
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
