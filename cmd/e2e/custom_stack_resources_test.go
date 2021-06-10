package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
)

const (
	customStackIngressRouteGroupAnnotation      = "custom-annotation"
	customStackIngressRouteGroupAnnotationValue = "custom-annotation-value"
)

func TestStackIngressCustomAnnotations(t *testing.T) {
	t.Parallel()
	stacksetName := "stack-ingress-custom-annotation"

	factory := NewTestStacksetSpecFactory(stacksetName).Ingress()
	version := "v1"
	spec := factory.Create(version)
	spec.StackTemplate.Spec.IngressOverrides = &zv1.StackIngressRouteGroupOverrides{
		EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
			Annotations: map[string]string{customStackIngressRouteGroupAnnotation: customStackIngressRouteGroupAnnotationValue},
		},
	}
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	stackIngress, err := waitForIngress(t, factory.StackName(version))
	require.NoError(t, err)

	require.EqualValues(t, customStackIngressRouteGroupAnnotationValue, stackIngress.Annotations[customStackIngressRouteGroupAnnotation])
	require.NotContains(t, stackIngress.Annotations, userTestAnnotation)
}

func TestStackIngressCustomHosts(t *testing.T) {
	t.Parallel()
	stacksetName := "stack-ingress-custom-hosts"

	factory := NewTestStacksetSpecFactory(stacksetName).Ingress()
	version := "v1"
	spec := factory.Create(version)
	spec.StackTemplate.Spec.IngressOverrides = &zv1.StackIngressRouteGroupOverrides{
		Hosts: []string{
			fmt.Sprintf("custom-$(STACK_NAME).%s", clusterDomain),
		},
	}
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	stackIngress, err := waitForIngress(t, factory.StackName(version))
	require.NoError(t, err)

	require.Len(t, stackIngress.Spec.Rules, 1)
	require.Equal(t, fmt.Sprintf("custom-%s.%s", factory.StackName("v1"), clusterDomain), stackIngress.Spec.Rules[0].Host)
}

func TestStackIngressEnabled(t *testing.T) {
	t.Parallel()
	stacksetName := "stack-ingress-enabled"

	factory := NewTestStacksetSpecFactory(stacksetName).Ingress()
	version := "v1"
	spec := factory.Create(version)
	boolTrue := true
	spec.StackTemplate.Spec.IngressOverrides = &zv1.StackIngressRouteGroupOverrides{
		Enabled: &boolTrue,
	}
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	_, err = waitForIngress(t, factory.StackName(version))
	require.NoError(t, err)

	stack, err := waitForStack(t, stacksetName, version)
	require.NoError(t, err)

	boolFalse := false
	stack.Spec.IngressOverrides = &zv1.StackIngressRouteGroupOverrides{
		Enabled: &boolFalse,
	}
	err = updateStack(factory.StackName(version), stack.Spec)
	require.NoError(t, err)

	err = resourceDeleted(t, "ingress", factory.StackName(version), ingressInterface()).await()
	require.NoError(t, err)
}

func TestStackRouteGroupCustomAnnotations(t *testing.T) {
	t.Parallel()
	stacksetName := "stack-routegroup-custom-annotation"

	factory := NewTestStacksetSpecFactory(stacksetName).RouteGroup()
	version := "v1"
	spec := factory.Create(version)
	spec.StackTemplate.Spec.RouteGroupOverrides = &zv1.StackIngressRouteGroupOverrides{
		EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
			Annotations: map[string]string{customStackIngressRouteGroupAnnotation: customStackIngressRouteGroupAnnotationValue},
		},
	}
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	stackRoutegroup, err := waitForRouteGroup(t, factory.StackName(version))
	require.NoError(t, err)

	require.EqualValues(t, customStackIngressRouteGroupAnnotationValue, stackRoutegroup.Annotations[customStackIngressRouteGroupAnnotation])
	require.NotContains(t, stackRoutegroup.Annotations, userTestAnnotation)
}

func TestStackRouteGroupCustomHosts(t *testing.T) {
	t.Parallel()
	stacksetName := "stack-routegroup-custom-hosts"

	factory := NewTestStacksetSpecFactory(stacksetName).RouteGroup()
	version := "v1"
	spec := factory.Create(version)
	spec.StackTemplate.Spec.RouteGroupOverrides = &zv1.StackIngressRouteGroupOverrides{
		Hosts: []string{
			fmt.Sprintf("custom-$(STACK_NAME).%s", clusterDomain),
		},
	}
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	stackRoutegroup, err := waitForRouteGroup(t, factory.StackName(version))
	require.NoError(t, err)

	require.EqualValues(t, []string{fmt.Sprintf("custom-%s.%s", factory.StackName("v1"), clusterDomain)}, stackRoutegroup.Spec.Hosts)
}

func TestStackRoutegroupEnabled(t *testing.T) {
	t.Parallel()
	stacksetName := "stack-routegroup-enabled"

	factory := NewTestStacksetSpecFactory(stacksetName).RouteGroup()
	version := "v1"
	spec := factory.Create(version)
	boolTrue := true
	spec.StackTemplate.Spec.RouteGroupOverrides = &zv1.StackIngressRouteGroupOverrides{
		Enabled: &boolTrue,
	}
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	_, err = waitForRouteGroup(t, factory.StackName(version))
	require.NoError(t, err)

	stack, err := waitForStack(t, stacksetName, version)
	require.NoError(t, err)

	boolFalse := false
	stack.Spec.RouteGroupOverrides = &zv1.StackIngressRouteGroupOverrides{
		Enabled: &boolFalse,
	}
	err = updateStack(factory.StackName(version), stack.Spec)
	require.NoError(t, err)

	err = resourceDeleted(t, "routegroup", factory.StackName(version), routegroupInterface()).await()
	require.NoError(t, err)
}
