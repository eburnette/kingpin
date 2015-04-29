package kingpin

import (
	"fmt"
	"os"
	"strings"
)

type flagGroup struct {
	short     map[string]*FlagClause
	long      map[string]*FlagClause
	flagOrder []*FlagClause
	model     *FlagGroupModel
}

func newFlagGroup() *flagGroup {
	return &flagGroup{
		short: make(map[string]*FlagClause),
		long:  make(map[string]*FlagClause),
		model: &FlagGroupModel{},
	}
}

// Flag defines a new flag with the given long name and help.
func (f *flagGroup) Flag(name, help string) *FlagClause {
	flag := newFlag(name, help)
	f.long[name] = flag
	f.flagOrder = append(f.flagOrder, flag)
	f.model.Flags = append(f.model.Flags, flag.Model)
	flag.Model.flag = flag
	return flag
}

func (f *flagGroup) init() error {
	for _, flag := range f.long {
		if err := flag.init(); err != nil {
			return err
		}
		if flag.Model.Short != 0 {
			f.short[string(flag.Model.Short)] = flag
		}
	}
	return nil
}

func (f *flagGroup) parse(context *ParseContext) error {
	var token *Token

loop:
	for {
		token = context.Peek()
		switch token.Type {
		case TokenEOL:
			break loop

		case TokenLong, TokenShort:
			flagToken := token
			defaultValue := ""
			var flag *FlagClause
			var ok bool
			invert := false

			name := token.Value
			if token.Type == TokenLong {
				if strings.HasPrefix(name, "no-") {
					name = name[3:]
					invert = true
				}
				flag, ok = f.long[name]
				if !ok {
					return fmt.Errorf("unknown long flag '%s'", flagToken)
				}
			} else {
				flag, ok = f.short[name]
				if !ok {
					return fmt.Errorf("unknown short flag '%s'", flagToken)
				}
			}

			context.Next()

			fb, ok := flag.value.(boolFlag)
			if ok && fb.IsBoolFlag() {
				if invert {
					defaultValue = "false"
				} else {
					defaultValue = "true"
				}
			} else {
				if invert {
					return fmt.Errorf("unknown long flag '%s'", flagToken)
				}
				token = context.Peek()
				if token.Type != TokenArg {
					return fmt.Errorf("expected argument for flag '%s'", flagToken)
				}
				context.Next()
				defaultValue = token.Value
			}

			context.matchedFlag(flag, defaultValue)

		default:
			break loop
		}
	}
	return nil
}

func (f *flagGroup) visibleFlags() int {
	count := 0
	for _, flag := range f.long {
		if !flag.Model.Hidden {
			count++
		}
	}
	return count
}

// FlagClause is a fluid interface used to build flags.
type FlagClause struct {
	parserMixin
	Model    *FlagModel
	dispatch Action
}

func newFlag(name, help string) *FlagClause {
	f := &FlagClause{
		Model: &FlagModel{
			Name: name,
			Help: help,
		},
	}
	return f
}

func (f *FlagClause) needsValue() bool {
	return f.Model.Required && f.Model.Default == ""
}

func (f *FlagClause) init() error {
	if f.Model.Required && f.Model.Default != "" {
		return fmt.Errorf("required flag '--%s' with default value that will never be used", f.Model.Name)
	}
	if f.value == nil {
		return fmt.Errorf("no type defined for --%s (eg. .String())", f.Model.Name)
	}
	if f.Model.Envar != "" {
		if v := os.Getenv(f.Model.Envar); v != "" {
			f.Model.Default = v
		}
	}
	return nil
}

// Dispatch to the given function when the flag is parsed.
func (f *FlagClause) Action(dispatch Action) *FlagClause {
	f.dispatch = dispatch
	return f
}

// Default value for this flag. It *must* be parseable by the value of the flag.
func (f *FlagClause) Default(value string) *FlagClause {
	f.Model.Default = value
	return f
}

// OverrideDefaultFromEnvar overrides the default value for a flag from an
// environment variable, if available.
func (f *FlagClause) OverrideDefaultFromEnvar(envar string) *FlagClause {
	f.Model.Envar = envar
	return f
}

// PlaceHolder sets the place-holder string used for flag values in the help. The
// default behaviour is to use the value provided by Default() if provided,
// then fall back on the capitalized flag name.
func (f *FlagClause) PlaceHolder(placeholder string) *FlagClause {
	f.Model.PlaceHolder = placeholder
	return f
}

// Hidden hides a flag from usage but still allows it to be used.
func (f *FlagClause) Hidden() *FlagClause {
	f.Model.Hidden = true
	return f
}

// Required enforces the constraint that this flag must be populated by the user. You can not provide a Default() value to a Required() flag.
func (f *FlagClause) Required() *FlagClause {
	f.Model.Required = true
	return f
}

// Short sets the short flag name.
func (f *FlagClause) Short(name rune) *FlagClause {
	f.Model.Short = name
	return f
}

// Bool makes this flag a boolean flag.
func (f *FlagClause) Bool() (target *bool) {
	target = new(bool)
	f.SetValue(newBoolValue(target))
	return
}
