package kingpin

import (
	"bytes"
	"fmt"
	"go/doc"
	"io"
	"strings"

	"github.com/alecthomas/template"
)

var (
	preIndent = "  "
)

func formatTwoColumns(w io.Writer, indent, padding, width int, rows [][2]string) {
	// Find size of first column.
	s := 0
	for _, row := range rows {
		if c := len(row[0]); c > s && c < 20 {
			s = c
		}
	}

	indentStr := strings.Repeat(" ", indent)
	offsetStr := strings.Repeat(" ", s+padding)

	for _, row := range rows {
		buf := bytes.NewBuffer(nil)
		doc.ToText(buf, row[1], "", preIndent, width-s-padding-indent)
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		fmt.Fprintf(w, "%s%-*s%*s", indentStr, s, row[0], padding, "")
		if len(row[0]) >= 20 {
			fmt.Fprintf(w, "\n%s%s", indentStr, offsetStr)
		}
		fmt.Fprintf(w, "%s\n", lines[0])
		for _, line := range lines[1:] {
			fmt.Fprintf(w, "%s%s%s\n", indentStr, offsetStr, line)
		}
	}
}

// Usage writes application usage to w. It parses args to determine
// appropriate help context, such as which command to show help for.
func (a *Application) Usage(w io.Writer, args []string) {
	context, err := a.ParseContext(args)
	a.FatalIfError(w, err, "")
	a.UsageTemplate(context, w, 2, UsageTemplate)
}

func formatAppUsage(app *ApplicationModel) string {
	s := []string{app.Name}
	if len(app.Flags) > 0 {
		s = append(s, app.FlagSummary())
	}
	if len(app.Args) > 0 {
		s = append(s, app.ArgSummary())
	}
	return strings.Join(s, " ")
}

func formatCmdUsage(app *ApplicationModel, cmd *CmdModel) string {
	s := []string{app.Name, cmd.String()}
	if len(app.Flags) > 0 {
		s = append(s, app.FlagSummary())
	}
	if len(app.Args) > 0 {
		s = append(s, app.ArgSummary())
	}
	return strings.Join(s, " ")
}

func formatFlag(flag *FlagModel) string {
	flagString := ""
	if flag.Short != 0 {
		flagString += fmt.Sprintf("-%c, ", flag.Short)
	}
	flagString += fmt.Sprintf("--%s", flag.Name)
	if !flag.IsBoolFlag() {
		flagString += fmt.Sprintf("=%s", flag.FormatPlaceHolder())
	}
	return flagString
}

var UsageTemplate = `{{define "FormatCommand"}}\
{{if .FlagSummary}} {{.FlagSummary}}{{end}}\
{{range .Args}} <{{.Name}}>{{end}}\
{{end}}\

{{define "FormatCommands"}}\
Commands:
{{range .FlattenedCommands}}\
  {{.}}{{template "FormatCommand" .}}
{{.Help|Wrap 4}}
{{end}}\
{{end}}\

{{define "FormatUsage"}}\
{{template "FormatCommand" .}}{{if .Commands}} <command> [<args> ...]{{end}}
{{if .Help}}
{{.Help|Wrap 0}}\
{{end}}\

{{end}}\

{{if .Context.SelectedCommand}}\
usage: {{.App.Name}} {{.Context.SelectedCommand}}{{template "FormatUsage" .Context.SelectedCommand}}
{{else}}\
usage: {{.App.Name}}{{template "FormatUsage" .App}}
{{end}}\
{{if .Context.Flags}}\
Flags:
{{.Context.Flags|FlagsToTwoColumns|FormatTwoColumns}}
{{end}}\
{{if .Context.Args}}\
Args:
{{.Context.Args|ArgsToTwoColumns|FormatTwoColumns}}
{{end}}\
{{if .Context.SelectedCommand}}\
{{if .Context.SelectedCommand.Commands}}\
{{template "FormatCommands" .Context.SelectedCommand}}
{{end}}\
{{else if .App.Commands}}\
{{template "FormatCommands" .App}}
{{end}}\
`

type templateParseContext struct {
	SelectedCommand *CmdModel
	*FlagGroupModel
	*ArgGroupModel
}

type templateContext struct {
	App     *ApplicationModel
	Width   int
	Context *templateParseContext
}

func (a *Application) UsageTemplate(context *ParseContext, w io.Writer, indent int, tmpl string) error {
	width := guessWidth(w)
	funcs := template.FuncMap{
		"Wrap": func(indent int, s string) string {
			buf := bytes.NewBuffer(nil)
			indentText := strings.Repeat(" ", indent)
			doc.ToText(buf, s, indentText, indentText, width)
			return buf.String()
		},
		"FormatFlag": formatFlag,
		"FlagsToTwoColumns": func(f []*FlagModel) [][2]string {
			rows := [][2]string{}
			for _, flag := range f {
				if !flag.Hidden {
					rows = append(rows, [2]string{formatFlag(flag), flag.Help})
				}
			}
			return rows
		},
		"ArgsToTwoColumns": func(a []*ArgModel) [][2]string {
			rows := [][2]string{}
			for _, arg := range a {
				s := "<" + arg.Name + ">"
				if !arg.Required {
					s = "[" + s + "]"
				}
				rows = append(rows, [2]string{s, arg.Help})
			}
			return rows
		},
		"FormatTwoColumns": func(rows [][2]string) string {
			buf := bytes.NewBuffer(nil)
			formatTwoColumns(buf, indent, indent, width, rows)
			return buf.String()
		},
		"FormatAppUsage":     formatAppUsage,
		"FormatCommandUsage": formatCmdUsage,
	}
	t, err := template.New("usage").Funcs(funcs).Parse(tmpl)
	if err != nil {
		return err
	}
	ctx := templateContext{
		App:   a.Model,
		Width: width,
		Context: &templateParseContext{
			SelectedCommand: context.SelectedCommand,
			FlagGroupModel:  context.Flags(),
			ArgGroupModel:   context.Args(),
		},
	}
	return t.Execute(w, ctx)
}
