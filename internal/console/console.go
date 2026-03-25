package console

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

// ErrInterrupt signals user intent to stop the interactive session.
var ErrInterrupt = errors.New("console interrupted")

type readWriter struct {
	io.Reader
	io.Writer
}

// Session owns interactive terminal state so line history and cursor editing
// persist across prompts.
type Session struct {
	in       *os.File
	out      *os.File
	terminal *term.Terminal
	reader   *bufio.Reader
	mu       sync.Mutex
}

// NewSession creates a reusable interactive input session.
func NewSession(in *os.File, out *os.File) (*Session, error) {
	if in == nil || out == nil {
		return nil, fmt.Errorf("console input/output is not initialized")
	}

	session := &Session{
		in:  in,
		out: out,
	}

	if term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd())) {
		session.terminal = term.NewTerminal(readWriter{
			Reader: in,
			Writer: out,
		}, "")
		return session, nil
	}

	session.reader = bufio.NewReader(in)
	return session, nil
}

// ReadLine reads a line using persistent terminal state when attached to a TTY.
func (s *Session) ReadLine(prompt string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("console session is not initialized")
	}

	if s.terminal != nil {
		s.mu.Lock()
		defer s.mu.Unlock()

		oldState, err := term.MakeRaw(int(s.in.Fd()))
		if err != nil {
			return "", err
		}
		defer func() {
			_ = term.Restore(int(s.in.Fd()), oldState)
		}()

		s.terminal.SetPrompt(prompt)
		line, err := s.terminal.ReadLine()
		if errors.Is(err, io.EOF) {
			return "", ErrInterrupt
		}
		return line, err
	}

	if _, err := io.WriteString(s.out, prompt); err != nil {
		return "", err
	}
	line, err := s.reader.ReadString('\n')
	if errors.Is(err, io.EOF) && line == "" {
		return "", ErrInterrupt
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line, nil
}

// IsInterrupt reports whether err represents a user interrupt/quit request.
func IsInterrupt(err error) bool {
	return errors.Is(err, ErrInterrupt)
}
