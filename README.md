package test

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type CcResourceGroup struct {
	Location string            `json:"location"`
	Tags     map[string]string `json:"tags"`
	Lock     *struct {
		Level string  `json:"level"`
		Notes *string `json:"notes"`
	} `json:"lock"`
}

func loadTfVars(t *testing.T) map[string]CcResourceGroup {
	data, err := os.ReadFile("../terraform.tfvars.json")
	require.NoError(t, err)

	raw := make(map[string]json.RawMessage)
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	var rawMap json.RawMessage = raw["cc_resource_groups"]
	result := make(map[string]CcResourceGroup)
	err = json.Unmarshal(rawMap, &result)
	require.NoError(t, err)

	return result
}

func TestCcRgOutputsAndLocks(t *testing.T) {
	t.Parallel()

	tfvars := loadTfVars(t)

	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../",
		VarFiles:     []string{"terraform.tfvars.json"},
	})

	// defer terraform.Destroy(t, terraformOptions)
	// terraform.InitAndApply(t, terraformOptions)

	rgOutputs := terraform.OutputMapOfObjects(t, terraformOptions, "cc_rg_outputs")
	rgLocks := terraform.OutputMap(t, terraformOptions, "cc_rg_locks")
	principalID := terraform.Output(t, terraformOptions, "current_principal_id")
	roleAssignments := terraform.OutputMapOfObjects(t, terraformOptions, "rg_role_assignments_principals")
	roleIDs := terraform.OutputMap(t, terraformOptions, "role_ids")

	for rgName, expected := range tfvars {
		t.Run(fmt.Sprintf("Validate_%s", rgName), func(t *testing.T) {
			outRaw, ok := rgOutputs[rgName]
			require.True(t, ok, "RG output missing for "+rgName)

			out, ok := outRaw.(map[string]interface{})
			require.True(t, ok, "RG output for "+rgName+" is not a map")

			name := out["name"].(string)
			location := out["location"].(string)
			id := out["id"].(string)
			tags := out["tags"].(map[string]interface{})

			// Name check (should contain logical name)
			assert.True(t, strings.Contains(strings.ToLower(name), strings.ToLower(rgName)), "Name mismatch for "+rgName)

			// Location check
			assert.Equal(t, expected.Location, location)

			// ID format check
			assert.Regexp(t, regexp.MustCompile(`^/subscriptions/.+/resourceGroups/.+`), id)

			// Tags check
			for k, v := range expected.Tags {
				tagValue, exists := tags[k]
				require.True(t, exists, fmt.Sprintf("Expected tag '%s' missing in %s", k, rgName))
				assert.Equal(t, v, tagValue)
			}

			// Lock level check
			expectedLock := ""
			if expected.Lock != nil {
				expectedLock = expected.Lock.Level
			}
			actualLock := rgLocks[rgName]

			if expectedLock == "" {
				assert.True(t, actualLock == "" || actualLock == "null", "Expected no lock for "+rgName)
			} else {
				assert.Equal(t, expectedLock, actualLock)
			}
		})

		// RBAC Security Check: Only current principal should be assigned to RG

		t.Run(fmt.Sprintf("RBACContainsCurrentPrincipal_%s", rgName), func(t *testing.T) {
			assignmentsRaw := roleAssignments[rgName]
			assignmentsList, ok := assignmentsRaw.([]interface{})
			require.True(t, ok, "Invalid RBAC assignments for "+rgName)

			found := false
			for _, entry := range assignmentsList {
				aMap := entry.(map[string]interface{})
				pid := aMap["principal_id"].(string)
				if pid == principalID {
					found = true
					break
				}
			}
			assert.True(t, found, fmt.Sprintf("Current principal %s not assigned to RG %s", principalID, rgName))
		})

		// uncomment the negative test case "RBAC_NoUnauthorizedPrincipals"  to test no unauthorized principals are assigned to a resource group
		// (i.e., only the current_principal_id is allowed) — and it will fail the test if any other principal is found.
		// t.Run("RBAC_NoUnauthorizedPrincipals", func(t *testing.T) {
		// 	for rgName, assignmentsRaw := range roleAssignments {
		// 		assignmentsList, ok := assignmentsRaw.([]interface{})
		// 		require.True(t, ok, "Invalid RBAC assignments for "+rgName)

		// 		for _, entry := range assignmentsList {
		// 			aMap := entry.(map[string]interface{})
		// 			pid := aMap["principal_id"].(string)

		// 			assert.Equal(t, principalID, pid, fmt.Sprintf(
		// 				"Unauthorized principal %s found in RG %s. Only current principal %s should have access.",
		// 				pid, rgName, principalID,
		// 			))
		// 		}
		// 	}
		// })

		// Ensure at least one role assignment exists per RG
		t.Run("RBAC_HasAtLeastOneAssignment", func(t *testing.T) {
			for rgName, assignmentsRaw := range roleAssignments {
				assignmentsList, ok := assignmentsRaw.([]interface{})
				require.True(t, ok, "Invalid RBAC assignments for "+rgName)
				assert.Greater(t, len(assignmentsList), 0, fmt.Sprintf("No RBAC assignments found for RG %s", rgName))
			}
		})

		// To Ensure current principal has Contributor role
		t.Run("CurrentPrincipalMustBeContributor", func(t *testing.T) {
			principalID := terraform.Output(t, terraformOptions, "current_principal_id")
			roleAssignments := terraform.OutputMapOfObjects(t, terraformOptions, "rg_role_assignments_principals")

			contributorIdShort := strings.ToLower(strings.TrimPrefix(roleIDs["Contributor"], "/providers/Microsoft.Authorization/roleDefinitions/"))

			fmt.Printf("Current Principal ID: %s\n", principalID)
			fmt.Printf("Contributor Role UUID: %s\n", contributorIdShort)

			for rgName, assignmentsRaw := range roleAssignments {
				fmt.Printf("Checking RG: %s\n", rgName)

				assignmentsList := assignmentsRaw.([]interface{})
				found := false
				for _, entry := range assignmentsList {
					e := entry.(map[string]interface{})
					pid := fmt.Sprintf("%v", e["principal_id"])
					rid := fmt.Sprintf("%v", e["role_definition_id"])

					fmt.Printf("  - principal_id:        %s\n", pid)
					fmt.Printf("  - role_definition_id:  %s\n", rid)

					if pid == principalID && strings.HasSuffix(strings.ToLower(rid), contributorIdShort) {
						found = true
						fmt.Printf("Match found in %s\n", rgName)
						break
					}
				}
				assert.True(t, found, fmt.Sprintf("Current principal %s must have Contributor role in %s", principalID, rgName))
			}
		})
	}
}







Hi Shreya,

As a continuation of our previous sprint where we did an R&D around leveraging Terratest for testing our Terraform-based infrastructure, we identified that until now, we were only validating the syntax of our IaC code, but the actual behavior and configuration of provisioned resources were being tested manually.

In this sprint, we focused on three major areas to strengthen our infrastructure testing approach:
	1.	Test Design Coverage Document
We created a comprehensive Test Coverage Design document for all key MDXi provisioned resources.
This document outlines major possible automated test cases for each resource type, including scenarios for validation, compliance and functional correctness for each resource. We categorised our test scenarios in 2 types static and runtime which will be better for us to 
It has been uploaded to Jira Confluence for easy access, so going forward, whenever we implement Terratest for any resource, we can reuse this design to save effort and maintain consistency.
	2.	Go-based Terratest Suites Implementation
We developed Go-based Terratest suites to automate the testing of our key MDIXI Azure resources.
As of now, we implemented automated test cases for Azure Container Registry (ACR), Azure Functions, Log Analytics with Diagnostics, and Azure WebApp modules.
Some resources like SQL MI are pending due to earlier challenges we faced with AVM module configurations. However, we have resolved these blockers and plan to complete their test coverage in the very soon.
	3.	Integration with GitHub Actions Pipeline
After developing the Automation tests, we integrated them into our GitHub Actions CI pipeline.
Now, whenever a PR is raised or updated, it automatically triggers the Terratest suites, validating the provisioned infrastructure end-to-end.
This means PR merges will be blocked if the  tests fail, ensuring that misconfigurations or policy violations are caught early in the CI/CD process before reaching production.
Ultimately, this integration enhances release confidence, improves deployment reliability, and minimizes the risk of outages due to incorrect infrastructure as code setup.

These efforts enable faster and safer Azure deployments with reduced manual testing, fewer production issues, and higher confidence in delivering secure and compliant infrastructure.
