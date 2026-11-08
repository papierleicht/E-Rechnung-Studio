package pdfinvoice

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type pagePlan struct {
	Pages   int
	Anchors map[string]int
}

// Every atomic table row and standalone block has a stable PDF destination.
// Existing paragraph IDs are preserved; bare cells get a margin-free paragraph.
// Both renderers consume this same instrumented template.
func flowDocument(source string, plan *pagePlan) (string, error) {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	var body *html.Node
	var findBody func(*html.Node)
	findBody = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			body = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findBody(c)
		}
	}
	findBody(doc)
	if body == nil {
		return "", fmt.Errorf("Rechnungsvorlage enthält keinen Inhalt")
	}
	var atoms []*html.Node
	for block := body.FirstChild; block != nil; block = block.NextSibling {
		if block.Type != html.ElementNode || block.Data == "footer" {
			continue
		}
		if block.Data != "table" {
			atoms = append(atoms, block)
			continue
		}
		for section := block.FirstChild; section != nil; section = section.NextSibling {
			if section.Type != html.ElementNode || section.Data != "tbody" {
				continue
			}
			for row := section.FirstChild; row != nil; row = row.NextSibling {
				if row.Type == html.ElementNode && row.Data == "tr" {
					atoms = append(atoms, row)
				}
			}
		}
	}
	for i, atom := range atoms {
		id := fmt.Sprintf("flow-%d", i)
		var marker *html.Node
		var findParagraph func(*html.Node)
		findParagraph = func(n *html.Node) {
			if marker != nil {
				return
			}
			if n.Type == html.ElementNode && (n.Data == "p" || n.Data == "h1" || n.Data == "h2") {
				marker = n
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				findParagraph(c)
			}
		}
		findParagraph(atom)
		if marker == nil {
			var cell *html.Node
			for c := atom.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") && hasFlowText(c) {
					cell = c
					break
				}
			}
			if cell == nil {
				return "", fmt.Errorf("Abschnitt %s enthält keinen Textblock", id)
			}
			marker = &html.Node{Type: html.ElementNode, Data: "p", Attr: []html.Attribute{{Key: "style", Val: "margin:0"}}}
			for cell.FirstChild != nil {
				child := cell.FirstChild
				cell.RemoveChild(child)
				marker.AppendChild(child)
			}
			cell.AppendChild(marker)
		}
		existing := false
		for _, attr := range marker.Attr {
			if attr.Key == "id" {
				id = attr.Val
				existing = true
				break
			}
		}
		if !existing {
			marker.Attr = append(marker.Attr, html.Attribute{Key: "id", Val: id})
		}
		atom.Attr = append(atom.Attr, html.Attribute{Key: "data-flow-id", Val: id})
		if plan != nil {
			page := plan.Anchors[id]
			if page < 1 || page > plan.Pages {
				return "", fmt.Errorf("Seitenzuordnung für %s fehlt", id)
			}
			atom.Attr = append(atom.Attr, html.Attribute{Key: "data-flow-page", Val: strconv.Itoa(page)})
		}
	}

	if plan != nil {
		body.Attr = append(body.Attr, html.Attribute{Key: "data-page-count", Val: strconv.Itoa(plan.Pages)})
	}
	var out bytes.Buffer
	if err = html.Render(&out, doc); err != nil {
		return "", err
	}
	return out.String(), nil
}

func hasFlowText(n *html.Node) bool {
	if n.Type == html.TextNode && strings.TrimSpace(n.Data) != "" {
		return true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if hasFlowText(c) {
			return true
		}
	}
	return false
}
