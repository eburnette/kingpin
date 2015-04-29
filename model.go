package kingpin

import (
	"fmt"
	"strconv"
	"strings"
)

type FlagGroupModel struct {
	Flags []*FlagModel
}

func (f *FlagGroupModel) FlagSummary() string {
	out := []string{}
	count := 0
	for _, flag := range f.Flags {
		if flag.Name != "help" {
			count++
		}
		if flag.Required {
			if flag.IsBoolFlag() {
				out = append(out, fmt.Sprintf("--[no-]%s", flag.Name))
			} else {
				out = append(out, fmt.Sprintf("--%s=%s", flag.Name, flag.FormatPlaceHolder()))
			}
		}
	}
	if count != len(out) {
		out = append(out, "[<flags>]")
	}
	return strings.Join(out, " ")
}

type FlagModel struct {
	Name        string
	Help        string
	Short       rune
	Default     string
	Envar       string
	PlaceHolder string
	Required    bool
	Hidden      bool
	flag        *FlagClause
}

func (f *FlagModel) String() string {
	return f.flag.value.String()
}

func (f *FlagModel) IsBoolFlag() bool {
	if fl, ok := f.flag.value.(boolFlag); ok {
		return fl.IsBoolFlag()
	}
	return false
}

func (f *FlagModel) FormatPlaceHolder() string {
	if f.PlaceHolder != "" {
		return f.PlaceHolder
	}
	if f.Default != "" {
		if _, ok := f.flag.value.(*stringValue); ok {
			return strconv.Quote(f.Default)
		}
		return f.Default
	}
	return strings.ToUpper(f.Name)
}

type ArgGroupModel struct {
	Args []*ArgModel
}

func (a *ArgGroupModel) ArgSummary() string {
	depth := 0
	out := []string{}
	for _, arg := range a.Args {
		h := "<" + arg.Name + ">"
		if !arg.Required {
			h = "[" + h
			depth++
		}
		out = append(out, h)
	}
	out[len(out)-1] = out[len(out)-1] + strings.Repeat("]", depth)
	return strings.Join(out, " ")
}

type ArgModel struct {
	Name     string
	Help     string
	Default  string
	Required bool
	arg      *ArgClause
}

func (a *ArgModel) String() string {
	return a.arg.value.String()
}

type CmdGroupModel struct {
	Commands []*CmdModel
}

func (c *CmdGroupModel) FlattenedCommands() (out []*CmdModel) {
	for _, cmd := range c.Commands {
		if len(cmd.Commands) == 0 {
			out = append(out, cmd)
		}
		out = append(out, cmd.FlattenedCommands()...)
	}
	return
}

type CmdModel struct {
	Name string
	Help string
	*FlagGroupModel
	*ArgGroupModel
	*CmdGroupModel
	cmd *CmdClause
}

func (c *CmdModel) String() string {
	return c.cmd.FullCommand()
}

type ApplicationModel struct {
	Name string
	Help string
	*ArgGroupModel
	*CmdGroupModel
	*FlagGroupModel
}
