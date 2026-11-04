package adapters

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
)

// stripJSONC turns JSON-with-comments (VS Code, Zed, Copilot configs) into
// plain JSON: drops // and /* */ comments and trailing commas outside strings.
func stripJSONC(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inStr, esc := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inStr {
			out = append(out, c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				i++
			}
			out = append(out, '\n')
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			for i += 2; i+1 < len(b) && !(b[i] == '*' && b[i+1] == '/'); i++ {
			}
			i++
		case c == ',':
			j := i + 1
			for j < len(b) && (b[j] == ' ' || b[j] == '\t' || b[j] == '\n' || b[j] == '\r') {
				j++
			}
			if j < len(b) && (b[j] == '}' || b[j] == ']') {
				continue // trailing comma
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

// jsonKeys returns the sorted keys of the object found at path inside a
// JSON/JSONC file. Missing, empty or unparsable files yield nothing.
// Only keys are read: values (env vars, API keys) never leave this function.
func jsonKeys(file string, path []string) []string {
	b, err := os.ReadFile(file)
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	raw := json.RawMessage(stripJSONC(b))
	for _, k := range path {
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			return nil
		}
		raw = obj[k]
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
