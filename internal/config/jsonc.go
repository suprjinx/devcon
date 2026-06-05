package config

// StripJSONC converts JSONC (the dialect VS Code uses for devcontainer.json)
// into plain JSON: it removes // line comments and /* */ block comments and
// drops trailing commas, all while leaving string contents untouched.
func StripJSONC(b []byte) []byte {
	return removeTrailingCommas(stripComments(b))
}

func stripComments(b []byte) []byte {
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
			i += 2
			for i < len(b) && b[i] != '\n' {
				i++
			}
			if i < len(b) {
				out = append(out, b[i]) // keep the newline
			}
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			i += 2
			for i+1 < len(b) && !(b[i] == '*' && b[i+1] == '/') {
				i++
			}
			i++ // skip the closing '/'
		default:
			out = append(out, c)
		}
	}
	return out
}

func removeTrailingCommas(b []byte) []byte {
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
		if c == '"' {
			inStr = true
			out = append(out, c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(b) && isJSONSpace(b[j]) {
				j++
			}
			if j < len(b) && (b[j] == '}' || b[j] == ']') {
				continue // drop the trailing comma
			}
		}
		out = append(out, c)
	}
	return out
}

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
