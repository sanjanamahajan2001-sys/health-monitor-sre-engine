package incident

import (
	"bufio"
	"fmt"
	"health-monitor/internal/audit"
	"os"
	"strconv"
	"strings"
	"time"
)

// promptForActionItem prompts the user for all action item fields interactively
// incident can be nil, in which case no smart defaults are used
func promptForActionItem(incident *Incident) (description, owner string, priority ActionItemPriority, status ActionItemStatus, dueDate time.Time, err error) {
	scanner := bufio.NewScanner(os.Stdin)

	// Calculate defaults
	defaultOwner := ""
	defaultPriority := ActionPriorityP2
	defaultDueDate := time.Now().AddDate(0, 0, 3) // Default P2 = +3 days

	if incident != nil {
		// Default owner from incident history
		defaultOwner = getIncidentOwner(incident)
		
		// Default priority maps to incident severity
		switch incident.Severity {
		case P1:
			defaultPriority = ActionPriorityP1
		case P2:
			defaultPriority = ActionPriorityP2
		case P3:
			defaultPriority = ActionPriorityP3
		case P4:
			defaultPriority = ActionPriorityP4
		}
		
		// Default due date based on priority
		var days int
		switch defaultPriority {
		case ActionPriorityP1:
			days = 1
		case ActionPriorityP2:
			days = 3
		case ActionPriorityP3:
			days = 7
		case ActionPriorityP4:
			days = 14
		default:
			days = 3
		}
		defaultDueDate = time.Now().AddDate(0, 0, days)
	}
	
	// Prompt for description
	fmt.Print("Description: ")
	if !scanner.Scan() {
		return "", "", "", "", time.Time{}, fmt.Errorf("failed to read description")
	}
	description = strings.TrimSpace(scanner.Text())
	if description == "" {
		return "", "", "", "", time.Time{}, fmt.Errorf("description cannot be empty")
	}
	
	// Prompt for owner
	ownerPrompt := "Owner"
	if defaultOwner != "" {
		ownerPrompt = fmt.Sprintf("Owner [%s]", defaultOwner)
	}
	fmt.Printf("%s: ", ownerPrompt)
	if !scanner.Scan() {
		return "", "", "", "", time.Time{}, fmt.Errorf("failed to read owner")
	}
	owner = strings.TrimSpace(scanner.Text())
	if owner == "" {
		if defaultOwner != "" {
			owner = defaultOwner
		} else {
			return "", "", "", "", time.Time{}, fmt.Errorf("owner cannot be empty")
		}
	}
	
	// Prompt for priority
	priority, err = promptForPriority(scanner, defaultPriority)
	if err != nil {
		return "", "", "", "", time.Time{}, err
	}
	
	// Prompt for due date
	dueDate, err = promptForDate(scanner, "Due date (YYYY-MM-DD)", defaultDueDate)
	if err != nil {
		return "", "", "", "", time.Time{}, err
	}
	
	// Prompt for status (with default)
	status, err = promptForStatus(scanner)
	if err != nil {
		return "", "", "", "", time.Time{}, err
	}
	
	return description, owner, priority, status, dueDate, nil
}

// getIncidentOwner attempts to find the most relevant owner from incident events
func getIncidentOwner(incident *Incident) string {
	if incident == nil {
		return ""
	}
	
	// 1. Check for Ack event (most recent)
	for i := len(incident.Events) - 1; i >= 0; i-- {
		if incident.Events[i].Type == EventAck {
			return parseUser(incident.Events[i].User)
		}
	}
	
	// 2. Check for Start event
	for i := len(incident.Events) - 1; i >= 0; i-- {
		if incident.Events[i].Type == EventStart {
			return parseUser(incident.Events[i].User)
		}
	}
	
	// 3. Fallback to current system user if no events
	return ""
}

// parseUser extracts the username from composed "user (system: ...)" strings
func parseUser(userStr string) string {
	if idx := strings.Index(userStr, " ("); idx != -1 {
		return strings.TrimSpace(userStr[:idx])
	}
	return strings.TrimSpace(userStr)
}

// promptForIncidentSelection prompts the user to select an incident from a list
func promptForIncidentSelection(incidents []Incident, allowActive bool) (*Incident, error) {
	if len(incidents) == 0 {
		return nil, fmt.Errorf("no incidents available")
	}
	
	scanner := bufio.NewScanner(os.Stdin)
	
	if allowActive {
		fmt.Println("Select incident (or press Enter for active):")
	} else {
		fmt.Println("Select incident:")
	}
	
	for i, inc := range incidents {
		fmt.Printf("  %d) %s - %s (%s)\n", i+1, inc.ID, inc.Title, inc.State)
	}
	fmt.Print("> ")
	
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read selection")
	}
	
	input := strings.TrimSpace(scanner.Text())
	if input == "" && allowActive {
		// Return first active incident
		for i := range incidents {
			if incidents[i].State != StateResolved {
				return &incidents[i], nil
			}
		}
		return nil, fmt.Errorf("no active incident found")
	}
	
	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(incidents) {
		return nil, fmt.Errorf("invalid selection")
	}
	
	return &incidents[selection-1], nil
}

// promptForActionItemSelection prompts the user to select an action item from a list
func promptForActionItemSelection(items []ActionItem) (*ActionItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no action items available")
	}
	
	scanner := bufio.NewScanner(os.Stdin)
	
	fmt.Println("Select action item to update:")
	for i, item := range items {
		dueDateStr := ""
		if !item.DueDate.IsZero() {
			dueDateStr = fmt.Sprintf(", Due: %s", item.DueDate.Format("2006-01-02"))
		}
		fmt.Printf("  %d) [%s] [%s] %s (Owner: %s%s)\n", 
			i+1, item.Priority, item.Status, item.Description, item.Owner, dueDateStr)
	}
	fmt.Print("> ")
	
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read selection")
	}
	
	selection, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || selection < 1 || selection > len(items) {
		return nil, fmt.Errorf("invalid selection")
	}
	
	return &items[selection-1], nil
}

// promptForStatus prompts for action item status with validation
func promptForStatus(scanner *bufio.Scanner) (ActionItemStatus, error) {
	fmt.Print("Status (TODO/IN_PROGRESS/DONE/WONT_FIX) [TODO]: ")
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read status")
	}
	
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return ActionItemTODO, nil // Default
	}
	
	status, err := audit.ParseActionItemStatus(input)
	if err != nil {
		return "", err
	}
	
	return status, nil
}

// promptForPriority prompts for action item priority with validation
func promptForPriority(scanner *bufio.Scanner, defaultPriority ActionItemPriority) (ActionItemPriority, error) {
	fmt.Printf("Priority (P1/P2/P3/P4) [%s]: ", defaultPriority)
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read priority")
	}
	
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return defaultPriority, nil
	}
	
	priority, err := audit.ParseActionItemPriority(input)
	if err != nil {
		return "", err
	}
	
	return priority, nil
}

// promptForDate prompts for a date with validation
func promptForDate(scanner *bufio.Scanner, prompt string, defaultDate time.Time) (time.Time, error) {
	defaultStr := defaultDate.Format("2006-01-02")
	fmt.Printf("%s [%s]: ", prompt, defaultStr)
	
	if !scanner.Scan() {
		return time.Time{}, fmt.Errorf("failed to read date")
	}
	
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return defaultDate, nil
	}
	
	// Parse date in YYYY-MM-DD format
	date, err := time.Parse("2006-01-02", input)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format (use YYYY-MM-DD)")
	}
	
	return date, nil
}

// promptYesNo prompts for a yes/no answer
func promptYesNo(question string) (bool, error) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("%s (y/n): ", question)
	
	if !scanner.Scan() {
		return false, fmt.Errorf("failed to read input")
	}
	
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes", nil
}

// promptForUpdateField prompts the user to select which field to update
func promptForUpdateField() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	
	fmt.Println("What do you want to update?")
	fmt.Println("  1) Status")
	fmt.Println("  2) Owner")
	fmt.Println("  3) Priority")
	fmt.Println("  4) Due date")
	fmt.Print("> ")
	
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read selection")
	}
	
	selection := strings.TrimSpace(scanner.Text())
	switch selection {
	case "1":
		return "status", nil
	case "2":
		return "owner", nil
	case "3":
		return "priority", nil
	case "4":
		return "due_date", nil
	default:
		return "", fmt.Errorf("invalid selection")
	}
}

// promptForNewStatus prompts for a new status value
func promptForNewStatus() (ActionItemStatus, error) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("New status (TODO/IN_PROGRESS/DONE/WONT_FIX): ")
	
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read status")
	}
	
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return "", fmt.Errorf("status cannot be empty")
	}
	
	return audit.ParseActionItemStatus(input)
}

// promptForNewOwner prompts for a new owner value
func promptForNewOwner() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("New owner: ")
	
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read owner")
	}
	
	owner := strings.TrimSpace(scanner.Text())
	if owner == "" {
		return "", fmt.Errorf("owner cannot be empty")
	}
	
	return owner, nil
}
