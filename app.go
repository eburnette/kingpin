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

type Dispatch func(*ParseContext) error

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
	validator   ApplicationValidator
}

// New creates a new Kingpin application instance.
func New(name, help string) *Application {
	a := &Application{
		flagGroup: newFlagGroup(),
		argGroup:  newArgGroup(),
		Name:      name,
		Help:      help,
	}
	a.cmdGroup = newCmdGroup(a)
	a.Flag("help", "Show help.").Action(a.onHelp).Bool()
	return a
}

// Validate sets a validation function to run when parsing.
func (a *Application) Validate(validator ApplicationValidator) *Application {
	a.validator = validator
	return a
}

// Parse parses command-line arguments. It returns the selected command and an
// error. The selected command will be a space separated subcommand, if
// subcommands have been configured.
func (a *Application) Parse(args []string) (command string, err error) {
	if err := a.init(); err != nil {
		return "", err
	}
	context := tokenize(args)
	command, err = a.parse(context)
	if err != nil {
		return "", err
	}

	if !context.EOL() {
		return "", fmt.Errorf("unexpected argument '%s'", context.Peek())
	}

	return command, err
}

// Version adds a --version flag for displaying the application version.
func (a *Application) Version(version string) *Application {
	a.Flag("version", "Show application version.").Action(func(*ParseContext) error {
		fmt.Println(version)
		os.Exit(0)
		return nil
	}).Bool()
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

	if len(a.commands) > 0 {
		cmd := a.Command("help", "Show help for a command.").Action(a.onHelp)
		cmd.Arg("command", "Command name.").String()
		// Make "help" command first in order. Also, Go's slice operations are woeful.
		l := len(a.commandOrder) - 1
		a.commandOrder = append(a.commandOrder[l:], a.commandOrder[:l]...)
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
			if flag.name != "help" {
				if _, ok := flags.long[flag.name]; ok {
					return fmt.Errorf("duplicate long flag --%s", flag.name)
				}
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

func (a *Application) onHelp(context *ParseContext) error {
	candidates := []string{}
	for {
		token := context.Peek()
		if token.Type == TokenArg {
			candidates = append(candidates, token.String())
			context.Next()
		} else {
			break
		}
	}

	var cmd *CmdClause
	for i := len(candidates); i > 0; i-- {
		command := strings.Join(candidates[:i], " ")
		cmd = a.findCommand(command)
		if cmd != nil {
			a.CommandUsage(os.Stderr, command)
			break
		}
	}
	if cmd == nil {
		a.Usage(os.Stderr)
	}
	os.Exit(0)
	return nil
}

func (a *Application) parse(context *ParseContext) (string, error) {
	context.mergeFlags(a.flagGroup)

	// Special-case "help" to avoid issues with required flags.
	runHelp := (context.Peek().IsFlag() && context.Peek().Value == "help")

	var err error
	err = a.flagGroup.parse(context, runHelp)
	if err != nil {
		return "", err
	}

	selected := []string{}

	// Parse arguments or commands.
	if a.argGroup.have() {
		err = a.argGroup.parse(context)
	} else if a.cmdGroup.have() {
		selected, err = a.cmdGroup.parse(context)
	}
	if a.validator != nil {
		err = a.validator(a)
	}
	return strings.Join(selected, " "), err
}

// Errorf prints an error message to w.
func (a *Application) Errorf(w io.Writer, format string, args ...interface{}) {
	fmt.Fprintf(w, a.Name+": error: "+format+"\n", args...)
}

func (a *Application) Fatalf(w io.Writer, format string, args ...interface{}) {
	a.Errorf(w, format, args...)
	os.Exit(1)
}

// UsageErrorf prints an error message followed by usage information, then
// exits with a non-zero status.
func (a *Application) UsageErrorf(w io.Writer, format string, args ...interface{}) {
	a.Errorf(w, format, args...)
	a.Usage(w)
	os.Exit(1)
}

// FatalIfError prints an error and exits if err is not nil. The error is printed
// with the given prefix.
func (a *Application) FatalIfError(w io.Writer, err error, prefix string) {
	if err != nil {
		if prefix != "" {
			prefix += ": "
		}
		a.Errorf(w, prefix+"%s", err)
		os.Exit(1)
	}
}
