package dreamprompt

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed dreamprompt.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("dreamprompt").Parse(promptTemplate))

type Page struct {
	Slug       string
	Title      string
	ModifiedAt string
	Body       string
}

type SystemData struct {
	LongTermPolicy  string
	ShortTermPolicy string
}

type UserData struct {
	Date      string
	LongTerm  string
	ShortTerm string
	Canonical []Page
	Inputs    []Page
}

func RenderSystem(data SystemData) (string, error) {
	return render("system", data)
}

func RenderUser(data UserData) (string, error) {
	return render("user", data)
}

func render(name string, data any) (string, error) {
	var out bytes.Buffer
	err := tmpl.ExecuteTemplate(&out, name, data)
	return out.String(), err
}
