package dictparser

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dawit909/sendtoanki/src/constants"
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
	Etymology   []string `json:"etymolgy"`
	Note        string   `json:"note"`
	FFWords     []string `json:"ff_words"`
	Definitions []Definition
}

type Definition struct {
	EntryID      int
	PartOfSpeech string
	Meaning      string
	Subs         []string
}

const noOfEntries = 105152

var DictMap = make(map[string]*WordEntry, noOfEntries)

func ParseJSON() ([]WordEntry, error) {
	// Open the JSON file
	jsonPath := filepath.Join(constants.ROOT, "resources", constants.JSON_FILENAME)
	file, err := os.Open(jsonPath)
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

func init() {
	if err := initJsonMap(); err != nil {
		panic(err)
	}
}
func initJsonMap() error {
	dictParsed, err := ParseJSON()
	if err != nil {
		return fmt.Errorf("Error parsing dictionary JSON: %w", err)
	}
	if len(dictParsed) != noOfEntries {
		return fmt.Errorf("The # of parsed words in JSON is %d, expected: %d", len(dictParsed), noOfEntries)
	}
	log.Printf("Dictionary loaded: %d entries\n", len(dictParsed))

	for i := range dictParsed {
		// Store pointer to avoid copying large structs
		DictMap[dictParsed[i].Title] = &dictParsed[i]
	}
	return nil
}
