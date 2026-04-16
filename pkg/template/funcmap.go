package template

import (
	"strings"
	gt "text/template"

	"sigs.k8s.io/yaml"
)

// Indent adds the specified number of spaces to each non-empty line of text.
// Negative values of spaces are treated as zero.
func Indent(spaces int, text string) string {
	if text == "" {
		return text
	}

	if spaces < 0 {
		spaces = 0
	}

	prefix := strings.Repeat(" ", spaces)

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}

	return strings.Join(lines, "\n")
}

// TextTemplateFuncMap returns a map of custom template functions for
// text/template. Includes indent, nindent, and toYaml helpers commonly used
// in Kubernetes manifest templates.
func TextTemplateFuncMap() gt.FuncMap {
	return gt.FuncMap{
		"indent": Indent,
		"nindent": func(spaces int, s string) string {
			if s == "" {
				return ""
			}

			return "\n" + Indent(spaces, s)
		},
		"toYaml": func(v any) (string, error) {
			b, err := yaml.Marshal(v)
			return string(b), err
		},
	}
}
