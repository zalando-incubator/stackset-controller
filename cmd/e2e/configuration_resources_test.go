package main

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConfigurationResources(t *testing.T) {
	suite.Run(t, new(ConfigurationResourcesConfigMapsTestSuite))
	suite.Run(t, new(ConfigurationResourcesSecretsTestSuite))
	suite.Run(t, new(ConfigurationResourcesPlatformCredentialsSetTestSuite))
}

type ConfigurationResourcesTestSuite struct {
	suite.Suite

	stacksetSpecFactory *TestStacksetSpecFactory

	stacksetName string
	stackVersion string
}

type ConfigurationResourcesConfigMapsTestSuite struct {
	ConfigurationResourcesTestSuite
}

func (suite *ConfigurationResourcesConfigMapsTestSuite) SetupTest() {
	suite.stacksetName = "stackset-cr-cm"
	suite.stackVersion = "v1"

	suite.stacksetSpecFactory = NewTestStacksetSpecFactory(suite.stacksetName)
}

func (suite *ConfigurationResourcesConfigMapsTestSuite) TearDownTest() {
	err := deleteStackset(suite.stacksetName)
	suite.Require().NoError(err)
}

// TestReferencedConfigMaps tests that ConfigMaps referenced in the StackSet spec are owned by the Stack.
func (suite *ConfigurationResourcesConfigMapsTestSuite) TestReferencedConfigMaps() {
	// Create a ConfigMap in the cluster following the naming convention
	configMapName := "stackset-cr-cm-v1-my-configmap"
	createConfigMap(suite.T(), configMapName)

	// Add the ConfigMap reference to the StackSet spec
	suite.stacksetSpecFactory.AddReferencedConfigMap(configMapName)

	// Generate the StackSet spec
	stacksetSpec := suite.stacksetSpecFactory.Create(suite.T(), suite.stackVersion)

	// Create the StackSet in the cluster
	err := createStackSet(suite.stacksetName, 0, stacksetSpec)
	suite.Require().NoError(err)

	// Wait for the first Stack to be created
	stack, err := waitForStack(suite.T(), suite.stacksetName, suite.stackVersion)
	suite.Require().NoError(err)

	// Ensure that the ConfigMap exists in the cluster
	_, err = waitForConfigMap(suite.T(), configMapName)
	suite.Require().NoError(err)

	// Ensure that the ConfigMap is owned by the Stack
	ownerReferences := []metav1.OwnerReference{
		{
			APIVersion: core.APIVersion,
			Kind:       core.KindStack,
			Name:       stack.Name,
			UID:        stack.UID,
		},
	}
	err = waitForConfigMapOwnerReferences(suite.T(), configMapName, ownerReferences).await()
	suite.Require().NoError(err)
}

func (suite *ConfigurationResourcesSecretsTestSuite) SetupTest() {
	suite.stacksetName = "stackset-cr-sec"
	suite.stackVersion = "v1"

	suite.stacksetSpecFactory = NewTestStacksetSpecFactory(suite.stacksetName)
}

func (suite *ConfigurationResourcesSecretsTestSuite) TearDownTest() {
	err := deleteStackset(suite.stacksetName)
	suite.Require().NoError(err)
}

type ConfigurationResourcesSecretsTestSuite struct {
	ConfigurationResourcesTestSuite
}

// TestReferencedSecrets tests that Secrets referenced in the StackSet spec are owned by the Stack.
func (suite *ConfigurationResourcesSecretsTestSuite) TestReferencedSecrets() {
	// Create a Secret in the cluster following the naming convention
	secretName := "stackset-cr-sec-v1-my-secret"
	createSecret(suite.T(), secretName)

	// Add the Secret reference to the StackSet spec
	suite.stacksetSpecFactory.AddReferencedSecret(secretName)

	// Generate the StackSet spec
	stacksetSpec := suite.stacksetSpecFactory.Create(suite.T(), suite.stackVersion)

	// Create the StackSet in the cluster
	err := createStackSet(suite.stacksetName, 0, stacksetSpec)
	suite.Require().NoError(err)

	// Wait for the first Stack to be created
	stack, err := waitForStack(suite.T(), suite.stacksetName, suite.stackVersion)
	suite.Require().NoError(err)

	// Ensure that the Secret exists in the cluster
	_, err = waitForSecret(suite.T(), secretName)
	suite.Require().NoError(err)

	// Ensure that the Secret is owned by the Stack
	ownerReferences := []metav1.OwnerReference{
		{
			APIVersion: core.APIVersion,
			Kind:       core.KindStack,
			Name:       stack.Name,
			UID:        stack.UID,
		},
	}
	err = waitForSecretOwnerReferences(suite.T(), secretName, ownerReferences).await()
	suite.Require().NoError(err)
}

type ConfigurationResourcesPlatformCredentialsSetTestSuite struct {
	ConfigurationResourcesTestSuite
}

func (suite *ConfigurationResourcesPlatformCredentialsSetTestSuite) SetupTest() {
	suite.stacksetName = "stackset-cr-pcs"
	suite.stackVersion = "v1"

	suite.stacksetSpecFactory = NewTestStacksetSpecFactory(suite.stacksetName)
}

func (suite *ConfigurationResourcesPlatformCredentialsSetTestSuite) TearDownTest() {
	err := deleteStackset(suite.stacksetName)
	suite.Require().NoError(err)
}

// TestGeneratedPCS tests that PlatformCredentialsSets defined in the StackSet are
// correctly created and owned by the Stack.
func (suite *ConfigurationResourcesPlatformCredentialsSetTestSuite) TestGeneratedPCS() {
	// Add the PlatformCredentialsSet reference to the StackSet spec
	pcsName := suite.stacksetName + "-" + suite.stackVersion + "-my-pcs"
	suite.stacksetSpecFactory.AddPlatformCredentialsSetDefinition(pcsName)

	// Generate the StackSet spec
	stacksetSpec := suite.stacksetSpecFactory.Create(suite.T(), suite.stackVersion)

	// Create the StackSet in the cluster
	err := createStackSet(suite.stacksetName, 0, stacksetSpec)
	suite.Require().NoError(err)

	// Wait for the first Stack to be created
	stack, err := waitForStack(suite.T(), suite.stacksetName, suite.stackVersion)
	suite.Require().NoError(err)

	// Fetch the latest version of the PlatformCredentialsSet
	pcs, err := waitForPlatformCredentialsSet(suite.T(), pcsName)
	suite.Require().NoError(err)

	// Ensure that the PlatformCredentialsSet is owned by the Stack
	suite.Equal([]metav1.OwnerReference{
		{
			APIVersion: core.APIVersion,
			Kind:       core.KindStack,
			Name:       stack.Name,
			UID:        stack.UID,
		},
	}, pcs.OwnerReferences)

	suite.Equal(stack.Labels["application"], pcs.Spec.Application)
	suite.Equal("v2", pcs.Spec.TokenVersion)
	suite.Equal([]string{"read"}, pcs.Spec.Tokens["token-example"].Privileges)
}
