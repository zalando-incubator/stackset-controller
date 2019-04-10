package entities

import (
	"testing"

	"github.com/stretchr/testify/assert"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	v1 "k8s.io/api/core/v1"
)

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
