package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/firehydrant/terraform-provider-firehydrant/firehydrant"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceRunbook() *schema.Resource {
	return &schema.Resource{
		CreateContext: createResourceFireHydrantRunbook,
		UpdateContext: updateResourceFireHydrantRunbook,
		ReadContext:   readResourceFireHydrantRunbook,
		DeleteContext: deleteResourceFireHydrantRunbook,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			// Required
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"type": {
				Type:     schema.TypeString,
				Required: true,
			},

			// Optional
			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"owner_id": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"severities": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"steps": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// Required
						"action_id": {
							Type:     schema.TypeString,
							Required: true,
						},
						"name": {
							Type:     schema.TypeString,
							Required: true,
						},

						// Optional
						"automatic": {
							Type:     schema.TypeBool,
							Optional: true,
						},
						"config": {
							Type:     schema.TypeMap,
							Optional: true,
						},
						"repeats": {
							Type:     schema.TypeBool,
							Optional: true,
						},
						"repeats_duration": {
							Type:     schema.TypeString,
							Optional: true,
						},

						// Computed
						"step_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func readResourceFireHydrantRunbook(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Get the API client
	firehydrantAPIClient := m.(firehydrant.Client)

	// Get the runbook
	runbookID := d.Id()
	tflog.Debug(ctx, fmt.Sprintf("Read runbook: %s", runbookID), map[string]interface{}{
		"id": runbookID,
	})
	runbookResponse, err := firehydrantAPIClient.Runbooks().Get(ctx, runbookID)
	if err != nil {
		if errors.Is(err, firehydrant.ErrorNotFound) {
			tflog.Debug(ctx, fmt.Sprintf("Runbook %s no longer exists", runbookID), map[string]interface{}{
				"id": runbookID,
			})
			d.SetId("")
			return nil
		}
		return diag.Errorf("Error reading runbook %s: %v", runbookID, err)
	}

	// Gather values from API response
	attributes := map[string]interface{}{
		"name":        runbookResponse.Name,
		"description": runbookResponse.Description,
		"type":        runbookResponse.Type,
	}

	var ownerID string
	if runbookResponse.Owner != nil {
		ownerID = runbookResponse.Owner.ID
	}
	attributes["owner_id"] = ownerID

	steps := make([]interface{}, len(runbookResponse.Steps))
	for index, currentStep := range runbookResponse.Steps {
		stepConfig := map[string]interface{}{}
		for key, value := range currentStep.Config {
			stepConfig[key] = value
		}

		steps[index] = map[string]interface{}{
			"step_id":   currentStep.StepID,
			"name":      currentStep.Name,
			"action_id": currentStep.ActionID,
			"config":    stepConfig,
			"automatic": currentStep.Automatic,
		}
	}
	attributes["steps"] = steps

	severities := make([]interface{}, len(runbookResponse.Severities))
	for index, currentSeverity := range runbookResponse.Severities {
		severities[index] = map[string]interface{}{
			"id": currentSeverity.ID,
		}
	}
	attributes["severities"] = severities

	// Set the resource attributes to the values we got from the API
	for key, value := range attributes {
		if err := d.Set(key, value); err != nil {
			return diag.Errorf("Error setting %s for runbook %s: %v", key, runbookID, err)
		}
	}

	return diag.Diagnostics{}
}

func createResourceFireHydrantRunbook(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Get the API client
	firehydrantAPIClient := m.(firehydrant.Client)

	// Get attributes from config and construct the create request
	createRequest := firehydrant.CreateRunbookRequest{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
		Type:        d.Get("type").(string),
	}

	// Process any optional attributes and add to the create request if necessary
	if ownerID, ok := d.GetOk("owner_id"); ok && ownerID.(string) != "" {
		createRequest.Owner = &firehydrant.RunbookTeam{ID: ownerID.(string)}
	}

	steps := d.Get("steps").([]interface{})
	for _, currentStep := range steps {
		step := currentStep.(map[string]interface{})

		createRequest.Steps = append(createRequest.Steps, firehydrant.RunbookStep{
			Name:      step["name"].(string),
			ActionID:  step["action_id"].(string),
			Automatic: step["automatic"].(bool),
			Config:    convertStringMap(step["config"].(map[string]interface{})),
		})
	}

	severities := d.Get("severities").([]interface{})
	for _, severity := range severities {
		currentSeverity := severity.(map[string]interface{})

		createRequest.Severities = append(createRequest.Severities, firehydrant.RunbookRelation{
			ID: currentSeverity["id"].(string),
		})
	}

	// Create the new runbook
	tflog.Debug(ctx, fmt.Sprintf("Create runbook: %s", createRequest.Name), map[string]interface{}{
		"name": createRequest.Name,
	})
	runbookResponse, err := firehydrantAPIClient.Runbooks().Create(ctx, createRequest)
	if err != nil {
		return diag.Errorf("Error creating runbook %s: %v", createRequest.Name, err)
	}

	// Set the new runbook's ID in state
	d.SetId(runbookResponse.ID)

	// Update state with the latest information from the API
	return readResourceFireHydrantRunbook(ctx, d, m)
}

func updateResourceFireHydrantRunbook(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Get the API client
	firehydrantAPIClient := m.(firehydrant.Client)

	// Construct the update request
	updateRequest := firehydrant.UpdateRunbookRequest{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
	}

	// Process any optional attributes and add to the update request if necessary
	ownerID, ownerIDSet := d.GetOk("owner_id")
	if ownerIDSet {
		updateRequest.Owner = &firehydrant.RunbookTeam{ID: ownerID.(string)}
	}

	steps := d.Get("steps").([]interface{})
	for _, currentStep := range steps {
		step := currentStep.(map[string]interface{})

		updateRequest.Steps = append(updateRequest.Steps, firehydrant.RunbookStep{
			Name:      step["name"].(string),
			ActionID:  step["action_id"].(string),
			Automatic: step["automatic"].(bool),
			Config:    convertStringMap(step["config"].(map[string]interface{})),
		})
	}

	severities := d.Get("severities").([]interface{})
	for _, currentSeverity := range severities {
		severity := currentSeverity.(map[string]interface{})

		updateRequest.Severities = append(updateRequest.Severities, firehydrant.RunbookRelation{
			ID: severity["id"].(string),
		})
	}

	// Update the runbook
	tflog.Debug(ctx, fmt.Sprintf("Update runbook: %s", d.Id()), map[string]interface{}{
		"id": d.Id(),
	})
	_, err := firehydrantAPIClient.Runbooks().Update(ctx, d.Id(), updateRequest)
	if err != nil {
		return diag.Errorf("Error updating runbook %s: %v", d.Id(), err)
	}

	// Update state with the latest information from the API
	return readResourceFireHydrantRunbook(ctx, d, m)
}

func deleteResourceFireHydrantRunbook(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Get the API client
	firehydrantAPIClient := m.(firehydrant.Client)

	// Delete the runbook
	runbookID := d.Id()
	tflog.Debug(ctx, fmt.Sprintf("Delete runbook: %s", runbookID), map[string]interface{}{
		"id": runbookID,
	})
	err := firehydrantAPIClient.Runbooks().Delete(ctx, runbookID)
	if err != nil {
		if errors.Is(err, firehydrant.ErrorNotFound) {
			return nil
		}
		return diag.Errorf("Error deleting runbook %s: %v", runbookID, err)
	}

	return diag.Diagnostics{}
}
