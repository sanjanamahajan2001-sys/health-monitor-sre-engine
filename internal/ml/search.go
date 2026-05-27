package ml

import (
	"math"
	"regexp"
	"strings"
)

// SimpleVectorizer converts text to a word-frequency vector
type SimpleVectorizer struct {
	Vocabulary map[string]int
}

func NewSimpleVectorizer(vocab []string) *SimpleVectorizer {
	m := make(map[string]int)
	for i, word := range vocab {
		m[strings.ToLower(word)] = i
	}
	return &SimpleVectorizer{Vocabulary: m}
}

func (sv *SimpleVectorizer) Vectorize(text string) Vector {
	v := make(Vector, len(sv.Vocabulary))
	words := sv.tokenize(text)
	for _, word := range words {
		if idx, ok := sv.Vocabulary[word]; ok {
			v[idx]++
		}
	}
	return v
}

func (sv *SimpleVectorizer) tokenize(text string) []string {
	re := regexp.MustCompile(`\w+`)
	matches := re.FindAllString(strings.ToLower(text), -1)
	return matches
}

// CosineSimilarity calculates similarity between two vectors
func CosineSimilarity(v1, v2 Vector) float64 {
	if len(v1) != len(v2) || len(v1) == 0 {
		return 0
	}

	var dotProduct, mag1, mag2 float64
	for i := 0; i < len(v1); i++ {
		dotProduct += v1[i] * v2[i]
		mag1 += v1[i] * v1[i]
		mag2 += v2[i] * v2[i]
	}

	mag1 = math.Sqrt(mag1)
	mag2 = math.Sqrt(mag2)

	if mag1 == 0 || mag2 == 0 {
		return 0
	}

	return dotProduct / (mag1 * mag2)
}
