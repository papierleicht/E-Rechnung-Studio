package fints

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type segmentHeader struct {
	Type      string
	Number    int
	Version   int
	Reference int
}

type field struct {
	raw        []byte
	binary     bool
	components []string
}

type segment struct {
	Header segmentHeader
	Fields []field
}

func parseSegments(data []byte) ([]segment, error) {
	return parseSegmentsAtDepth(data, 0)
}

func parseSegmentsAtDepth(data []byte, depth int) ([]segment, error) {
	if depth > 4 {
		return nil, fmt.Errorf("FinTS payload nesting exceeds limit")
	}
	rawSegments, err := splitWire(data, '\'')
	if err != nil {
		return nil, err
	}

	var result []segment
	for _, raw := range rawSegments {
		if len(raw) == 0 {
			continue
		}
		parts, err := splitWire(raw, '+')
		if err != nil {
			return nil, err
		}
		if len(parts) == 0 {
			continue
		}
		headerParts, err := splitWire(parts[0], ':')
		if err != nil || len(headerParts) < 3 {
			return nil, fmt.Errorf("invalid FinTS segment header")
		}
		number, err := strconv.Atoi(string(headerParts[1]))
		if err != nil || number <= 0 {
			return nil, fmt.Errorf("invalid FinTS segment number")
		}
		version, err := strconv.Atoi(string(headerParts[2]))
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid FinTS segment version")
		}
		header := segmentHeader{Type: string(headerParts[0]), Number: number, Version: version}
		if len(headerParts) > 3 && len(headerParts[3]) > 0 {
			header.Reference, _ = strconv.Atoi(string(headerParts[3]))
		}

		s := segment{Header: header}
		for _, part := range parts[1:] {
			f := field{raw: part}
			if content, ok := binaryContent(part); ok {
				f.raw = content
				f.binary = true
			} else {
				componentParts, splitErr := splitWire(part, ':')
				if splitErr != nil {
					return nil, splitErr
				}
				for _, component := range componentParts {
					f.components = append(f.components, decodeLatin1(unescape(component)))
				}
			}
			s.Fields = append(s.Fields, f)
		}
		result = append(result, s)

		if s.Header.Type == "HNVSD" && len(s.Fields) > 0 && s.Fields[0].binary {
			nested, nestedErr := parseSegmentsAtDepth(s.Fields[0].raw, depth+1)
			if nestedErr != nil {
				return nil, fmt.Errorf("parse encrypted payload: %w", nestedErr)
			}
			result = append(result, nested...)
		}
	}
	return result, nil
}

func splitWire(data []byte, separator byte) ([][]byte, error) {
	var result [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '?':
			if i+1 >= len(data) {
				return nil, fmt.Errorf("unterminated FinTS escape")
			}
			i++
		case '@':
			end := bytes.IndexByte(data[i+1:], '@')
			if end < 0 {
				return nil, fmt.Errorf("unterminated FinTS binary header")
			}
			end += i + 1
			size, err := strconv.Atoi(string(data[i+1 : end]))
			if err != nil || size < 0 || strings.HasPrefix(string(data[i+1:end]), "+") {
				return nil, fmt.Errorf("invalid FinTS binary length")
			}
			if size > len(data)-(end+1) {
				return nil, fmt.Errorf("binary field declares %d bytes, only %d remain", size, len(data)-(end+1))
			}
			i = end + size
		default:
			if data[i] == separator {
				result = append(result, data[start:i])
				start = i + 1
			}
		}
	}
	if start < len(data) {
		result = append(result, data[start:])
	} else if start == len(data) && (len(data) == 0 || data[len(data)-1] == separator) {
		result = append(result, nil)
	}
	return result, nil
}

func binaryContent(raw []byte) ([]byte, bool) {
	if len(raw) < 3 || raw[0] != '@' {
		return nil, false
	}
	end := bytes.IndexByte(raw[1:], '@')
	if end < 0 {
		return nil, false
	}
	end++
	size, err := strconv.Atoi(string(raw[1:end]))
	if err != nil || size < 0 || size != len(raw)-(end+1) {
		return nil, false
	}
	return raw[end+1:], true
}

func unescape(raw []byte) []byte {
	result := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '?' && i+1 < len(raw) {
			i++
		}
		result = append(result, raw[i])
	}
	return result
}

func escape(value string) string {
	replacer := strings.NewReplacer(
		"?", "??",
		"+", "?+",
		":", "?:",
		"'", "?'",
		"@", "?@",
	)
	return replacer.Replace(value)
}

func binary(value []byte) string {
	return "@" + strconv.Itoa(len(value)) + "@" + string(value)
}

func findSegments(segments []segment, kind string) []segment {
	var matches []segment
	for _, s := range segments {
		if s.Header.Type == kind {
			matches = append(matches, s)
		}
	}
	return matches
}

func component(s segment, fieldIndex, componentIndex int) string {
	if fieldIndex < 0 || fieldIndex >= len(s.Fields) {
		return ""
	}
	f := s.Fields[fieldIndex]
	if componentIndex < 0 || componentIndex >= len(f.components) {
		return ""
	}
	return f.components[componentIndex]
}

// decodeLatin1 converts FinTS textual fields to Go's UTF-8 string convention.
// Binary fields must bypass this conversion (XML may itself be UTF-8).
func decodeLatin1(raw []byte) string {
	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	return string(runes)
}

// encodeWireText converts a constructed segment sequence from UTF-8 text to
// ISO-8859-1 while retaining the exact bytes/lengths of opaque binary fields.
// It runs before the inner segment sequence is wrapped in HNVSD.
func encodeWireText(data []byte) ([]byte, error) {
	result := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		if data[i] == '@' {
			end := bytes.IndexByte(data[i+1:], '@')
			if end < 0 {
				return nil, fmt.Errorf("invalid outgoing binary marker")
			}
			end += i + 1
			size, err := strconv.Atoi(string(data[i+1 : end]))
			if err != nil || size < 0 || size > len(data)-end-1 {
				return nil, fmt.Errorf("invalid outgoing binary length")
			}
			next := end + 1 + size
			result = append(result, data[i:next]...)
			i = next
			continue
		}
		if data[i] == '?' {
			result = append(result, '?')
			i++
			if i == len(data) {
				return nil, fmt.Errorf("invalid outgoing escape")
			}
		}
		r, size := utf8.DecodeRune(data[i:])
		if r > 255 || (r == utf8.RuneError && size == 1) {
			return nil, fmt.Errorf("FinTS text contains characters outside ISO-8859-1")
		}
		result = append(result, byte(r))
		i += size
	}
	return result, nil
}
