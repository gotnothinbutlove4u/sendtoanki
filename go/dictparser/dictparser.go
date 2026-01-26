package dictparser

import (
	"encoding/json"
	"fmt"
	"os"
)

// WordEntry represents a single dictionary entry.
// The strings in `json:"..."` tags match the keys in your JSON file.
type WordEntry struct {
	Title        string `json:"title"`
	Syllable     string `json:"syllable"`
	NumSyllable  int    `json:"num_syllable"`
	IPA          string `json:"ipa"`
	Meaning      string `json:"meaning"`
	Phrases      string `json:"phrases"`
	PhrasalVerbs string `json:"phrasal_verbs"`
	Derivatives  string `json:"derivatives"`
	// Note: 'etymolgy' matches the specific spelling in your JSON source
	Etymology []string `json:"etymolgy"`
	Note      string   `json:"note"`
	FFWords   []string `json:"ff_words"`
}

func ParseJSON() ([]WordEntry, error) {
	// Open the JSON file
	file, err := os.Open("./json.json") // Replace with your actual file name
	if err != nil {
		fmt.Println("Error opening file:", err)
		return []WordEntry{}, err
	}
	defer file.Close()

	// Parse the JSON data
	var words []WordEntry
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&words)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
		return []WordEntry{}, err
	}

	return words, nil
}
