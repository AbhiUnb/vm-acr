package test

import (
	"strings"
	"testing"

	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
)

func TestACRProvisioning(t *testing.T) {
	t.Parallel()

	// Define Terraform options
	terraformOptions := &terraform.Options{
		TerraformDir: "../",
		VarFiles:     []string{"terraform.tfvars"},
	}

	// Ensure resources are destroyed at the end
	defer terraform.Destroy(t, terraformOptions)

	// Run Terraform Init and Apply
	terraform.InitAndApply(t, terraformOptions)

	// Fetch output values
	acrName := terraform.Output(t, terraformOptions, "acr_name")
	acrLoginServer := terraform.Output(t, terraformOptions, "acr_login_server")
	resourceGroupName := terraform.Output(t, terraformOptions, "resource_group_name")
	subscriptionID := terraform.Output(t, terraformOptions, "subscription_id")

	// Option 1: Use Azure SDK directly (Recommended)
	tags := getACRTags(t, subscriptionID, resourceGroupName, acrName)

	// Option 2: Alternative using generic resource (if available in your Terratest version)
	// resource := azure.GetResource(t, subscriptionID, resourceGroupName, acrName, "Microsoft.ContainerRegistry/registries")
	// tags := resource.Tags

	// Run actual assertions
	assert.True(t, strings.HasPrefix(acrName, "acr"), "ACR name should start with 'acr'")
	assert.Contains(t, acrLoginServer, ".azurecr.io", "Login server must be a valid Azure Container Registry endpoint")

	assert.NotNil(t, tags["owner"])
	assert.Equal(t, "devops", *tags["owner"])

	assert.NotNil(t, tags["environment"])
	assert.Equal(t, "development", *tags["environment"])

	assert.NotNil(t, tags["created"])
	assert.NotEmpty(t, *tags["created"])

	// Validate RBAC role assignments on ACR
	roleAssignments := getACRRoleAssignments(t, subscriptionID, resourceGroupName, acrName)
	assert.Greater(t, len(roleAssignments), 0, "No RBAC assignments found on ACR")
}

// Helper function to get ACR tags using Azure SDK
func getACRTags(t *testing.T, subscriptionID, resourceGroupName, acrName string) map[string]*string {
	// Create Azure credential
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	assert.NoError(t, err, "Failed to create Azure credential")

	// Create Container Registry client
	client, err := armcontainerregistry.NewRegistriesClient(subscriptionID, cred, nil)
	assert.NoError(t, err, "Failed to create ACR client")

	// Get the ACR resource
	ctx := context.Background()
	acr, err := client.Get(ctx, resourceGroupName, acrName, nil)
	assert.NoError(t, err, "Failed to get ACR resource")

	return acr.Tags
}

func getACRRoleAssignments(t *testing.T, subscriptionID, resourceGroupName, acrName string) []armauthorization.RoleAssignment {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	assert.NoError(t, err, "Failed to create Azure credential")

	client, err := armauthorization.NewRoleAssignmentsClient(subscriptionID, cred, nil)
	assert.NoError(t, err, "Failed to create RoleAssignments client")

	scope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s", subscriptionID, resourceGroupName, acrName)
	pager := client.NewListForScopePager(scope, nil)

	var results []armauthorization.RoleAssignment
	ctx := context.Background()

	for pager.More() {
		page, err := pager.NextPage(ctx)
		assert.NoError(t, err, "Failed to list role assignments")
		for _, v := range page.Value {
			results = append(results, *v)
		}
	}
	return results
}
