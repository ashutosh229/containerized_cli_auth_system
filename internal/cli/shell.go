package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"containerized_cli_auth_system/internal/auth"

	"github.com/chzyer/readline"
)

type Shell struct {
	in         io.Reader
	out        io.Writer
	auth       *auth.Service
	sessionID  string
	completer  *readline.PrefixCompleter
	commandSet map[string]command
}

type command struct {
	description string
	handler     func(context.Context, []string) error
	authOnly    bool
	guestOnly   bool
}

func NewShell(in io.Reader, out io.Writer, service *auth.Service) *Shell {
	s := &Shell{in: in, out: out, auth: service}
	s.commandSet = map[string]command{
		"register":   {description: "create a new user", handler: s.register, guestOnly: true},
		"login":      {description: "login with username/password and optional TOTP", handler: s.login, guestOnly: true},
		"whoami":     {description: "show current user details", handler: s.whoami, authOnly: true},
		"enable-2fa": {description: "enable Google Authenticator compatible MFA", handler: s.enable2FA, authOnly: true},
		"disable-2fa": {description: "disable MFA after password and TOTP verification",
			handler: s.disable2FA, authOnly: true},
		"logout": {description: "end current session", handler: s.logout, authOnly: true},
		"help":   {description: "show available commands", handler: s.help},
		"exit":   {description: "quit program", handler: s.exit},
	}
	s.completer = readline.NewPrefixCompleter(
		readline.PcItem("register"),
		readline.PcItem("login"),
		readline.PcItem("whoami"),
		readline.PcItem("enable-2fa"),
		readline.PcItem("disable-2fa"),
		readline.PcItem("logout"),
		readline.PcItem("help"),
		readline.PcItem("exit"),
	)
	return s
}

func (s *Shell) Run(ctx context.Context) error {
	cfg := &readline.Config{
		Prompt:          s.prompt(),
		HistoryFile:     "/tmp/authcli-history",
		AutoComplete:    s.completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		Stdout:          s.out,
		Stderr:          s.out,
	}
	if stdin, ok := s.in.(io.ReadCloser); ok {
		cfg.Stdin = stdin
	}
	rl, err := readline.NewEx(cfg)
	if err != nil {
		return err
	}
	defer rl.Close()

	fmt.Fprintln(s.out, "Secure CLI Login System. Type help for commands.")
	for {
		rl.SetPrompt(s.prompt())
		line, err := rl.Readline()
		if errors.Is(err, readline.ErrInterrupt) {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.dispatch(ctx, strings.Fields(line)); err != nil {
			if errors.Is(err, errExit) {
				return nil
			}
			fmt.Fprintf(s.out, "Error: %v\n", err)
		}
	}
}

func (s *Shell) dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return nil
	}
	cmd, ok := s.commandSet[args[0]]
	if !ok {
		return fmt.Errorf("unknown command %q; type help", args[0])
	}
	loggedIn := s.sessionID != ""
	if loggedIn {
		if _, err := s.auth.Current(s.sessionID); err != nil {
			s.sessionID = ""
			fmt.Fprintln(s.out, "Session expired. Please login again.")
			loggedIn = false
		}
	}
	if cmd.authOnly && !loggedIn {
		return errors.New("please login first")
	}
	if cmd.guestOnly && loggedIn {
		return errors.New("logout before using this command")
	}
	return cmd.handler(ctx, args[1:])
}

func (s *Shell) prompt() string {
	if s.sessionID == "" {
		return "auth> "
	}
	return "auth# "
}

func (s *Shell) register(ctx context.Context, _ []string) error {
	username, err := readLine("Username: ")
	if err != nil {
		return err
	}
	password, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	if _, err := s.auth.Register(ctx, username, password); err != nil {
		return err
	}
	fmt.Fprintln(s.out, "Registration successful. You can now login.")
	return nil
}

func (s *Shell) login(ctx context.Context, _ []string) error {
	username, err := readLine("Username: ")
	if err != nil {
		return err
	}
	password, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	result, err := s.auth.Login(ctx, username, password, "")
	if errors.Is(err, auth.ErrMFARequired) {
		code, codeErr := readLine("Authenticator code: ")
		if codeErr != nil {
			return codeErr
		}
		result, err = s.auth.Login(ctx, username, password, code)
	}
	if err != nil {
		return err
	}
	s.sessionID = result.Session.ID
	fmt.Fprintln(s.out, "Login successful.")
	s.printUserDetails(result.Session)
	return nil
}

func (s *Shell) whoami(ctx context.Context, _ []string) error {
	session, err := s.auth.RefreshSession(ctx, s.sessionID)
	if err != nil {
		return err
	}
	s.printUserDetails(session)
	return nil
}

func (s *Shell) enable2FA(ctx context.Context, _ []string) error {
	session, err := s.auth.Current(s.sessionID)
	if err != nil {
		return err
	}
	if session.User.MFAEnabled {
		return errors.New("MFA is already enabled")
	}
	secret, url, err := s.auth.BeginEnableTOTP(session.User.Username)
	if err != nil {
		return err
	}
	fmt.Fprintln(s.out, "Add this secret to Google Authenticator or another TOTP app:")
	fmt.Fprintf(s.out, "Secret: %s\n", secret)
	fmt.Fprintf(s.out, "otpauth URL: %s\n", url)
	code, err := readLine("Enter the current authenticator code to confirm: ")
	if err != nil {
		return err
	}
	if err := s.auth.ConfirmEnableTOTP(ctx, session.User.Username, secret, code); err != nil {
		return err
	}
	if _, err := s.auth.RefreshSession(ctx, s.sessionID); err != nil {
		return err
	}
	fmt.Fprintln(s.out, "MFA enabled successfully.")
	return nil
}

func (s *Shell) disable2FA(ctx context.Context, _ []string) error {
	session, err := s.auth.Current(s.sessionID)
	if err != nil {
		return err
	}
	password, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	code := ""
	if session.User.MFAEnabled {
		code, err = readLine("Authenticator code: ")
		if err != nil {
			return err
		}
	}
	if err := s.auth.DisableTOTP(ctx, session.User.Username, password, code); err != nil {
		return err
	}
	if _, err := s.auth.RefreshSession(ctx, s.sessionID); err != nil {
		return err
	}
	fmt.Fprintln(s.out, "MFA disabled.")
	return nil
}

func (s *Shell) logout(context.Context, []string) error {
	s.auth.Logout(s.sessionID)
	s.sessionID = ""
	fmt.Fprintln(s.out, "Logged out.")
	return nil
}

func (s *Shell) help(context.Context, []string) error {
	loggedIn := s.sessionID != ""
	fmt.Fprintln(s.out, "Available commands:")
	for name, cmd := range s.commandSet {
		if cmd.authOnly && !loggedIn {
			continue
		}
		if cmd.guestOnly && loggedIn {
			continue
		}
		fmt.Fprintf(s.out, "  %-12s %s\n", name, cmd.description)
	}
	return nil
}

var errExit = errors.New("exit")

func (s *Shell) exit(context.Context, []string) error {
	return errExit
}

func (s *Shell) printUserDetails(session auth.Session) {
	user := session.User
	fmt.Fprintf(s.out, "Username: %s\n", user.Username)
	fmt.Fprintf(s.out, "Registration date: %s\n", formatTime(user.RegisteredAt))
	status := "disabled"
	if user.MFAEnabled {
		status = "enabled"
	}
	fmt.Fprintf(s.out, "MFA status: %s\n", status)
	fmt.Fprintf(s.out, "Session expiration time: %s\n", formatTime(session.ExpiresAt))
	if user.LastLoginAt == nil {
		fmt.Fprintln(s.out, "Last login time: never")
		return
	}
	fmt.Fprintf(s.out, "Last login time: %s\n", formatTime(*user.LastLoginAt))
}

func formatTime(t time.Time) string {
	return t.Local().Format(time.RFC1123)
}
