package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"golang.org/x/term"
)

type terminalLoginSecrets struct {
	input  *bufio.Reader
	file   *os.File
	output io.Writer
}

func newTerminalLoginSecrets(input io.Reader, output io.Writer) *terminalLoginSecrets {
	secrets := &terminalLoginSecrets{input: bufio.NewReader(input), output: output}
	if file, ok := input.(*os.File); ok {
		secrets.file = file
	}
	return secrets
}

func (s *terminalLoginSecrets) Phone(context.Context) (string, error) {
	value, err := s.readHidden("Phone number: ")
	if err != nil {
		return "", err
	}
	return digitsOnly(value), nil
}

func (s *terminalLoginSecrets) OTP(context.Context) (string, error) {
	value, err := s.readHidden("SMS OTP: ")
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if len(value) != 6 || strings.IndexFunc(value, func(r rune) bool { return !unicode.IsDigit(r) }) != -1 {
		return "", errors.New("OTP must contain exactly six digits")
	}
	return value, nil
}

func (s *terminalLoginSecrets) readHidden(prompt string) (string, error) {
	_, _ = fmt.Fprint(s.output, prompt)
	if s.file != nil && term.IsTerminal(int(s.file.Fd())) {
		value, err := term.ReadPassword(int(s.file.Fd()))
		_, _ = fmt.Fprintln(s.output)
		if err != nil {
			return "", errors.New("read private login input")
		}
		return string(value), nil
	}
	value, err := s.input.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("read private login input")
	}
	if value == "" {
		return "", errors.New("read private login input")
	}
	return strings.TrimSpace(value), nil
}

func digitsOnly(value string) string {
	var result strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}
