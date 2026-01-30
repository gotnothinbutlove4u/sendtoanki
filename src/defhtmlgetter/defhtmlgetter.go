package defhtmlgetter

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const closingTag = "</d:entry>"

const htmlFooter = `</body>
</html>


`

var htmlEntries []*Entry

func init() {
	htmlEntries = LoadFromCacheFile("/tmp/noad.cache")
}

type Entry struct {
	Title string // word
	Body  []byte // word definition in XML
}

func renderSingleHTML(entries []*Entry, word string) string {
	str := ""
	for _, ent := range entries {
		if word == ent.Title {
			// trim "</d:entry>"
			bodyWithoutClosingTag := ent.Body[:len(ent.Body)-len(closingTag)]
			str += string(bodyWithoutClosingTag)
			str += closingTag
		}
	}
	str += htmlFooter
	return str
}

const DLMT = "\t"

func LoadFromCacheFile(path string) []*Entry {
	var r []*Entry
	contents, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	lines := bytes.Split(contents, []byte{'\n'})
	for _, line := range lines[:] {
		if len(line) == 0 {
			// Possibly end of file
			continue
		}
		ttlBytes, rawBody, found := bytes.Cut(line, []byte(DLMT))
		if !found {
			panic("failed to Cut:" + (string(line)))
		}
		title := string(ttlBytes)
		e := &Entry{
			Title: title,
			Body:  rawBody,
		}
		r = append(r, e)
	}
	return r
}

func Get(word string) (string, error) {
	dirtyHtml := renderSingleHTML(htmlEntries, word)
	return CleanAppleDictHTML(dirtyHtml)
}

// CleanAppleDictHTML processes the raw Apple Dictionary HTML to fix layout issues and remove clutter.
func CleanAppleDictHTML(rawXML string) (string, error) {
	doc, err := html.Parse(strings.NewReader(rawXML))
	if err != nil {
		return "", err
	}

	var cleanNode func(*html.Node)
	cleanNode = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// 1. Transform <d:entry> to <div>
			if n.Data == "d:entry" {
				n.Data = "div"
				n.DataAtom = atom.Div
			}

			// 2. Attribute Processing
			var cleanAttrs []html.Attribute
			for _, attr := range n.Attr {
				key := strings.ToLower(attr.Key)

				// A. Blocklist: Remove metadata
				if strings.HasPrefix(key, "d:") ||
					strings.HasPrefix(key, "xmlns") ||
					key == "id" ||
					key == "prlexid" ||
					key == "soundfile" ||
					key == "media" ||
					key == "dialect" ||
					key == "role" ||
					key == "onmouseover" ||
					key == "onmouseout" {
					continue
				}

				// B. Fix The "Gap" (Collusion vs Commend)
				if key == "class" {
					// We split the classes into a slice to check them individually
					classes := strings.Fields(attr.Val)

					hasPosg := false
					hasXdH := false

					for _, c := range classes {
						if c == "posg" {
							hasPosg = true
						}
						if c == "x_xdh" {
							hasXdH = true
						}
					}

					// TARGET: If an element has BOTH 'posg' and 'x_xdh' (like in 'collusion'),
					// it means the POS header is empty of other content (like [with object]).
					// We MUST remove 'x_xdh' to stop it from acting like a block with margins.
					if hasPosg && hasXdH {
						var newClasses []string
						for _, c := range classes {
							if c != "x_xdh" { // Skip the block class
								newClasses = append(newClasses, c)
							}
						}
						attr.Val = strings.Join(newClasses, " ")
					}
				}

				cleanAttrs = append(cleanAttrs, attr)
			}
			n.Attr = cleanAttrs
		}

		// 3. Process Children
		var next *html.Node
		for c := n.FirstChild; c != nil; c = next {
			next = c.NextSibling
			cleanNode(c)
		}

		// 4. Remove Empty Tags (Cleanup)
		if n.Type == html.ElementNode {
			if strings.HasPrefix(n.Data, "d:") && n.FirstChild == nil {
				removeNode(n)
				return
			}
			// Remove empty structural spans
			if n.Data == "span" && n.FirstChild == nil {
				for _, a := range n.Attr {
					if a.Key == "class" && (a.Val == "hsb" || a.Val == "tg_pos" || a.Val == "tg_df") {
						removeNode(n)
						return
					}
				}
			}
		}
	}

	// Extract body content
	var body *html.Node
	var finder func(*html.Node)
	finder = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			body = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			finder(c)
		}
	}
	finder(doc)

	if body != nil {
		cleanNode(body)
		var buf bytes.Buffer
		for c := body.FirstChild; c != nil; c = c.NextSibling {
			html.Render(&buf, c)
		}
		return buf.String(), nil
	}

	return "", fmt.Errorf("failed to parse HTML structure")
}

func removeNode(n *html.Node) {
	if n.Parent != nil {
		n.Parent.RemoveChild(n)
	}
}
