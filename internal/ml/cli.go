package ml

import (
	"flag"
	"fmt"
	"strings"
)

// HandleMLCommand handles subcommands for the 'ml' group
func HandleMLCommand(args []string) int {
	if len(args) == 0 {
		fmt.Println("Usage: health-monitor ml <predict|sim|vocab>")
		return 1
	}

	agent := NewMLAgent("v1.0.0")

	switch args[0] {
	case "predict":
		return handlePredict(agent, args[1:])
	case "sim":
		return handleSim(agent, args[1:])
	case "vocab":
		return handleVocab(agent)
	default:
		fmt.Printf("Unknown ml subcommand: %s\n", args[0])
		return 1
	}
}

func handlePredict(agent *MLAgent, args []string) int {
	fs := flag.NewFlagSet("ml predict", flag.ExitOnError)
	text := fs.String("text", "", "Text to analyze (e.g., log sample or error message)")
	fs.Parse(args)

	if *text == "" {
		fmt.Println("Error: --text is required")
		return 1
	}

	prediction := agent.Predict(*text)
	if prediction == nil {
		fmt.Println("No semantic features identified.")
		return 0
	}

	fmt.Printf("--- Semantic Analysis ---\n")
	fmt.Printf("Category   : %s\n", prediction.Category)
	fmt.Printf("Pattern    : %s\n", prediction.Pattern)
	fmt.Printf("Confidence : %.2f%%\n", prediction.Confidence*100)
	fmt.Printf("Explanation: %s\n", prediction.Explanation)
	fmt.Printf("Action     : %s\n", prediction.ResolutionHint)
	fmt.Printf("Model      : %s\n", prediction.ModelID)
	return 0
}

func handleSim(agent *MLAgent, args []string) int {
	fs := flag.NewFlagSet("ml sim", flag.ExitOnError)
	source := fs.String("source", "", "Source text")
	target := fs.String("target", "", "Target text to compare against")
	fs.Parse(args)

	if *source == "" || *target == "" {
		fmt.Println("Error: --source and --target are required")
		return 1
	}

	sims := agent.FindSimilar(*source, []string{*target})
	fmt.Printf("Semantic Similarity: %.2f%%\n", sims[0]*100)
	return 0
}

func handleVocab(agent *MLAgent) int {
	fmt.Println("Current ML Vocabulary (SRE/Ops Features):")
	fmt.Println(strings.Join(defaultVocab, ", "))
	fmt.Printf("\nTotal features: %d\n", len(defaultVocab))
	return 0
}
