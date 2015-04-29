package kingpin

import "fmt"

type argGroup struct {
	args  []*ArgClause
	model *ArgGroupModel
}

func newArgGroup() *argGroup {
	return &argGroup{
		model: &ArgGroupModel{},
	}
}

func (a *argGroup) have() bool {
	return len(a.args) > 0
}

func (a *argGroup) Arg(name, help string) *ArgClause {
	arg := newArg(name, help)
	a.args = append(a.args, arg)
	a.model.Args = append(a.model.Args, arg.Model)
	arg.Model.arg = arg
	return arg
}

func (a *argGroup) parse(context *ParseContext) error {
	i := 0
	var last *Token
	consumed := 0
	for i < len(a.args) {
		arg := a.args[i]
		token := context.Peek()
		if token.Type == TokenEOL {
			if consumed == 0 && arg.Model.Required {
				return fmt.Errorf("'%s' is required", arg.Model.Name)
			}
			break
		}

		var err error
		err = arg.parse(context)
		if err != nil {
			return err
		}

		if arg.consumesRemainder() {
			if last == context.Peek() {
				return fmt.Errorf("expected positional arguments <%s>", arg.Model.Name)
			}
			consumed++
		} else {
			i++
		}
		last = token
	}

	// Set defaults for all remaining args.
	for i < len(a.args) {
		arg := a.args[i]
		if arg.Model.Default != "" {
			if err := arg.value.Set(arg.Model.Default); err != nil {
				return fmt.Errorf("invalid default value '%s' for argument '%s'", arg.Model.Default, arg.Model.Name)
			}
		}
		i++
	}
	return nil
}

func (a *argGroup) init() error {
	required := 0
	seen := map[string]struct{}{}
	previousArgMustBeLast := false
	for i, arg := range a.args {
		if previousArgMustBeLast {
			return fmt.Errorf("Args() can't be followed by another argument '%s'", arg.Model.Name)
		}
		if arg.consumesRemainder() {
			previousArgMustBeLast = true
		}
		if _, ok := seen[arg.Model.Name]; ok {
			return fmt.Errorf("duplicate argument '%s'", arg.Model.Name)
		}
		seen[arg.Model.Name] = struct{}{}
		if arg.Model.Required && required != i {
			return fmt.Errorf("required arguments found after non-required")
		}
		if arg.Model.Required {
			required++
		}
		if err := arg.init(); err != nil {
			return err
		}
	}
	return nil
}

type ArgClause struct {
	parserMixin
	Model    *ArgModel
	dispatch Action
}

func newArg(name, help string) *ArgClause {
	a := &ArgClause{
		Model: &ArgModel{
			Name: name,
			Help: help,
		},
	}
	return a
}

func (a *ArgClause) consumesRemainder() bool {
	if r, ok := a.value.(remainderArg); ok {
		return r.IsCumulative()
	}
	return false
}

// Required arguments must be input by the user. They can not have a Default() value provided.
func (a *ArgClause) Required() *ArgClause {
	a.Model.Required = true
	return a
}

// Default value for this argument. It *must* be parseable by the value of the argument.
func (a *ArgClause) Default(value string) *ArgClause {
	a.Model.Default = value
	return a
}

func (a *ArgClause) Action(dispatch Action) *ArgClause {
	a.dispatch = dispatch
	return a
}

func (a *ArgClause) init() error {
	if a.Model.Required && a.Model.Default != "" {
		return fmt.Errorf("required argument '%s' with unusable default value", a.Model.Name)
	}
	if a.value == nil {
		return fmt.Errorf("no parser defined for arg '%s'", a.Model.Name)
	}
	return nil
}

func (a *ArgClause) parse(context *ParseContext) error {
	token := context.Peek()
	if token.Type != TokenArg {
		return fmt.Errorf("expected positional argument <%s>", a.Model.Name)
	}
	context.matchedArg(a, token.Value)
	context.Next()
	return nil
}
