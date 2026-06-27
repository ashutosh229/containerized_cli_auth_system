package cli

import (
	"fmt"
	"io"
	"strings"
)

const (
	reset = "\033[0m"

	black   = "\033[30m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"

	brightBlack = "\033[90m"
	brightWhite = "\033[97m"

	bold = "\033[1m"
)

type Printer struct {
	out io.Writer
}

type TableRow struct {
	Icon  string
	Label string
	Value string
	Color string
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

func (p *Printer) PrintTable(rows []TableRow) {
	if len(rows) == 0 {
		return
	}

	labelWidth := 0
	valueWidth := 0

	for _, row := range rows {
		if l := len(row.Icon) + len(row.Label) + 1; l > labelWidth {
			labelWidth = l
		}
		if len(row.Value) > valueWidth {
			valueWidth = len(row.Value)
		}
	}

	if labelWidth < 26 {
		labelWidth = 26
	}

	if valueWidth < 32 {
		valueWidth = 32
	}

	border := brightBlack

	fmt.Fprintln(p.out)

	fmt.Fprintf(
		p.out,
		"%s┏%s┳%s┓%s\n",
		border,
		strings.Repeat("━", labelWidth+2),
		strings.Repeat("━", valueWidth+2),
		reset,
	)

	for i, row := range rows {

		label := fmt.Sprintf("%s %s", row.Icon, row.Label)

		valueColor := brightWhite
		if row.Color != "" {
			valueColor = row.Color
		}

		fmt.Fprintf(
			p.out,
			"%s┃%s %-*s %s┃ %s%-*s%s %s┃%s\n",
			border,
			reset,
			labelWidth,
			label,
			border,
			valueColor,
			valueWidth,
			row.Value,
			reset,
			border,
			reset,
		)

		if i != len(rows)-1 {
			fmt.Fprintf(
				p.out,
				"%s┣%s╋%s┫%s\n",
				border,
				strings.Repeat("━", labelWidth+2),
				strings.Repeat("━", valueWidth+2),
				reset,
			)
		}
	}

	fmt.Fprintf(
		p.out,
		"%s┗%s┻%s┛%s\n\n",
		border,
		strings.Repeat("━", labelWidth+2),
		strings.Repeat("━", valueWidth+2),
		reset,
	)
}

func StatusColor(enabled bool) string {
	if enabled {
		return green
	}
	return yellow
}
