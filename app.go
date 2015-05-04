// Package commander is used to manage a set of command-line "commands", with
// per-command flags and arguments.
//
// Supports command like so:
//
//   <command> <required> [<optional> [<optional> ...]]
//   <command> <remainder...>
//
// eg.
//
//   register [--name <name>] <nick>|<id>
//   post --channel|-a <channel> [--image <image>] [<text>]
//
// var (
//   chat = commander.New()
//   debug = chat.Flag("debug", "enable debug mode").Default("false").Bool()
//
//   register = chat.Command("register", "Register a new user.")
//   registerName = register.Flag("name", "name of user").Required().String()
//   registerNick = register.Arg("nick", "nickname for user").Required().String()
//
//   post = chat.Command("post", "Post a message to a channel.")
//   postChannel = post.Flag("channel", "channel to post to").Short('a').Required().String()
//   postImage = post.Flag("image", "image to post").String()
// )
//

package kingpin

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Action callback executed at various stages after all values are populated.
// The application, commands, arguments and flags all have corresponding
// actions.
type Action func(*ParseContext) error

type ApplicationValidator func(*Application) error

// An Application contains the definitions of flags, arguments and commands
// for an application.
type Application struct {
	*flagGroup
	*argGroup
	*cmdGroup
	initialized bool
	Name        string
	Help        string
	action      Action
	validator   ApplicationValidator
	terminate   func(status int) // See Terminate()
}

// New creates a new Kingpin application instance.
func New(name, help string) *Application {
	a := &Application{
		flagGroup: newFlagGroup(),
		argGroup:  newArgGroup(),
		Name:      name,
		Help:      help,
		terminate: func(status int) { os.Exit(status) },
	}
	a.cmdGroup = newCmdGroup(a)
	a.Flag("help", "Show help.").Bool()
	return a
}

// Terminate specifies the termination function. Defaults to os.Exit(status).
func (a *Application) Terminate(terminate func(int)) *Application {
	a.terminate = terminate
	return a
}

// Validate sets a validation function to run when parsing.
func (a *Application) Validate(validator ApplicationValidator) *Application {
	a.validator = validator
	return a
}

// ParseContext parses the given command line and returns the fully populated
// ParseContext.
func (a *Application) ParseContext(args []string) (*ParseContext, error) {
	if err := a.init(); err != nil {
		return nil, err
	}
	context := tokenize(args)
	err := a.parse(context)
	return context, err
}

// Parse parses command-line arguments. It returns the selected command and an
// error. The selected command will be a space separated subcommand, if
// subcommands have been configured.
//
// This will populate all flag and argument values, call all callbacks, and so
// on.
func (a *Application) Parse(args []string) (command string, err error) {
	context, err := a.ParseContext(args)
	if err != nil {
		if a.hasHelp(args) {
			a.Errorf(os.Stdout, "%s", err)
			a.usageForContext(os.Stdout, context)
			a.terminate(1)
		}
		return "", err
	}
	a.maybeHelp(context)
	if !context.EOL() {
		return "", fmt.Errorf("unexpected argument '%s'", context.Peek())
	}
	return a.execute(context)
}

func (a *Application) hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" {
			return true
		}
	}
	return false
}

func (a *Application) maybeHelp(context *ParseContext) {
	for _, element := range context.Elements {
		if flag, ok := element.Clause.(*FlagClause); ok && flag.name == "help" {
			a.usageForContext(os.Stdout, context)
			a.terminate(1)
		}
	}
}

// findCommandFromArgs finds a command (if any) from the given command line arguments.
func (a *Application) findCommandFromArgs(args []string) (command string, err error) {
	if err := a.init(); err != nil {
		return "", err
	}
	context := tokenize(args)
	if err := a.parse(context); err != nil {
		return "", err
	}
	return a.findCommandFromContext(context), nil
}

// findCommandFromContext finds a command (if any) from a parsed context.
func (a *Application) findCommandFromContext(context *ParseContext) string {
	commands := []string{}
	for _, element := range context.Elements {
		if c, ok := element.Clause.(*CmdClause); ok {
			commands = append(commands, c.name)
		}
	}
	return strings.Join(commands, " ")
}

// Version adds a --version flag for displaying the application version.
func (a *Application) Version(version string) *Application {
	a.Flag("version", "Show application version.").Action(func(*ParseContext) error {
		fmt.Println(version)
		a.terminate(0)
		return nil
	}).Bool()
	return a
}

// Action callback to call when all values are populated and parsing is
// complete, but before any command, flag or argument actions.
//
// All Action() callbacks are called in the order they are encountered on the
// command line.
func (a *Application) Action(action Action) *Application {
	a.action = action
	return a
}

// Command adds a new top-level command.
func (a *Application) Command(name, help string) *CmdClause {
	return a.addCommand(name, help)
}

func (a *Application) init() error {
	if a.initialized {
		return nil
	}
	if a.cmdGroup.have() && a.argGroup.have() {
		return fmt.Errorf("can't mix top-level Arg()s with Command()s")
	}

	if err := a.flagGroup.init(); err != nil {
		return err
	}
	if err := a.cmdGroup.init(); err != nil {
		return err
	}
	if err := a.argGroup.init(); err != nil {
		return err
	}
	for _, cmd := range a.commands {
		if err := cmd.init(); err != nil {
			return err
		}
	}
	flagGroups := []*flagGroup{a.flagGroup}
	for _, cmd := range a.commandOrder {
		if err := checkDuplicateFlags(cmd, flagGroups); err != nil {
			return err
		}
	}
	a.initialized = true
	return nil
}

// Recursively check commands for duplicate flags.
func checkDuplicateFlags(current *CmdClause, flagGroups []*flagGroup) error {
	// Check for duplicates.
	for _, flags := range flagGroups {
		for _, flag := range current.flagOrder {
			if flag.shorthand != 0 {
				if _, ok := flags.short[string(flag.shorthand)]; ok {
					return fmt.Errorf("duplicate short flag -%c", flag.shorthand)
				}
			}
			if _, ok := flags.long[flag.name]; ok {
				return fmt.Errorf("duplicate long flag --%s", flag.name)
			}
		}
	}
	flagGroups = append(flagGroups, current.flagGroup)
	// Check subcommands.
	for _, subcmd := range current.commandOrder {
		if err := checkDuplicateFlags(subcmd, flagGroups); err != nil {
			return err
		}
	}
	return nil
}

func parsePositional(context *ParseContext, cmds *cmdGroup, args *argGroup) (selected []string, err error) {
	if cmds.have() {
		selected, err = cmds.parse(context)
	} else if args.have() {
		context.mergeArgs(args)
		err = args.parse(context)
	}
	return
}

func parseNode(context *ParseContext, flags *flagGroup, args *argGroup, cmds *cmdGroup) (selected []string, err error) {
	context.mergeFlags(flags)
	seenArg := false
loop:
	for !context.EOL() && err == nil {
		token := context.Peek()
		switch token.Type {
		case TokenLong, TokenShort:
			err = context.flags.parse(context)

		case TokenArg:
			if seenArg {
				return nil, fmt.Errorf("unexpected %s", token)
			}
			selected, err = parsePositional(context, cmds, args)
			seenArg = true

		default:
			break loop
		}

		// Token has not moved; nothing matched.
		if !context.EOL() && token.Equal(context.Peek()) {
			return nil, fmt.Errorf("unexpected %s", token)
		}
	}
	if !seenArg && context.Peek().Type == TokenArg {
		selected, err = parsePositional(context, cmds, args)
	}
	return
}

func (a *Application) parse(context *ParseContext) (err error) {
	_, err = parseNode(context, a.flagGroup, a.argGroup, a.cmdGroup)
	return
}

func (a *Application) execute(context *ParseContext) (string, error) {
	var err error
	selected := []string{}

	if err = a.setDefaults(context); err != nil {
		return "", err
	}

	selected, err = a.setValues(context)
	if err != nil {
		return "", err
	}

	if err = a.applyValidators(context); err != nil {
		return "", err
	}

	if err = a.applyActions(context); err != nil {
		return "", err
	}

	return strings.Join(selected, " "), err
}

func (a *Application) setDefaults(context *ParseContext) error {
	flagElements := map[string]*ParseElement{}
	for _, element := range context.Elements {
		if flag, ok := element.Clause.(*FlagClause); ok {
			flagElements[flag.name] = element
		}
	}

	argElements := map[string]*ParseElement{}
	for _, element := range context.Elements {
		if arg, ok := element.Clause.(*ArgClause); ok {
			argElements[arg.name] = element
		}
	}

	// Check required flags and set defaults.
	for _, flag := range context.flags.long {
		if flagElements[flag.name] == nil {
			// Check required flags were provided.
			if flag.needsValue() {
				return fmt.Errorf("required flag --%s not provided", flag.name)
			}
			// Set defaults, if any.
			if flag.defaultValue != "" {
				if err := flag.value.Set(flag.defaultValue); err != nil {
					return err
				}
			}
		}
	}

	for _, arg := range context.arguments.args {
		if argElements[arg.name] == nil {
			if arg.required {
				return fmt.Errorf("required argument '%s' not provided", arg.name)
			}
			// Set defaults, if any.
			if arg.defaultValue != "" {
				if err := arg.value.Set(arg.defaultValue); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (a *Application) setValues(context *ParseContext) (selected []string, err error) {
	// Set all arg and flag values.
	var lastCmd *CmdClause
	for _, element := range context.Elements {
		switch clause := element.Clause.(type) {
		case *FlagClause:
			if err = clause.value.Set(*element.Value); err != nil {
				return
			}

		case *ArgClause:
			if err = clause.value.Set(*element.Value); err != nil {
				return
			}

		case *CmdClause:
			if clause.validator != nil {
				if err = clause.validator(clause); err != nil {
					return
				}
			}
			selected = append(selected, clause.name)
			lastCmd = clause
		}
	}

	if lastCmd != nil && len(lastCmd.commands) > 0 {
		return nil, fmt.Errorf("must select a subcommand of '%s'", lastCmd.FullCommand())
	}

	return
}

func (a *Application) applyValidators(context *ParseContext) (err error) {
	// Call command validation functions.
	for _, element := range context.Elements {
		if cmd, ok := element.Clause.(*CmdClause); ok && cmd.validator != nil {
			if err = cmd.validator(cmd); err != nil {
				return err
			}
		}
	}

	if a.validator != nil {
		err = a.validator(a)
	}
	return err
}

func (a *Application) applyActions(context *ParseContext) error {
	if a.action != nil {
		if err := a.action(context); err != nil {
			return err
		}
	}
	// Dispatch to actions.
	for _, element := range context.Elements {
		switch clause := element.Clause.(type) {
		case *ArgClause:
			if clause.dispatch != nil {
				if err := clause.dispatch(context); err != nil {
					return err
				}
			}
		case *CmdClause:
			if clause.dispatch != nil {
				if err := clause.dispatch(context); err != nil {
					return err
				}
			}
		case *FlagClause:
			if clause.dispatch != nil {
				if err := clause.dispatch(context); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Errorf prints an error message to w in the format "<appname>: error: <message>".
func (a *Application) Errorf(w io.Writer, format string, args ...interface{}) {
	fmt.Fprintf(w, a.Name+": error: "+format+"\n", args...)
}

// Fatalf writes a formatted error to w then terminates with exit status 1.
func (a *Application) Fatalf(w io.Writer, format string, args ...interface{}) {
	a.Errorf(w, format, args...)
	a.terminate(1)
}

// UsageErrorf prints an error message followed by usage information, then
// exits with a non-zero status.
func (a *Application) UsageErrorf(w io.Writer, format string, args ...interface{}) {
	a.Errorf(w, format, args...)
	a.Usage(w, []string{})
	a.terminate(1)
}

// UsageErrorContextf writes a printf formatted error message to w, then usage
// information for the given ParseContext, before exiting.
func (a *Application) UsageErrorContextf(w io.Writer, context *ParseContext, format string, args ...interface{}) {
	a.Errorf(w, format, args...)
	a.usageForContext(w, context)
	a.terminate(1)
}

// FatalIfError prints an error and exits if err is not nil. The error is printed
// with the given prefix if any.
func (a *Application) FatalIfError(w io.Writer, err error, prefix string) {
	if err != nil {
		if prefix != "" {
			prefix += ": "
		}
		a.Errorf(w, prefix+"%s", err)
		a.terminate(1)
	}
}
