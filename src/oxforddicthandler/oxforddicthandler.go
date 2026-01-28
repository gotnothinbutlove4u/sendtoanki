package oxforddicthandler

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dawit909/sendtoanki/src/dictparser"
	"github.com/dawit909/sendtoanki/src/tutorial"
)

type OxfordWord struct {
	entry        dictparser.WordEntry
	wordsInUsage []string
	usage        []string
	book         []string
}

// Accessors

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

func (w OxfordWord) Definition() string {
	return generateHTML(w.entry)
}

func (w OxfordWord) Usage() string {
	var sb strings.Builder
	for i := range w.usage {
		split := strings.Split(w.usage[i], " ")
		sb.WriteString("<blockquote>")
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
		sb.WriteString(fmt.Sprintf("<small>%s</small></blockquote>", bookTitle))
	}
	return sb.String()
}

func (w *OxfordWord) AppendUsageAndBook(usage, book string) {
	w.usage = append(w.usage, usage)
	w.book = append(w.book, book)
}

func (w *OxfordWord) AppendWordsInUsage(str string) {
	w.wordsInUsage = append(w.wordsInUsage, str)
}

func CreateWord(fromDB *tutorial.GetRowsRow, fromJSON *dictparser.WordEntry) *OxfordWord {
	w := OxfordWord{entry: *fromJSON}
	w.AppendUsageAndBook(fromDB.Usage.String, fromDB.Title.String)
	w.AppendWordsInUsage(fromDB.Word.String)
	return &w
}

// --- HTML GENERATOR ---

func generateHTML(e dictparser.WordEntry) string {
	var sb strings.Builder
	sb.WriteString("<div class=\"entry-container\">\n")

	// 1. Header & IPA
	sb.WriteString("    <div class=\"word-header\">")
	sb.WriteString(e.Title)
	if e.IPA != "" {
		sb.WriteString(" <span class=\"ipa\" style=\"color:#777; font-weight:normal; font-size:0.8em;\">/")
		sb.WriteString(e.IPA)
		sb.WriteString("/</span>")
	}
	sb.WriteString("</div>\n")

	// 2. Syllables
	if e.Syllable != "" {
		sb.WriteString("    <div class=\"syllable\" style=\"margin-bottom:10px; color:#555;\">")
		sb.WriteString(e.Syllable)
		sb.WriteString("</div>\n")
	}

	// 3. Meaning
	if e.Meaning != "" {
		sb.WriteString("    <div class=\"def-group\">\n")
		sb.WriteString("        <div class=\"def-content\" style=\"line-height:1.6; font-size:0.95em;\">\n")
		sb.WriteString(formatMeaning(e.Meaning))
		sb.WriteString("\n        </div>\n")
		sb.WriteString("    </div>\n")
	}

	// 4. Derivatives (NEW)
	if e.Derivatives != "" {
		sb.WriteString("    <div class=\"derivatives\" style=\"margin-top:10px; padding:8px; background:#f9f9f9; border-left:3px solid #ccc; font-size:0.9em;\">")
		// Clean up the derivative text which often looks like "DERIVATIVES fuzzily | fuzziness ..."
		cleanDeriv := strings.Replace(e.Derivatives, "DERIVATIVES", "<strong>Derivatives:</strong> ", 1)
		cleanDeriv = strings.ReplaceAll(cleanDeriv, "|", "•")
		sb.WriteString(cleanDeriv)
		sb.WriteString("</div>\n")
	}

	// 5. Etymology
	if len(e.Etymology) > 0 {
		sb.WriteString("    <div class=\"etymology\" style=\"margin-top:15px; padding-top:10px; border-top:1px solid #eee; font-size:0.9em; color:#666;\"><strong>Origin:</strong> ")
		for i, part := range e.Etymology {
			sb.WriteString(part)
			if i < len(e.Etymology)-1 {
				sb.WriteString(" ")
			}
		}
		sb.WriteString("</div>\n")
	}

	sb.WriteString("</div>")
	return sb.String()
}

// formatMeaning cleans up the messy raw string from the JSON
func formatMeaning(raw string) string {
	text := strings.TrimSpace(raw)

	// --- 1. Remove Garbage Artifacts ---

	// Remove leading number clumps like "1 2 3" or "1 2" at the very start
	// Match start -> digits/spaces -> Capture the first letter ($1)
	leadingNums := regexp.MustCompile(`^[\d\s]+([a-zA-Z])`)
	// Replace the match with just the captured letter ($1), effectively dropping the numbers
	text = leadingNums.ReplaceAllString(text, "$1")

	// Remove grammar blocks like "( fuzzier | fuzziest | )"
	// Regex: ( word | word | )
	grammarBlock := regexp.MustCompile(`\(\s*\w+\s*\|\s*\w+\s*\|\s*\)`)
	text = grammarBlock.ReplaceAllString(text, "")

	// Remove stray IPA-like characters often found at the end of these strings
	// This is a bit aggressive but necessary for the "fuzzy" example
	// Regex: Match common IPA chars at the end of the string
	ipaTrash := regexp.MustCompile(`[\sˈəɛɪʊɔæɑː]+$`)
	text = ipaTrash.ReplaceAllString(text, "")

	// --- 2. Format Structure ---

	// Highlight Parts of Speech (POS)
	posPatterns := []string{
		"noun", "verb", "adjective", "adverb",
		"preposition", "pronoun", "exclamation",
		"conjunction", "interjection", "prefix", "suffix",
		"abbreviation",
	}

	for _, pos := range posPatterns {
		re := regexp.MustCompile(fmt.Sprintf(`(?i)\b%s\b`, pos))
		if re.MatchString(text) {
			replacement := fmt.Sprintf(`<div class="pos-header" style="color:#2980b9; font-weight:700; font-style:italic; margin-top:12px; margin-bottom:4px; text-transform:lowercase; border-bottom:1px solid #eee;">%s</div>`, pos)
			text = re.ReplaceAllString(text, replacement)
		}
	}

	// Handle specific numbered definitions embedded in text (e.g. " 1 ", " 2 ")
	numRe := regexp.MustCompile(`\s([1-9])\s`)
	text = numRe.ReplaceAllString(text, `<br><b style="color:#444; margin-right:5px;">$1.</b> `)

	// Handle Examples (indicated by colon :)
	// Replaces " : " with a newline and indentation
	text = strings.ReplaceAll(text, " : ", `<br><span style="display:block; margin-left:15px; color:#555; font-style:italic;">&rdsh; `)

	// Handle Bullets '•'
	text = strings.ReplaceAll(text, "•", `<br><span style="color:#2980b9; margin-right:5px;">•</span>`)

	// Handle Pipes '|' often used to separate multiple examples
	text = strings.ReplaceAll(text, " | ", `<br><span style="display:block; margin-left:15px; color:#555; font-style:italic;">&rdsh; `)

	// Cleanup formatting artifacts
	text = strings.TrimPrefix(text, "<br>")
	text = strings.ReplaceAll(text, " .", ".")
	text = strings.ReplaceAll(text, " ,", ",")

	return text
}
