package incident

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// promptForResolveDetails prompts the user for incident resolution details
func promptForResolveDetails() (summary, rootCause, fix, component, category, dependency, failureType, prevention string, whatWentWell, whatCouldBeBetter, whereWeGotLucky string, downtime int, toilMinutes int, toilCategory string, err error) {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Interactive Resolve Mode")
	fmt.Println("------------------------")

	// Summary (Required)
	for {
		fmt.Print("Resolution Summary: ")
		if !scanner.Scan() {
			return "", "", "", "", "", "", "", "", "", "", "", 0, 0, "", fmt.Errorf("failed to read summary")
		}
		summary = strings.TrimSpace(scanner.Text())
		if summary != "" {
			break
		}
		fmt.Println("Summary is required.")
	}

	// Root Cause
	fmt.Print("Root Cause: ")
	if scanner.Scan() {
		rootCause = strings.TrimSpace(scanner.Text())
	}

	// Fix
	fmt.Print("Fix Summary: ")
	if scanner.Scan() {
		fix = strings.TrimSpace(scanner.Text())
	}

	// Component
	fmt.Print("Affected Component (e.g., postgres_db): ")
	if scanner.Scan() {
		component = strings.TrimSpace(scanner.Text())
	}

	// Category (Select)
	category, err = promptForCategorySelect(scanner)
	if err != nil {
		return "", "", "", "", "", "", "", "", "", "", "", 0, 0, "", err
	}

	// Dependency
	fmt.Print("Dependency (e.g., api->db): ")
	if scanner.Scan() {
		dependency = strings.TrimSpace(scanner.Text())
	}

	// Failure Type (Select)
	failureType, err = promptForFailureTypeSelect(scanner)
	if err != nil {
		return "", "", "", "", "", "", "", "", "", "", "", 0, 0, "", err
	}

	// Prevention
	fmt.Print("Prevention Measures: ")
	if scanner.Scan() {
		prevention = strings.TrimSpace(scanner.Text())
	}

	// Blameless Postmortem Fields
	fmt.Println("\nBlameless Postmortem Details (Optional)")
	fmt.Println("---------------------------------------")
	
	fmt.Print("What Went Well: ")
	if scanner.Scan() {
		whatWentWell = strings.TrimSpace(scanner.Text())
	}

	fmt.Print("What Could Be Better: ")
	if scanner.Scan() {
		whatCouldBeBetter = strings.TrimSpace(scanner.Text())
	}

	fmt.Print("Where We Got Lucky: ")
	if scanner.Scan() {
		whereWeGotLucky = strings.TrimSpace(scanner.Text())
	}

	fmt.Print("Estimated Downtime (minutes): ")
	if scanner.Scan() {
		downtimeStr := strings.TrimSpace(scanner.Text())
		if downtimeStr != "" {
			if d, err := strconv.Atoi(downtimeStr); err == nil {
				downtime = d
			}
		}
	}

	fmt.Println("\nToil & Manual Effort Tracking")
	fmt.Println("-----------------------------")

	fmt.Print("Toil / Manual Investigation Time (minutes): ")
	if scanner.Scan() {
		toilStr := strings.TrimSpace(scanner.Text())
		if toilStr != "" {
			if t, err := strconv.Atoi(toilStr); err == nil {
				toilMinutes = t
			}
		}
	}

	if toilMinutes > 0 {
		toilCategory, err = promptForToilCategorySelect(scanner)
		if err != nil {
			return "", "", "", "", "", "", "", "", "", "", "", 0, 0, "", err
		}
	}

	return summary, rootCause, fix, component, category, dependency, failureType, prevention, whatWentWell, whatCouldBeBetter, whereWeGotLucky, downtime, toilMinutes, toilCategory, nil
}

// promptForCategorySelect prompts users to select a valid category
func promptForCategorySelect(scanner *bufio.Scanner) (string, error) {
	fmt.Println("Incident Category:")
	for i, cat := range ValidCategories {
		fmt.Printf("  %d) %s\n", i+1, cat)
	}
	fmt.Print("Select Category (or Enter to skip): ")

	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read input")
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return "", nil // Optional
	}

	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(ValidCategories) {
		fmt.Println("Invalid selection, skipping category.")
		return "", nil
	}

	return ValidCategories[selection-1], nil
}

// promptForFailureTypeSelect prompts users to select a valid failure type
func promptForFailureTypeSelect(scanner *bufio.Scanner) (string, error) {
	fmt.Println("Failure Type:")
	for i, ft := range ValidFailureTypes {
		fmt.Printf("  %d) %s\n", i+1, ft)
	}
	fmt.Print("Select Failure Type (or Enter to skip): ")

	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read input")
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return "", nil // Optional
	}

	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(ValidFailureTypes) {
		fmt.Println("Invalid selection, skipping failure type.")
		return "", nil
	}

	return ValidFailureTypes[selection-1], nil
}

// promptForToilCategorySelect prompts users to select a valid toil category
func promptForToilCategorySelect(scanner *bufio.Scanner) (string, error) {
	fmt.Println("Toil Category:")
	for i, tc := range ValidToilCategories {
		fmt.Printf("  %d) %s\n", i+1, tc)
	}
	fmt.Print("Select Toil Category (or Enter to skip): ")

	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read input")
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return "", nil // Optional
	}

	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(ValidToilCategories) {
		fmt.Println("Invalid selection, skipping toil category.")
		return "", nil
	}

	return ValidToilCategories[selection-1], nil
}
