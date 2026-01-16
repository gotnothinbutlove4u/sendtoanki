package handler

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"tuto.sqlc.dev/app/go/constants"
	"tuto.sqlc.dev/app/go/dbreader"
	"tuto.sqlc.dev/app/go/dictparser"
	"tuto.sqlc.dev/app/go/oxforddicthandler"
	"tuto.sqlc.dev/app/go/sendtoanki"
)

// Holds the parsed HTML templates.
var templates = template.Must(template.ParseFiles("html/upload.html", "html/view.html"))

var processedWords []oxforddicthandler.OxfordWord

// NEW: Struct to pass data to the View template
type ViewData struct {
	Words       []oxforddicthandler.OxfordWord
	DownloadURL string
}

// UploadHandler serves the upload page on GET and processes the file on POST.
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		err := templates.ExecuteTemplate(w, "upload.html", nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error rendering template: %v", err), http.StatusInternalServerError)
		}
		return
	}

	if r.Method == "POST" {
		file, header, err := r.FormFile("vocab.db")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Create a temporary file to store the upload.
		tmpFilePath := filepath.Join(constants.ROOT, "resources", header.Filename)
		tmpFile, err := os.Create(tmpFilePath)
		if err != nil {
			http.Error(w, "Could not save file", http.StatusInternalServerError)
			return
		}
		defer tmpFile.Close()

		_, err = io.Copy(tmpFile, file)
		if err != nil {
			http.Error(w, "Could not save file content", http.StatusInternalServerError)
			return
		}

		dbName := header.Filename
		// 1. Write stems.txt
		if err := dbreader.WriteStems(filepath.Join(constants.ROOT, "resources", dbName), filepath.Join(constants.ROOT, "resources/stems.txt")); err != nil {
			log.Fatal(err)
		}

		// 2. create a raw JSON containing definitions of stems
		cmd := exec.Command(filepath.Join(constants.ROOT, "venv/bin/python3"), "./python/extract.py", "./resources/stems.txt", "./resources/"+constants.ZIP_FILENAME)
		cmd.Dir = constants.ROOT
		out, err := cmd.Output()
		if err != nil {
			log.Println("Python script error")
			log.Fatal(err)
		}
		println(string(out))

		// 3. unzip and read the raw JSON
		jsonDirLoc := strings.TrimSuffix(filepath.Join(constants.ROOT, "resources", constants.ZIP_FILENAME), filepath.Ext(constants.ZIP_FILENAME))
		if err := Unzip(filepath.Join(constants.ROOT, "resources", constants.ZIP_FILENAME), jsonDirLoc); err != nil {
			log.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(jsonDirLoc, constants.JSON_FILENAME))
		if err != nil {
			panic(err)
		}
		rawJSON := string(b)

		dictParsed, err := dictparser.ParseDictJson(rawJSON)
		if err != nil {
			log.Panic(err)
		}

		// 4. parse the raw JSON into golang structures
		wordsDB, err := dbreader.ReadDB(filepath.Join(constants.ROOT, "resources", dbName))
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%d words found\n", len(wordsDB))

		oxWords := []oxforddicthandler.OxfordWord{}
		for _, v := range wordsDB {
			found := false
			for _, p := range dictParsed {
				if v.Stem.String == p.Word {
					oxWords = append(oxWords, oxforddicthandler.CreateWord(v, p))
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("\"%s\" not found in a dictionary\n", v.Stem.String)
			}
		}
		processedWords = oxWords

		log.Println("All's well!")
		http.Redirect(w, r, "/view", http.StatusSeeOther)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ViewHandler generates the deck and displays the list of words.
func ViewHandler(w http.ResponseWriter, r *http.Request) {
	if len(processedWords) == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// 1. Generate the deck FIRST. If this fails, we shouldn't show the success page.
	filePath := filepath.Join(constants.ROOT, "resources/output.apkg")
	err := sendtoanki.GenerateDeck(processedWords, filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating deck: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. Prepare the data for the template
	data := ViewData{
		Words:       processedWords,
		DownloadURL: "/download", // This matches the route in main.go
	}

	// 3. Render the template
	err = templates.ExecuteTemplate(w, "view.html", data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error rendering template: %v", err), http.StatusInternalServerError)
	}
}

// NEW: DownloadHandler specifically serves the generated .apkg file
func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	filePath := filepath.Join(constants.ROOT, "resources/output.apkg")

	// Check if file exists before serving
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "File not found. Please upload a database first.", http.StatusNotFound)
		return
	}

	// Force the browser to download the file instead of trying to open it
	w.Header().Set("Content-Disposition", "attachment; filename=output.apkg")
	w.Header().Set("Content-Type", "application/octet-stream")

	http.ServeFile(w, r, filePath)
}

// Unzip helper
func Unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	os.MkdirAll(dest, 0755)

	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		if err := extractAndWriteFile(f); err != nil {
			return err
		}
	}
	return nil
}
