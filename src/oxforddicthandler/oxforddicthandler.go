package oxforddicthandler

import (
	"fmt"
	"strings"

	"github.com/dawit909/sendtoanki/src/defhtmlgetter"
	"github.com/dawit909/sendtoanki/src/dictparser"
	"github.com/dawit909/sendtoanki/src/tutorial"
)

// --- STRUCTS ---

type SubDefinition struct {
	Text string `json:"text"`
}

type Definition struct {
	EntryID      int             `json:"entry_id"`
	PartOfSpeech string          `json:"part_of_speech"`
	Meaning      string          `json:"meaning"`
	Subs         []SubDefinition `json:"sub_definitions"`
}

type OxfordWord struct {
	entry        dictparser.WordEntry
	parsedDefs   []Definition
	wordsInUsage []string
	usage        []string
	book         []string
}

// --- ACCESSORS ---

func (w OxfordWord) Stem() string {
	return w.entry.Title
}

func (w OxfordWord) Books() []string {
	return w.book
}

func (w OxfordWord) Book() string {
	if len(w.book) > 0 {
		return w.book[0]
	}
	return ""
}

// Definition generates the HTML using the robust parser
func (w OxfordWord) Definition() string {
	html, err := defhtmlgetter.Get(w.Stem())
	if err != nil {
		panic(err)
	}
	return html
	// // Parse on the fly
	// w.parsedDefs = ParseHierarchicalEntry(w.entry.Meaning).Definitions
	// return generateHTML(w.entry, w.parsedDefs)
}

func (w OxfordWord) Usage() string {
	var sb strings.Builder
	for i := range w.usage {
		split := strings.Split(w.usage[i], " ")
		sb.WriteString(`<div class="w-usage">`)
		for _, v := range split {
			contained := false
			vLower := strings.ToLower(v)
			for _, target := range w.wordsInUsage {
				if strings.Contains(vLower, strings.ToLower(target)) {
					contained = true
					break
				}
			}
			if contained {
				sb.WriteString("<b>" + v + "</b> ")
			} else {
				sb.WriteString(v + " ")
			}
		}
		bookTitle := ""
		if i < len(w.book) {
			bookTitle = w.book[i]
		}
		sb.WriteString(fmt.Sprintf("<small>%s</small></div>", bookTitle))
	}
	return sb.String()
}

// --- MODIFIERS ---

func (w *OxfordWord) AppendUsageAndBook(usage, book string) {
	w.usage = append(w.usage, usage)
	w.book = append(w.book, book)
}

func (w *OxfordWord) AppendWordsInUsage(str string) {
	w.wordsInUsage = append(w.wordsInUsage, str)
}

// --- CONSTRUCTOR ---

func CreateWord(fromDB *tutorial.GetRowsRow, fromJSON *dictparser.WordEntry) *OxfordWord {
	w := OxfordWord{entry: *fromJSON}
	w.AppendUsageAndBook(fromDB.Usage.String, fromDB.Title.String)
	w.AppendWordsInUsage(fromDB.Word.String)
	return &w
}
