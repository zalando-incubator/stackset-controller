package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConfigurationResources(t *testing.T) {
	suite.Run(t, new(ConfigurationResourcesTestSuite))
}

type ConfigurationResourcesTestSuite struct {
	suite.Suite

	stacksetSpecFactory *TestStacksetSpecFactory

	stacksetName string
	stackVersion string
}

func (suite *ConfigurationResourcesTestSuite) SetupTest() {
	suite.stacksetName = "stackset-cr"
	suite.stackVersion = "v1"

	suite.cleanUpStackSet(suite.stacksetName)

	suite.stacksetSpecFactory = NewTestStacksetSpecFactory(suite.stacksetName)

	// TODO: Remove this because it's not needed for this test but fails otherwise
	suite.stacksetSpecFactory.SubResourceAnnotations(map[string]string{"foo": "bar"})
}

// TestInlinePlatformCredentialsSets tests that PlatformCredentialsSets defined inline in the StackSet spec are created and owned by the Stack.
func (suite *ConfigurationResourcesTestSuite) TestInlinePlatformCredentialsSets() {
	suite.stacksetSpecFactory.AddInlinePlatformCredentialsSet(&zv1.PlatformCredentialsSet{
		Name: "my-platform-credentials-set",
		// Data: map[string]string{"key": "value"},
	})

	stacksetSpec := suite.stacksetSpecFactory.Create(suite.T(), suite.stackVersion)

	err := createStackSet(suite.stacksetName, 0, stacksetSpec)
	suite.Require().NoError(err)

	stack, err := waitForStack(suite.T(), suite.stacksetName, suite.stackVersion)
	suite.Require().NoError(err)

	// verifyStack(t, stacksetName, stackVersion, stacksetSpec, nil)

	platformCredentialsSet, err := waitForPlatformCredentialsSet(suite.T(), "stackset-cr-v1-my-platform-credentials-set")
	suite.Require().NoError(err)

	// suite.Equal(map[string]string{"key": "value"}, platformCredentialsSet.Data)
	suite.Equal([]metav1.OwnerReference{
		{
			APIVersion: core.APIVersion,
			Kind:       core.KindStack,
			Name:       stack.Name,
			UID:        stack.UID,
		},
	}, platformCredentialsSet.OwnerReferences)

	// TODO: test the Pod Template rewrite
}

//
// HELPER FUNCTIONS
//

// cleanUpStackSet deletes the StackSet with the given name.
func (suite *ConfigurationResourcesTestSuite) cleanUpStackSet(name string) {
	_ = stacksetInterface().Delete(context.Background(), name, metav1.DeleteOptions{})
}
