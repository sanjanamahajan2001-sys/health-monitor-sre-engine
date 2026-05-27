package scorecard

import (
	"flag"
	"fmt"
)

func HandleScorecardCommand(args []string) int {
	fs := flag.NewFlagSet("scorecard", flag.ContinueOnError)
	profile := fs.String("profile", "", "skip selection and use this profile")
	org := fs.Bool("org", false, "view organization-level aggregated scorecard")
	env := fs.String("env", "", "filter organization view by environment (e.g., prod, staging)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	
	if err := RunTUI(*profile, *org, *env); err != nil {
		fmt.Printf("Error running scorecard: %v\n", err)
		return 1
	}
	return 0
}
