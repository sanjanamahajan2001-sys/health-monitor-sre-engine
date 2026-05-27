package incident

// This file contains the CLI command handlers for action item management

import (
	"bufio"
	"flag"
	"fmt"
	"health-monitor/internal/output"
	"os"
	"strings"
	"time"
)

// handleAction routes action item subcommands
func handleAction(service *Service, args []string) int {
	if len(args) == 0 {
		output.Errorf("Action subcommand required (add, list, update, delete)")
		return 1
	}
	
	switch args[0] {
	case "add":
		return handleActionAdd(service, args[1:])
	case "list":
		return handleActionList(service, args[1:])
	case "update":
		return handleActionUpdate(service, args[1:])
	case "delete":
		return handleActionDelete(service, args[1:])
	default:
		output.Errorf("Unknown action subcommand: %s", args[0])
		return 1
	}
}

// handleActionAdd adds a new action item to an incident
func handleActionAdd(service *Service, args []string) int {
	fs := flag.NewFlagSet("incident action add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	incidentID := fs.String("id", "", "incident ID (optional, will prompt if not provided)")
	
	if err := fs.Parse(args); err != nil {
		return 1
	}
	
	var targetIncident *Incident
	var err error
	
	// If --id flag provided, use it directly
	if *incidentID != "" {
		loaded, err := service.store.Load(strings.TrimSpace(*incidentID))
		if err != nil {
			output.Errorf("Failed to load incident: %v", err)
			return 1
		}
		targetIncident = &loaded
	} else {
		// Interactive: prompt for incident selection
		incidents, _, err := service.store.List()
		if err != nil {
			output.Errorf("Failed to list incidents: %v", err)
			return 1
		}
		
		if len(incidents) == 0 {
			output.Errorf("No incidents found")
			return 1
		}
		
		targetIncident, err = promptForIncidentSelection(incidents, true)
		if err != nil {
			output.Errorf("Failed to select incident: %v", err)
			return 1
		}
	}
	
	// Prompt for action item details
	description, owner, priority, status, dueDate, err := promptForActionItem(targetIncident)
	if err != nil {
		output.Errorf("Failed to get action item details: %v", err)
		return 1
	}
	
	// Add action item
	actionItem, err := service.AddActionItem(targetIncident.ID, description, owner, priority, status, dueDate)
	if err != nil {
		output.Errorf("Failed to add action item: %v", err)
		return 1
	}
	
	fmt.Printf("✓ Action item added: %s\n", actionItem.ID)
	return 0
}

// handleActionList lists all action items for an incident or all incidents
func handleActionList(service *Service, args []string) int {
	fs := flag.NewFlagSet("incident action list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	incidentID := fs.String("id", "", "incident ID (optional, lists all if not provided)")
	
	if err := fs.Parse(args); err != nil {
		return 1
	}
	
	// If --id provided, list for specific incident
	if *incidentID != "" {
		actionItems, err := service.ListActionItems(strings.TrimSpace(*incidentID))
		if err != nil {
			output.Errorf("Failed to list action items: %v", err)
			return 1
		}
		
		if len(actionItems) == 0 {
			fmt.Println("No action items found")
			return 0
		}
		
		fmt.Printf("Action Items for %s:\n\n", *incidentID)
		for _, item := range actionItems {
			dueDateStr := "No due date"
			if !item.DueDate.IsZero() {
				dueDateStr = fmt.Sprintf("Due: %s", item.DueDate.Format("2006-01-02"))
			}
			
			fmt.Printf("[%s] [%s] %s\n", item.Priority, item.Status, item.Description)
			fmt.Printf("    Owner: %s | %s | ID: %s\n", item.Owner, dueDateStr, item.ID)
			fmt.Println()
		}
		
		return 0
	}
	
	// No --id provided, list all action items across all incidents
	incidents, _, err := service.store.List()
	if err != nil {
		output.Errorf("Failed to list incidents: %v", err)
		return 1
	}
	
	// Collect all action items with their incident info
	type ActionItemWithIncident struct {
		Item       ActionItem
		IncidentID string
		Title      string
	}
	
	var allItems []ActionItemWithIncident
	for _, inc := range incidents {
		for _, item := range inc.ActionItems {
			allItems = append(allItems, ActionItemWithIncident{
				Item:       item,
				IncidentID: inc.ID,
				Title:      inc.Title,
			})
		}
	}
	
	if len(allItems) == 0 {
		fmt.Println("No action items found across all incidents")
		return 0
	}
	
	fmt.Printf("All Action Items (%d total):\n\n", len(allItems))
	
	for _, entry := range allItems {
		item := entry.Item
		dueDateStr := "No due date"
		if !item.DueDate.IsZero() {
			dueDateStr = fmt.Sprintf("Due: %s", item.DueDate.Format("2006-01-02"))
		}
		
		fmt.Printf("[%s] [%s] %s\n", item.Priority, item.Status, item.Description)
		fmt.Printf("    Incident: %s - %s\n", entry.IncidentID, entry.Title)
		fmt.Printf("    Owner: %s | %s | ID: %s\n", item.Owner, dueDateStr, item.ID)
		fmt.Println()
	}
	
	return 0
}

// handleActionUpdate updates an action item
func handleActionUpdate(service *Service, args []string) int {
	fs := flag.NewFlagSet("incident action update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	incidentID := fs.String("id", "", "incident ID (optional, will prompt if not provided)")
	
	if err := fs.Parse(args); err != nil {
		return 1
	}
	
	var targetIncident *Incident
	var err error
	
	// If --id flag provided, use it directly
	if *incidentID != "" {
		loaded, err := service.store.Load(strings.TrimSpace(*incidentID))
		if err != nil {
			output.Errorf("Failed to load incident: %v", err)
			return 1
		}
		targetIncident = &loaded
	} else {
		// Interactive: prompt for incident selection
		incidents, _, err := service.store.List()
		if err != nil {
			output.Errorf("Failed to list incidents: %v", err)
			return 1
		}
		
		if len(incidents) == 0 {
			output.Errorf("No incidents found")
			return 1
		}
		
		// Filter to incidents with action items
		var incidentsWithActions []Incident
		for _, inc := range incidents {
			if len(inc.ActionItems) > 0 {
				incidentsWithActions = append(incidentsWithActions, inc)
			}
		}
		
		if len(incidentsWithActions) == 0 {
			output.Errorf("No incidents with action items found")
			return 1
		}
		
		targetIncident, err = promptForIncidentSelection(incidentsWithActions, false)
		if err != nil {
			output.Errorf("Failed to select incident: %v", err)
			return 1
		}
	}
	
	if len(targetIncident.ActionItems) == 0 {
		output.Errorf("No action items found for this incident")
		return 1
	}
	
	// Prompt for action item selection
	actionItem, err := promptForActionItemSelection(targetIncident.ActionItems)
	if err != nil {
		output.Errorf("Failed to select action item: %v", err)
		return 1
	}
	
	// Prompt for field to update
	field, err := promptForUpdateField()
	if err != nil {
		output.Errorf("Failed to select field: %v", err)
		return 1
	}
	
	// Update based on field
	switch field {
	case "status":
		newStatus, err := promptForNewStatus()
		if err != nil {
			output.Errorf("Failed to get new status: %v", err)
			return 1
		}
		if err := service.UpdateActionItemStatus(targetIncident.ID, actionItem.ID, newStatus); err != nil {
			output.Errorf("Failed to update status: %v", err)
			return 1
		}
		fmt.Println("✓ Action item status updated")
		
	case "owner":
		newOwner, err := promptForNewOwner()
		if err != nil {
			output.Errorf("Failed to get new owner: %v", err)
			return 1
		}
		if err := service.UpdateActionItemOwner(targetIncident.ID, actionItem.ID, newOwner); err != nil {
			output.Errorf("Failed to update owner: %v", err)
			return 1
		}
		fmt.Println("✓ Action item owner updated")
		
	case "priority":
		// Create a scanner for input
		scanner := bufio.NewScanner(os.Stdin)
		newPriority, err := promptForPriority(scanner, actionItem.Priority)
		if err != nil {
			output.Errorf("Failed to get new priority: %v", err)
			return 1
		}
		if err := service.UpdateActionItemPriority(targetIncident.ID, actionItem.ID, newPriority); err != nil {
			output.Errorf("Failed to update priority: %v", err)
			return 1
		}
		fmt.Println("✓ Action item priority updated")
		
	case "due_date":
		// Create a scanner for input
		scanner := bufio.NewScanner(os.Stdin)
		// Use current due date as default if set, otherwise default to "now" for prompt purposes (though logic handles empty)
		defaultDate := actionItem.DueDate
		if defaultDate.IsZero() {
			defaultDate = time.Now()
		}
		
		newDate, err := promptForDate(scanner, "New due date (YYYY-MM-DD)", defaultDate)
		if err != nil {
			output.Errorf("Failed to get new due date: %v", err)
			return 1
		}
		if err := service.UpdateActionItemDueDate(targetIncident.ID, actionItem.ID, newDate); err != nil {
			output.Errorf("Failed to update due date: %v", err)
			return 1
		}
		fmt.Println("✓ Action item due date updated")

	default:
		output.Errorf("Update for field %q not valid", field)
		return 1
	}
	
	return 0
}

// handleActionDelete deletes an action item
func handleActionDelete(service *Service, args []string) int {
	fs := flag.NewFlagSet("incident action delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	incidentID := fs.String("id", "", "incident ID (optional, will prompt if not provided)")
	
	if err := fs.Parse(args); err != nil {
		return 1
	}
	
	var targetIncident *Incident
	var err error
	
	// If --id flag provided, use it directly
	if *incidentID != "" {
		loaded, err := service.store.Load(strings.TrimSpace(*incidentID))
		if err != nil {
			output.Errorf("Failed to load incident: %v", err)
			return 1
		}
		targetIncident = &loaded
	} else {
		// Interactive: prompt for incident selection
		incidents, _, err := service.store.List()
		if err != nil {
			output.Errorf("Failed to list incidents: %v", err)
			return 1
		}
		
		if len(incidents) == 0 {
			output.Errorf("No incidents found")
			return 1
		}
		
		// Filter to incidents with action items
		var incidentsWithActions []Incident
		for _, inc := range incidents {
			if len(inc.ActionItems) > 0 {
				incidentsWithActions = append(incidentsWithActions, inc)
			}
		}
		
		if len(incidentsWithActions) == 0 {
			output.Errorf("No incidents with action items found")
			return 1
		}
		
		targetIncident, err = promptForIncidentSelection(incidentsWithActions, false)
		if err != nil {
			output.Errorf("Failed to select incident: %v", err)
			return 1
		}
	}
	
	if len(targetIncident.ActionItems) == 0 {
		output.Errorf("No action items found for this incident")
		return 1
	}
	
	// Prompt for action item selection
	actionItem, err := promptForActionItemSelection(targetIncident.ActionItems)
	if err != nil {
		output.Errorf("Failed to select action item: %v", err)
		return 1
	}
	
	// Confirm deletion
	confirm, err := promptYesNo(fmt.Sprintf("Delete action item '%s'?", actionItem.Description))
	if err != nil {
		output.Errorf("Failed to get confirmation: %v", err)
		return 1
	}
	
	if !confirm {
		fmt.Println("Deletion cancelled")
		return 0
	}
	
	// Delete action item
	if err := service.DeleteActionItem(targetIncident.ID, actionItem.ID); err != nil {
		output.Errorf("Failed to delete action item: %v", err)
		return 1
	}
	
	fmt.Println("✓ Action item deleted")
	return 0
}
