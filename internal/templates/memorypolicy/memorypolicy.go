// Package memorypolicy renders the shared memory horizon policy used by
// jazmem dream prompts and embedding hosts.
package memorypolicy

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"
)

//go:embed memorypolicy.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("memorypolicy").Parse(promptTemplate))

func RenderLongTerm() string {
	return render("long_term")
}

func RenderShortTerm() string {
	return render("short_term")
}

func render(name string) string {
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, name, nil); err != nil {
		panic(err)
	}
	return strings.TrimSpace(out.String())
}
