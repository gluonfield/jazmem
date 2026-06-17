package agenticprompt

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed agenticprompt.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("agenticprompt").Parse(promptTemplate))

type Evidence struct {
	ID      int
	Slug    string
	Title   string
	Chunk   int
	Snippet string
}

type UserData struct {
	Query    string
	Evidence []Evidence
}

func RenderSystem() (string, error) {
	return render("system", nil)
}

func RenderUser(data UserData) (string, error) {
	return render("user", data)
}

func render(name string, data any) (string, error) {
	var out bytes.Buffer
	err := tmpl.ExecuteTemplate(&out, name, data)
	return out.String(), err
}
