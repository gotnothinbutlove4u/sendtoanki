package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"

	// "os/exec" // Exec commented out as we are using native Go parser now

	"strings"
	"sync"

	"github.com/dawit909/sendtoanki/src/constants"
	"github.com/dawit909/sendtoanki/src/dictparser"
	"github.com/dawit909/sendtoanki/src/oxforddicthandler"
	"github.com/dawit909/sendtoanki/src/sendtoanki"
	"github.com/dawit909/sendtoanki/src/tutorial"
)

// Use a mutex for thread-safe access to the global variable
var (
	processedWordsMutex sync.RWMutex
	processedWords      map[string]*oxforddicthandler.OxfordWord
)

// ViewWord struct for JSON response
type ViewWord struct {
	Stem       string   `json:"stem"`
	Definition string   `json:"definition"`
	Usage      string   `json:"usage"`
	IsBasic    bool     `json:"isBasic"`
	Books      []string `json:"books"`
}

func getProcessedWords() map[string]*oxforddicthandler.OxfordWord {
	processedWordsMutex.RLock()
	defer processedWordsMutex.RUnlock()
	return processedWords
}

func setProcessedWords(words map[string]*oxforddicthandler.OxfordWord) {
	processedWordsMutex.Lock()
	defer processedWordsMutex.Unlock()
	processedWords = words
}

func sendJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// UploadAPIHandler processes the uploaded file and returns the word list as JSON.
func UploadAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userFile, header, err := r.FormFile("vocab.db")
	if err != nil {
		sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer userFile.Close()

	tmp, err := os.CreateTemp(constants.ROOT, header.Filename)
	if err != nil {
		sendJSONError(w, "Could not save file", http.StatusInternalServerError)
		return
	}

	defer func() {
		if err := tmp.Close(); err != nil {
			log.Println(err)
			return
		}
		if err := os.Remove(tmp.Name()); err != nil {
			log.Println(err)
		}
	}()

	_, err = io.Copy(tmp, userFile)
	if err != nil {
		sendJSONError(w, fmt.Sprintf("Could not save file content: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// 1. Read User's DB (The "Words")
	rawRows, err := readEveryRowFromDB(tmp.Name())
	if err != nil {
		sendJSONError(w, fmt.Sprintf("Could not read database: %s", err.Error()), http.StatusInternalServerError)
		return
	}
	log.Printf("%d rows found in user DB\n", len(rawRows))

	// 2. Match Words (O(N) complexity now, instead of O(N*M))
	wordKeysLookedUpAlot, _ := readWordKeysWithMultipleUsagesFromDB(tmp.Name())
	var oxWords = map[string]*oxforddicthandler.OxfordWord{}

	for _, r := range rawRows {
		// FAST LOOKUP
		if entry, found := dictparser.DictMap[r.Stem.String]; found {
			// Check if we already have this word in our result map (for multiple usages)
			if existingWord, exists := oxWords[r.Stem.String]; exists && isLookedUpAlot(r.WordKey.String, wordKeysLookedUpAlot) {
				existingWord.AppendUsageAndBook(r.Usage.String, r.Title.String)
				existingWord.AppendWordsInUsage(r.Word.String)
			} else {
				// Create new word
				oxWords[r.Stem.String] = oxforddicthandler.CreateWord(&r, entry)
			}
		}
	}

	setProcessedWords(oxWords)

	// 3. Prepare JSON response
	viewWords := make([]ViewWord, 0, len(oxWords))
	for _, word := range oxWords {
		viewWords = append(viewWords, ViewWord{
			Stem:       word.Stem(),
			Definition: word.Definition(),
			Usage:      word.Usage(),
			// Check constant map safely
			IsBasic: isBasic(word.Stem()),
			Books:   word.Books(),
		})
	}

	sort.Slice(viewWords, func(i, j int) bool {
		return strings.ToLower(viewWords[i].Stem) < strings.ToLower(viewWords[j].Stem)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(viewWords)
}

func DeleteAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stemToDelete := strings.TrimPrefix(r.URL.Path, "/api/words/")
	if stemToDelete == "" {
		sendJSONError(w, "Word stem required", http.StatusBadRequest)
		return
	}

	processedWordsMutex.Lock()
	delete(processedWords, stemToDelete)
	processedWordsMutex.Unlock()
	w.WriteHeader(http.StatusOK)
}

func DownloadAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentWords := getProcessedWords()
	if len(currentWords) == 0 {
		sendJSONError(w, "No words to download. Please upload a file first.", http.StatusBadRequest)
		return
	}

	if err := sendtoanki.GenerateDeck(currentWords, constants.DECK_PATH); err != nil {
		log.Printf("Error generating deck: %v", err)
		sendJSONError(w, "Error generating deck", http.StatusInternalServerError)
		return
	}
	log.Println("Deck generated successfully.")

	w.Header().Set("Content-Disposition", "attachment; filename="+constants.DECK_FILENAME)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, constants.DECK_PATH)
	if err := os.Remove(constants.DECK_PATH); err != nil {
		log.Fatal(err)
	}
}

// Helpers

func readEveryRowFromDB(dbLoc string) ([]tutorial.GetRowsRow, error) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", dbLoc)
	if err != nil {
		return nil, err
	}
	defer db.Close() // Ensure DB is closed

	queries := tutorial.New(db)
	rows, err := queries.GetRows(ctx)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func readWordKeysWithMultipleUsagesFromDB(dbLoc string) ([]tutorial.GetWordKeysWithMultipleUsagesRow, error) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", dbLoc)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	queries := tutorial.New(db)
	rows, err := queries.GetWordKeysWithMultipleUsages(ctx)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func isLookedUpAlot(wordKey string, wordKeysWithMultipleUsages []tutorial.GetWordKeysWithMultipleUsagesRow) bool {
	for _, v := range wordKeysWithMultipleUsages {
		if v.WordKey.String == wordKey {
			return true
		}
	}
	return false
}

func isBasic(str string) bool {
	if val, ok := constants.WIKIPEDIA_ENG_1000_BASIC[str]; ok {
		return val
	}
	return false
}
