package cli

import (
	"fmt"
	"strings"

	"github.com/chzyer/readline"
)

func readLine(prompt string) (string, error) {
	line, err := readline.Line(prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func readPassword(prompt string) (string, error) {
	password, err := readline.Password(prompt)
	if err != nil {
		return "", err
	}
	if len(password) == 0 {
		return "", fmt.Errorf("%s cannot be empty", strings.TrimSuffix(prompt, ": "))
	}
	return string(password), nil
}
