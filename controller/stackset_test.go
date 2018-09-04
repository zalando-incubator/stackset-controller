package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestGetOwnerUID(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		OwnerReferences: []metav1.OwnerReference{
			{
				UID: types.UID("x"),
			},
		},
	}

	uid, ok := getOwnerUID(objectMeta)
	assert.Equal(t, types.UID("x"), uid)
	assert.True(t, ok)

	uid, ok = getOwnerUID(metav1.ObjectMeta{})
	assert.Equal(t, types.UID(""), uid)
	assert.False(t, ok)
}

func TestSanitizeServicePorts(t *testing.T) {
	service := &zv1.StackServiceSpec{
		Ports: []v1.ServicePort{
			{
				Protocol: "",
			},
		},
	}

	service = sanitizeServicePorts(service)
	assert.Len(t, service.Ports, 1)
	assert.Equal(t, v1.ProtocolTCP, service.Ports[0].Protocol)
}

func TestMergeLabels(t *testing.T) {
	labels1 := map[string]string{
		"foo": "bar",
	}

	labels2 := map[string]string{
		"foo": "baz",
	}

	merged := mergeLabels(labels1, labels2)
	assert.Equal(t, labels2, merged)

	labels3 := map[string]string{
		"bar": "foo",
	}

	merged = mergeLabels(labels1, labels3)
	assert.Equal(t, map[string]string{"foo": "bar", "bar": "foo"}, merged)
}

func TestIntOrStrIsEmpty(t *testing.T) {
	assert.True(t, intOrStrIsEmpty(intstr.FromInt(0)))
	assert.True(t, intOrStrIsEmpty(intstr.FromString("")))
	assert.False(t, intOrStrIsEmpty(intstr.FromInt(1)))
	assert.False(t, intOrStrIsEmpty(intstr.FromString("1")))
}
