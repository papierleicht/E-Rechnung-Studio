package einvoice

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// xmlNode is the small XML tree used by the invoice field mappings. Child paths
// contain only the fixed, namespace-qualified element names used below: they
// are not XPath expressions and cannot execute predicates or external lookups.
type xmlNode struct {
	name     xml.Name
	attrs    []xml.Attr
	children []*xmlNode
	content  []xmlContent
}

type xmlContent struct {
	text string
	node *xmlNode
}

const (
	nsCIIRAM = "urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100"
	nsCIIUDT = "urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100"
	nsCIIQDT = "urn:un:unece:uncefact:data:standard:QualifiedDataType:100"
)

// readInvoiceXML uses encoding/xml's strict parser. DTDs are not part of the
// supported invoice formats; rejecting directives also prevents entity loading.
// The decoder never accesses the filesystem or network to resolve XML content.
func readInvoiceXML(r io.Reader) (*xmlNode, error) {
	reader := bufio.NewReader(r)
	if prefix, _ := reader.Peek(3); string(prefix) == "\xef\xbb\xbf" {
		_, _ = reader.Discard(3)
	}
	decoder := xml.NewDecoder(reader)
	var root *xmlNode
	var stack []xmlFrame
	sawToken := false
	for {
		// RawToken preserves prefixes so undeclared prefixes can be rejected.
		// Namespace expansion and balanced element checks are performed below.
		token, err := decoder.RawToken()
		if err == io.EOF {
			if root == nil {
				return nil, fmt.Errorf("missing XML root element")
			}
			if len(stack) != 0 {
				return nil, fmt.Errorf("unclosed XML element %s", stack[len(stack)-1].rawName.Local)
			}
			return root, nil
		}
		if err != nil {
			return nil, err
		}
		firstToken := !sawToken
		sawToken = true
		switch token := token.(type) {
		case xml.StartElement:
			if len(stack) >= 256 {
				return nil, fmt.Errorf("XML nesting exceeds 256 elements")
			}
			namespaces := map[string]string{"xml": "http://www.w3.org/XML/1998/namespace"}
			if len(stack) > 0 {
				for prefix, uri := range stack[len(stack)-1].namespaces {
					namespaces[prefix] = uri
				}
			}
			if err := declareXMLNamespaces(namespaces, token.Attr); err != nil {
				return nil, err
			}
			name, err := resolveXMLName(token.Name, namespaces, false)
			if err != nil {
				return nil, err
			}
			node := &xmlNode{name: name}
			seen := make(map[xml.Name]bool, len(token.Attr))
			for _, attr := range token.Attr {
				if attr.Name.Space == "xmlns" || attr.Name == (xml.Name{Local: "xmlns"}) {
					continue
				}
				attr.Name, err = resolveXMLName(attr.Name, namespaces, true)
				if err != nil {
					return nil, err
				}
				if seen[attr.Name] {
					return nil, fmt.Errorf("duplicate XML attribute %s", attr.Name.Local)
				}
				seen[attr.Name] = true
				node.attrs = append(node.attrs, attr)
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("multiple XML root elements")
				}
				root = node
			} else {
				parent := stack[len(stack)-1].node
				parent.children = append(parent.children, node)
				parent.content = append(parent.content, xmlContent{node: node})
			}
			stack = append(stack, xmlFrame{node: node, rawName: token.Name, namespaces: namespaces})
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].rawName != token.Name {
				return nil, fmt.Errorf("unmatched XML closing element %s", token.Name.Local)
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.Trim(string(token), " \t\r\n") != "" {
					return nil, fmt.Errorf("text outside XML root element")
				}
				continue
			}
			parent := stack[len(stack)-1].node
			parent.content = append(parent.content, xmlContent{text: string(token)})
		case xml.Directive:
			return nil, fmt.Errorf("XML directives and DTDs are not supported")
		case xml.ProcInst:
			if strings.EqualFold(token.Target, "xml") && (!firstToken || token.Target != "xml") {
				return nil, fmt.Errorf("XML declaration must occur once at the start of the document")
			}
		}
	}
}

type xmlFrame struct {
	node       *xmlNode
	rawName    xml.Name
	namespaces map[string]string
}

func declareXMLNamespaces(namespaces map[string]string, attrs []xml.Attr) error {
	seen := make(map[string]bool)
	for _, attr := range attrs {
		var prefix string
		switch {
		case attr.Name.Space == "xmlns":
			prefix = attr.Name.Local
			if prefix == "xmlns" || !validXMLNCName(prefix) || attr.Value == "" {
				return fmt.Errorf("invalid XML namespace declaration %s", prefix)
			}
		case attr.Name == (xml.Name{Local: "xmlns"}):
		default:
			continue
		}
		if seen[prefix] {
			return fmt.Errorf("duplicate XML namespace declaration %s", prefix)
		}
		seen[prefix] = true
		const xmlURI = "http://www.w3.org/XML/1998/namespace"
		if attr.Value == "http://www.w3.org/2000/xmlns/" || (prefix == "xml") != (attr.Value == xmlURI) {
			return fmt.Errorf("invalid reserved XML namespace binding %s", prefix)
		}
		namespaces[prefix] = attr.Value
	}
	return nil
}

func resolveXMLName(raw xml.Name, namespaces map[string]string, attribute bool) (xml.Name, error) {
	if !validXMLNCName(raw.Local) || (raw.Space != "" && !validXMLNCName(raw.Space)) || raw.Space == "xmlns" {
		return xml.Name{}, fmt.Errorf("invalid qualified XML name %s:%s", raw.Space, raw.Local)
	}
	if raw.Space == "" && attribute {
		return raw, nil
	}
	namespace, declared := namespaces[raw.Space]
	if raw.Space != "" && !declared {
		return xml.Name{}, fmt.Errorf("undeclared XML namespace prefix %s", raw.Space)
	}
	return xml.Name{Space: namespace, Local: raw.Local}, nil
}

// XML 1.0 (fifth edition) NCNames use the Name productions without a colon.
// encoding/xml accepts some colon-containing Names that are not legal QNames.
func validXMLNCName(name string) bool {
	if name == "" {
		return false
	}
	for i, ch := range name {
		if xmlNameStart(ch) {
			continue
		}
		if i == 0 || !(ch == '-' || ch == '.' || ch >= '0' && ch <= '9' || ch == 0xB7 || ch >= 0x300 && ch <= 0x36F || ch >= 0x203F && ch <= 0x2040) {
			return false
		}
	}
	return true
}

func xmlNameStart(ch rune) bool {
	return ch == '_' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' ||
		ch >= 0xC0 && ch <= 0xD6 || ch >= 0xD8 && ch <= 0xF6 ||
		ch >= 0xF8 && ch <= 0x2FF || ch >= 0x370 && ch <= 0x37D ||
		ch >= 0x37F && ch <= 0x1FFF || ch >= 0x200C && ch <= 0x200D ||
		ch >= 0x2070 && ch <= 0x218F || ch >= 0x2C00 && ch <= 0x2FEF ||
		ch >= 0x3001 && ch <= 0xD7FF || ch >= 0xF900 && ch <= 0xFDCF ||
		ch >= 0xFDF0 && ch <= 0xFFFD || ch >= 0x10000 && ch <= 0xEFFFF
}

// invoiceElementName resolves mapping prefixes, independently of whichever
// prefixes (or default namespace) the input document chooses.
func invoiceElementName(step string) (xml.Name, bool) {
	prefix, local, ok := strings.Cut(step, ":")
	if !ok || local == "" {
		return xml.Name{}, false
	}
	var namespace string
	switch prefix {
	case "rsm":
		namespace = nsCIIRootInvoice
	case "ram":
		namespace = nsCIIRAM
	case "udt":
		namespace = nsCIIUDT
	case "qdt":
		namespace = nsCIIQDT
	case "cac":
		namespace = nsUBLCAC
	case "cbc":
		namespace = nsUBLCBC
	default:
		return xml.Name{}, false
	}
	return xml.Name{Space: namespace, Local: local}, true
}

func (n *xmlNode) nodes(path string) []*xmlNode {
	if n == nil {
		return nil
	}
	current := []*xmlNode{n}
	if path == "" || path == "." {
		return current
	}
	for step := range strings.SplitSeq(path, "/") {
		name, ok := invoiceElementName(step)
		if !ok {
			return nil
		}
		var next []*xmlNode
		for _, parent := range current {
			for _, child := range parent.children {
				if child.name == name {
					next = append(next, child)
				}
			}
		}
		current = next
		if len(current) == 0 {
			break
		}
	}
	return current
}

func (n *xmlNode) node(path string) *xmlNode {
	if nodes := n.nodes(path); len(nodes) > 0 {
		return nodes[0]
	}
	return nil
}

func (n *xmlNode) count(path string) int   { return len(n.nodes(path)) }
func (n *xmlNode) exists(path string) bool { return n.node(path) != nil }

func (n *xmlNode) value(path string) string {
	n = n.node(path)
	if n == nil {
		return ""
	}
	var text strings.Builder
	n.appendText(&text)
	return text.String()
}

func (n *xmlNode) appendText(text *strings.Builder) {
	for _, item := range n.content {
		if item.node != nil {
			item.node.appendText(text)
		} else {
			text.WriteString(item.text)
		}
	}
}

// Invoice attributes such as currencyID and schemeID are unqualified. A
// foreign, namespaced attribute with the same local name must not match them.
func (n *xmlNode) attribute(name string) string {
	if n != nil {
		for _, attr := range n.attrs {
			if attr.Name.Space == "" && attr.Name.Local == name {
				return attr.Value
			}
		}
	}
	return ""
}

func (n *xmlNode) integer(path string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(n.value(path)))
	return value
}

func (n *xmlNode) taxRegistration(scheme string) string {
	for _, id := range n.nodes("ram:SpecifiedTaxRegistration/ram:ID") {
		if id.attribute("schemeID") == scheme {
			return id.value("")
		}
	}
	return ""
}
