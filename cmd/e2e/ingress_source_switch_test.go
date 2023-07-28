package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/controller"
)

const (
	// The e2e env IngressSourceSwitchTTL
	IngressSourceSwitchTTL = time.Minute
)

func TestIngressSourceSwitch(t *testing.T) {
	t.Parallel()

	stacksetName := "ingress-source-switch-stackset"
	firstVersion := "v1"

	// create stackset with ingress
	factory := NewTestStacksetSpecFactory(stacksetName).Ingress()
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)
	_, err = waitForIngress(
		t,
		fmt.Sprintf("%s-%s-traffic-segment", stacksetName, firstVersion),
	)
	require.NoError(t, err)

	// update stackset adding a routegroup
	updatedVersion := "v2"
	spec = factory.RouteGroup().Create(updatedVersion)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	
	rg, err := waitForRouteGroup(
		t,
		fmt.Sprintf("%s-%s-traffic-segment", stacksetName, updatedVersion),
	)
	require.NoError(t, err)

	_, err = waitForIngress(
		t,
		fmt.Sprintf("%s-%s-traffic-segment", stacksetName, updatedVersion),
	)
	require.NoError(t, err)

	// update the routegroup and delete the ingress
	lastVersion := "v3"
	lastSpec := spec
	lastSpec.StackTemplate.Spec.Version = lastVersion
	lastSpec.RouteGroup.AdditionalBackends = []v1.RouteGroupBackend{{Name: "shunt", Type: v1.ShuntRouteGroupBackend}}
	lastSpec.Ingress = nil
	err = updateStackset(stacksetName, lastSpec)
	require.NoError(t, err)
	_, err = waitForUpdatedRouteGroup(
		t,
		fmt.Sprintf("%s-%s-traffic-segment", stacksetName, lastVersion),
		rg.Annotations[controller.ControllerLastUpdatedAnnotationKey],
	)
	require.NoError(t, err)
	// If the ingress is not deleted right away, then, it respects the
	// TTL.
	err = resourceDeleted(
		t,
		"ingress",
		fmt.Sprintf("%s-%s-traffic-segment", stacksetName, lastVersion),
		ingressInterface(),
	).await()
	require.Error(t, err)

	// make sure the ingress is finally deleted after twice the TTL
	a := resourceDeleted(
		t,
		"ingress",
		fmt.Sprintf("%s-%s-traffic-segment", stacksetName, lastVersion),
		ingressInterface(),
	)
	a.timeout += IngressSourceSwitchTTL * 2
	err = a.await()
	require.NoError(t, err)
}

func TestIngressToRouteGroupSwitch(t *testing.T) {
	t.Parallel()

	stacksetName := "ingress-to-routegroup-switch"
	firstVersion := "v1"

	// create stackset with ingress and routegroup
	factory := NewTestStacksetSpecFactory(stacksetName).Ingress().RouteGroup()
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)
	rg, err := waitForRouteGroup(
		t,
		fmt.Sprintf("%s-%s-traffic-segment", stacksetName, firstVersion),
	)
	require.NoError(t, err)

	// Wait the IngressSourceSwitchTTL to make sure the ingress is
	// created a time long enough ago.
	time.Sleep(IngressSourceSwitchTTL)

	// update the routegroup and delete the ingress
	lastVersion := "v2"
	lastSpec := spec
	lastSpec.StackTemplate.Spec.Version = lastVersion
	lastSpec.RouteGroup.AdditionalBackends = []v1.RouteGroupBackend{{Name: "shunt", Type: v1.ShuntRouteGroupBackend}}
	lastSpec.Ingress = nil

	err = updateStackset(stacksetName, lastSpec)
	require.NoError(t, err)
	_, err = waitForUpdatedRouteGroup(
		t,
		fmt.Sprintf("%s-%s-traffic-segment", stacksetName, lastVersion),
		rg.Annotations[controller.ControllerLastUpdatedAnnotationKey],
	)
	require.NoError(t, err)

	// make sure ingress is not deleted before IngressSourceSwitchTTL
	err = resourceDeleted(
		t, "ingress", stacksetName, ingressInterface()).await()
	require.Error(t, err)
}

func TestRouteGroupToIngressSwitch(t *testing.T) {
	t.Parallel()

	stacksetName := "routegroup-to-ingress-switch"
	firstVersion := "v1"

	// create stackset with ingress and routegroup
	factory := NewTestStacksetSpecFactory(stacksetName).Ingress().RouteGroup()
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)
	ing, err := waitForIngress(t, stacksetName)
	require.NoError(t, err)

	// Wait the IngressSourceSwitchTTL to make sure the RouteGroup is
	// created a time long enough ago.
	time.Sleep(IngressSourceSwitchTTL)

	// update the Ingress and delete the RouteGroup
	lastVersion := "v2"
	lastSpec := spec
	lastSpec.StackTemplate.Spec.Version = lastVersion
	lastSpec.Ingress.Annotations["a-random-annotation"] = "a-random-annotation-value"
	lastSpec.RouteGroup = nil
	err = updateStackset(stacksetName, lastSpec)
	require.NoError(t, err)
	_, err = waitForUpdatedIngress(t, ing.Name, ing.Annotations[controller.ControllerLastUpdatedAnnotationKey])
	require.NoError(t, err)

	// make sure ingress is not deleted before IngressSourceSwitchTTL
	err = resourceDeleted(t, "routegroup", stacksetName, routegroupInterface()).await()
	require.Error(t, err)
}
