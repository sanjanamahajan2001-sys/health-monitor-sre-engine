package incident

import "fmt"

func ValidateTransition(from State, to State) error {
	switch from {
	case "":
		if to == StateSuggested || to == StateStarted {
			return nil
		}
	case StateSuggested:
		if to == StateStarted || to == StateResolved {
			return nil
		}
	case StateStarted:
		if to == StateAcknowledged || to == StateResolved {
			return nil
		}
	case StateAcknowledged:
		if to == StateResolved {
			return nil
		}
	case StateResolved:
		return fmt.Errorf("incident already resolved")
	}
	return fmt.Errorf("invalid state transition from %q to %q", from, to)
}
