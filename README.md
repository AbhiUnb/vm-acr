package test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fetchAzureCLI runs an az cli command and returns output trimmed.
func fetchAzureCLI(args ...string) string {
	cmd := exec.Command("az", args...)
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("Azure CLI command failed: az %s\nError: %v\n", strings.Join(args, " "), err)
		return ""
	}
	return strings.TrimSpace(string(output))
}

func TestAzureFunctionApp(t *testing.T) {
	t.Parallel()

	// Fetch function app name and resource group from ENV variables
	functionAppName := os.Getenv("FUNCTION_APP_NAME")
	resourceGroup := os.Getenv("RESOURCE_GROUP")

	require.NotEmpty(t, functionAppName, "FUNCTION_APP_NAME env variable is required")
	require.NotEmpty(t, resourceGroup, "RESOURCE_GROUP env variable is required")

	t.Logf("🔧 Testing Function App: %s in RG: %s", functionAppName, resourceGroup)

	// Fetch function app URL dynamically
	functionAppURL := fetchAzureCLI("webapp", "show", "--name", functionAppName, "--resource-group", resourceGroup, "--query", "defaultHostName", "-o", "tsv")
	require.NotEmpty(t, functionAppURL, "Function App URL could not be fetched from Azure CLI")
	t.Logf("🔗 Function App URL: %s", functionAppURL)

	// 1. Validate existence
	t.Run("ValidateFunctionAppExistence", func(t *testing.T) {
		assert.NotEmpty(t, functionAppURL, "Function App URL should not be empty")
	})

	// 2. Validate hosting plan SKU
	t.Run("ValidateHostingPlanSKU", func(t *testing.T) {
		// Fetch service plan resource ID from function app
		planID := fetchAzureCLI("webapp", "show", "--name", functionAppName, "--resource-group", resourceGroup, "--query", "appServicePlanId", "-o", "tsv")
		require.NotEmpty(t, planID, "Service Plan ID could not be fetched")

		// Extract the plan name from the resource ID
		parts := strings.Split(planID, "/")
		appServicePlanName := parts[len(parts)-1]
		t.Logf("✅ Dynamically fetched App Service Plan Name: %s", appServicePlanName)

		// Validate the SKU of the App Service Plan
		sku := fetchAzureCLI("appservice", "plan", "show", "--name", appServicePlanName, "--resource-group", resourceGroup, "--query", "sku.name", "-o", "tsv")
		assert.Contains(t, []string{"Y1", "EP1", "P1v2"}, sku, "App Service Plan SKU should be Consumption, Premium, or Dedicated")
	})

	// 3. Validate deployment source
	t.Run("ValidateDeploymentSource", func(t *testing.T) {
		deploymentSource := fetchAzureCLI("functionapp", "deployment", "source", "show", "--name", functionAppName, "--resource-group", resourceGroup, "--query", "repoUrl", "-o", "tsv")
		if deploymentSource == "" {
			t.Log("⚠️ Deployment source not configured; skipping")
		} else {
			t.Logf("🔗 Deployment Source: %s", deploymentSource)
		}
	})

	// 4. Validate App Settings (runtime and Application Insights)
	t.Run("ValidateAppSettings", func(t *testing.T) {
		runtime := fetchAzureCLI("webapp", "config", "show", "--name", functionAppName, "--resource-group", resourceGroup, "--query", "linuxFxVersion", "-o", "tsv")
		if runtime == "" {
			// fallback for Windows .NET apps
			runtime = fetchAzureCLI("webapp", "config", "show", "--name", functionAppName, "--resource-group", resourceGroup, "--query", "netFrameworkVersion", "-o", "tsv")
		}

		assert.NotEmpty(t, runtime, "Web App runtime could not be determined")
		t.Logf("✅ Web App runtime: %s", runtime)

		// Adjust runtime validation based on actual deployment expectations
		if strings.Contains(strings.ToLower(runtime), "dotnet") || strings.Contains(strings.ToLower(runtime), "net") || strings.Contains(strings.ToLower(runtime), ".net") {
			t.Log("✅ Detected .NET runtime")
		} else if strings.Contains(strings.ToLower(runtime), "node") {
			t.Log("✅ Detected Node.js runtime")
		} else {
			t.Logf("⚠️ Detected other runtime: %s", runtime)
		}

		insightsName := "ai-" + functionAppName
		insightsKey := fetchAzureCLI("resource", "show", "--name", insightsName, "--resource-group", resourceGroup, "--resource-type", "Microsoft.Insights/components", "--query", "instrumentationKey", "-o", "tsv")
		assert.NotEmpty(t, insightsKey, fmt.Sprintf("Application Insights key for %s should exist", insightsName))
	})

	// 5. Validate trigger behavior (HTTP test)
	/*
		t.Run("ValidateTriggerBehavior", func(t *testing.T) {
			triggerName := "httpexample"
			testName := "Terratest"

			testURL := functionAppURL
			if !strings.HasPrefix(testURL, "http") {
				testURL = "https://" + testURL
			}
			testURL += "/api/" + triggerName + "?name=" + testName

			resp, err := http.Get(testURL)
			require.NoError(t, err, "HTTP request to function should not error")
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected HTTP 200 OK")
			assert.Contains(t, string(body), "Hello, "+testName+"!", "Response should greet the test name")
		})
	*/
}
test file


main.tf



# # resource "azurerm_app_service_plan" "MFDMCCASPAFUNC" {
# #   name                = "MFDMCCPRODASPAFUNC"
# #   resource_group_name = var.cc_core_resource_group_name
# #   location            = var.cc_location
# #   kind                = "FunctionApp"
# #   sku {
# #     tier = "PremiumV2"
# #     size = "P1v2"
# #   }
# # }


resource "azurerm_service_plan" "MFDMCCASPAFUNC" {
  resource_group_name = var.cc_core_resource_group_name
  location            = var.cc_location
  name                = "MFDMCCPRODASPAFUNC"
  os_type             = "Linux"
  sku_name            = "Y1"
  tags                = local.tag_list_1
}

module "avm-res-web-site" {
  source              = "Azure/avm-res-web-site/azurerm"
  version             = "0.16.4"
  for_each            = local.functionapp
  name                = each.value.name
  resource_group_name = var.cc_core_resource_group_name
  location            = var.cc_location

  kind = each.value.kind

  # Uses an existing app service plan
  os_type                  = azurerm_service_plan.MFDMCCASPAFUNC.os_type
  service_plan_resource_id = azurerm_service_plan.MFDMCCASPAFUNC.id

  # Uses an existing storage account
  storage_account_name       = each.value.storage_account_name
  storage_account_access_key = each.value.storage_account_access_key
  
  # storage_uses_managed_identity = true
  site_config = {
    always_on = false
  }

  tags = local.tag_list_1


}

# resource "azurerm_linux_function_app" "my_function" {
#   name                = "my-node-function-app1"
#   resource_group_name = var.cc_core_resource_group_name
#   location            = var.cc_location
#   service_plan_id     = azurerm_service_plan.MFDMCCASPAFUNC.id

#   storage_account_name       = "mfmdiccprodfunctionsa"
#   storage_account_access_key = 
#   site_config {
#     always_on = false

#     application_stack {
#       node_version = "18"
#     }

#     //function_app_timeout = "PT5M"
#   }

#   identity {
#     type = "SystemAssigned"
#   }

#   tags = local.tag_list_1
# }

output
output "cc_location" {
  value       = var.cc_location
  description = "Canada Central Region"
}

output "cc_core_resource_group_name" {
  value       = var.cc_core_resource_group_name
  description = "Resource Group Name for McCain Foods Manufacturing Digital Shared Azure Components in Canada Central"
}

output "MF_DM_CC_CORE_appSP_Name" {
  value       = var.MF_DM_CC_CORE_appSP_Name
  description = "Web App Service Plan name for McCain Foods MF Digital Canada Central"
}

output "MF_DM_CC_CORE_Webapp_Name" {
  value       = var.MF_DM_CC_CORE_Webapp_Name
  description = "Web App name for McCain Foods MF Digital Canada Central"
}

output "function_app_url" {
  value       = module.avm-res-web-site["MF-MDI-CC-GHPROD-DDDS-AFUNC"].resource_uri
  description = "The default hostname of the Azure Function App"
}

output "function_app_insights" {
  value       = module.avm-res-web-site["MF-MDI-CC-GHPROD-DDDS-AFUNC"].application_insights
  description = "Application Insights resource details for the Function App"
  sensitive   = true
}

tfvarzs


cc_location = "canadacentral"

cc_core_resource_group_name = "rg-mccain-core-prod"

MF_DM_CC_CORE_appSP_Name = "MFDMCCPRODASPAFUNCie"

MF_DM_CC_CORE_Webapp_Name = "MFDMCCPRODFUNCTIONAPP"

cc_core_function_apps = {
  myfunctionapp = {
    name                        = "MFDMCCPRODFUNCTIONAPP"
    location                    = "canadacentral"
    os_type                     = "Linux"
    storage_account_name        = "mfmdiccprodsa"
    storage_account_access_key  = "your-storage-account-access-key"
    storage_account_rg          = "rg-mccain-core-prod"
    network_name                = "mccain-vnet-prod"
    subnet_name                 = "function-subnet"
    user_assigned_identity_name = "mccain-func-identity"
    user_assigned_identity_rg   = "rg-mccain-core-prod"
    app_insights_name           = "mccain-func-appinsights"
    app_insights_rg             = "rg-mccain-core-prod"
    key_vault_name              = "mccain-keyvault-prod"


    additional_app_settings = {
      WEBSITE_RUN_FROM_PACKAGE = "https://storageaccount.blob.core.windows.net/container/package.zip?<sas_token>"
      # Example Key Vault reference syntax
     
    }
  }
}

variable
variable "cc_location" {
  type        = string
  description = "Canada Central Region"
}

variable "cc_core_resource_group_name" {
  type        = string
  description = "Resource Group Name for McCain Foods Manufacturing Digital Shared Azure Components in Canada Central"
}


variable "MF_DM_CC_CORE_appSP_Name" {
  description = "Web App Service Plan name for McCain Foods MF Digital Canada Central"
  type        = string
}

variable "MF_DM_CC_CORE_Webapp_Name" {
  description = "Web App name for McCain Foods MF Digital Canada Central"
  type        = string
}

variable "cc_core_function_apps" {
  type        = map(any)
  description = "Map of Function Apps with their configuration"
}


locals.tf
locals {
  tag_list_1 = {
    "Application Name" = "McCain DevSecOps"
    "GL Code"          = "N/A"
    "Environment"      = "sandbox"
    "IT Owner"         = "mccain-azurecontributor@mccain.ca"
    "Onboard Date"     = "12/19/2024"
    "Modified Date"    = "N/A"
    "Organization"     = "McCain Foods Limited"
    "Business Owner"   = "trilok.tater@mccain.ca"
    "Implemented by"   = "trilok.tater@mccain.ca"
    "Resource Owner"   = "trilok.tater@mccain.ca"
    "Resource Posture" = "Private"
    "Resource Type"    = "Terraform POC"
    "Built Using"      = "Terraform"
  }


  functionapp = {
    MF-MDI-CC-GHPROD-DDDS-AFUNC = {
      name                       = "MF-MDI-CC-GHPROD-DDDS-AFUNCie"
      kind                       = "functionapp"
      storage_account_name       = "mfmdiccprodfunctionsa"
      storage_account_access_key = 
    }
  }

  applicationinsights = {
  ai-MF-MDI-CC-GHPROD-DDDS-AFUNC = {
    name                = "ai-MF-MDI-CC-GHPROD-DDDS-AFUNCie"
    location            = "canadacentral"
    resource_group_name = "rg-mccain-core-prod"
    application_type    = "web"
  }
}




----------
@startuml
title **Azure VM Stop/Start Automation Roadmap with Challenges**

skinparam note {
  BackgroundColor #FDF6E3
  BorderColor #586E75
}

skinparam title {
  BackgroundColor #268BD2
  FontColor white
}

start

:🔷 Phase 0: Goal Clarification;
note right
🎯 Objective: Automate Excel reports recommending VM stop/start times
✅ Design: Azure Monitor + Azure Function (no Log Analytics)
end note

:🔷 Phase 1: Environment Setup;
note right
📝 Tasks:
- Create test VM (B1S x64)
- Enable guest diagnostics

⚠️ Challenges:
- ARM64 image incompatibility
- Free tier regional quota limits
end note

:🔷 Phase 2: Metric Verification;
note right
📝 Tasks:
- Verify metrics in Azure Monitor
  • CPU
  • Memory (guest agent)
  • Disk IO
  • Network

⚠️ Challenges:
- Metrics delayed 5-10 mins post-deployment
- Missing guest agent blocks memory/disk metrics
end note

:🔷 Phase 3: Azure Function Setup;
note right
📝 Tasks:
- Deploy Python Function App
- Enable managed identity
- Assign Monitoring Reader role

⚠️ Challenges:
- VNet integration issues
- RBAC permission propagation delays
end note

:🔷 Phase 4: Develop Function Logic;
note right
📝 Tasks:
- MVP: Query CPU metric, return JSON
- Full: Process all metrics, apply thresholds
- Generate Excel report with pandas/openpyxl

⚠️ Challenges:
- API rate limits with many VMs
- Function timeout on large datasets
end note

:🔷 Phase 5: Testing;
note right
📝 Tasks:
- Unit tests (metric queries)
- End-to-end tests (Excel output)

⚠️ Challenges:
- Data gaps if metrics missing
- Timezone consistency (UTC vs local business hours)
end note

:🔷 Phase 6: Optimization & Security;
note right
📝 Tasks:
- Optimize API calls
- Secure HTTP trigger endpoint

⚠️ Challenges:
- Managed identity secret rotation (if used)
- Key Vault integration if secrets introduced
end note

:🔷 Phase 7: Production Scaling;
note right
📝 Tasks:
- Scale to multiple VMs dynamically
- Integrate email delivery (SendGrid/Graph API)
- Document and seek governance approvals

⚠️ Challenges:
- API call limits at scale
- Approvals for automated stop/start actions
end note

stop

@enduml





}

