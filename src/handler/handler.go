package handler

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	// "os/exec" // Exec commented out as we are using native Go parser now
	"path/filepath"
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

	file, header, err := r.FormFile("vocab.db")
	if err != nil {
		sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Ensure resources dir exists
	os.MkdirAll(filepath.Join(constants.ROOT, "resources"), 0755)

	tmpFilePath := filepath.Join(constants.ROOT, "resources", header.Filename)
	tmpFile, err := os.Create(tmpFilePath)
	if err != nil {
		sendJSONError(w, "Could not save file", http.StatusInternalServerError)
		return
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, file)
	if err != nil {
		sendJSONError(w, fmt.Sprintf("Could not save file content: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// 1. Read User's DB (The "Words")
	rawRows, err := readEveryRowFromDB(tmpFilePath)
	if err != nil {
		sendJSONError(w, fmt.Sprintf("Could not read database: %s", err.Error()), http.StatusInternalServerError)
		return
	}
	log.Printf("%d rows found in user DB\n", len(rawRows))

	// 2. Parse the Dictionary (The "Definitions")
	// Note: dictparser.ParseJSON() reads "./json.json" by default as per your code.
	// Make sure this file exists in the root of your working directory.
	dictParsed, err := dictparser.ParseJSON()
	if err != nil {
		log.Println("Error parsing dictionary JSON:", err)
		sendJSONError(w, "Error parsing dictionary JSON", http.StatusInternalServerError)
		return
	}
	log.Printf("Dictionary loaded: %d entries\n", len(dictParsed))

	// 3. OPTIMIZATION: Convert Dictionary Slice to Map for O(1) Lookup
	// This is critical for 100,000+ entries.
	dictMap := make(map[string]*dictparser.WordEntry, len(dictParsed))
	for i := range dictParsed {
		// Store pointer to avoid copying large structs
		dictMap[dictParsed[i].Title] = &dictParsed[i]
	}

	// 4. Match Words (O(N) complexity now, instead of O(N*M))
	wordKeysLookedUpAlot, _ := readWordKeysWithMultipleUsagesFromDB(tmpFilePath)
	var oxWords = map[string]*oxforddicthandler.OxfordWord{}

	for _, r := range rawRows {
		// FAST LOOKUP
		if entry, found := dictMap[r.Stem.String]; found {
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

	// 5. Prepare JSON response
	viewWords := make([]ViewWord, 0, len(oxWords))
	for _, word := range oxWords {
		viewWords = append(viewWords, ViewWord{
			Stem:       word.Stem(),
			Definition: word.Definition(),
			Usage:      word.Usage(),
			// Check constant map safely
			IsBasic: func() bool {
				if val, ok := constants.WIKIPEDIA_ENG_1000_BASIC[word.Stem()]; ok {
					return val
				}
				return false
			}(),
			Books: word.Books(),
		})
	}

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

	// Ensure resources dir exists
	os.MkdirAll(filepath.Join(constants.ROOT, "resources"), 0755)
	filePath := filepath.Join(constants.ROOT, "resources/output.apkg")

	log.Println("Generating Anki deck...")
	if err := sendtoanki.GenerateDeck(currentWords, filePath); err != nil {
		log.Printf("Error generating deck: %v", err)
		sendJSONError(w, "Error generating deck", http.StatusInternalServerError)
		return
	}
	log.Println("Deck generated successfully.")

	w.Header().Set("Content-Disposition", "attachment; filename=output.apkg")
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, filePath)
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

func Unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	os.MkdirAll(dest, 0755)

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}
