package cli

import (
	"fmt"
	"io"
)

const (
	reset = "\033[0m"

	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"

	bold = "\033[1m"
)

type Printer struct {
	out io.Writer
}

func NewPrinter(out io.Writer) *Printer {
	return &Printer{
		out: out,
	}
}

func (p *Printer) Success(msg string) {
	fmt.Fprintln(p.out, green+"✓ "+msg+reset)
}

func (p *Printer) Error(msg string) {
	fmt.Fprintln(p.out, red+"✗ "+msg+reset)
}

func (p *Printer) Warning(msg string) {
	fmt.Fprintln(p.out, yellow+"⚠ "+msg+reset)
}

func (p *Printer) Info(msg string) {
	fmt.Fprintln(p.out, blue+"ℹ "+msg+reset)
}

func (p *Printer) Heading(msg string) {
	fmt.Fprintln(p.out, bold+cyan+msg+reset)
}
