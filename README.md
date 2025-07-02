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
