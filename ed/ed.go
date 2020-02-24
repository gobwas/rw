package ed

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"

	"github.com/gobwas/rw/ioutil"
)

type Mode uint8

const (
	ModeZero Mode = iota
	ModeAdd
	ModeChange
	ModeDelete
)

func (m Mode) String() string {
	switch m {
	case ModeAdd:
		return "add"
	case ModeChange:
		return "change"
	case ModeDelete:
		return "delete"
	default:
		return "???"
	}
}

type Command struct {
	Start int
	End   int
	Mode  Mode
	Text  []byte
}

var zeroCommand Command

// Diff parses diff provided by r and sequentially calls fn with each found
// command.
func Diff(r io.Reader, fn func(Command)) (err error) {
	var (
		rb = bufio.NewReader(r)

		line []byte
		buf  []byte

		cmd Command

		txt []byte
	)
	for {
		line, buf, err = ioutil.ReadLine(rb, buf[:0])
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
		if cmd.Mode == ModeZero {
			cmd, err = ParseCommand(line)
			if err != nil {
				return err
			}
			if cmd.Mode == ModeDelete {
				fn(cmd)
				cmd = zeroCommand
			}
			continue
		}
		if bytes.Equal(line, []byte{'.'}) {
			cmd.Text = txt
			fn(cmd)

			cmd = zeroCommand
			txt = nil

			continue
		}
		txt = append(txt, line...)
		txt = append(txt, '\n')
	}
}

func ParseCommand(bts []byte) (Command, error) {
	var p commandParser
	p.bts = bts
	p.parser = p.num
	return p.Command, p.parse()
}

type commandState func() commandState

type commandParser struct {
	Command

	bts    []byte
	pos    int
	err    error
	parser commandState
}

func (c *commandParser) parse() error {
	for p := c.parser(); p != nil; p = p() {
	}
	return c.err
}

func (c *commandParser) next() byte {
	if c.pos >= len(c.bts) {
		return 0
	}
	return c.bts[c.pos]
}

func (c *commandParser) num() commandState {
	n := numbers(c.bts[c.pos:])
	if n == 0 {
		c.fatalf("no numeric characters")
		return nil
	}
	x, err := strconv.ParseInt(string(c.bts[c.pos:c.pos+n]), 10, 32)
	if err != nil {
		c.fatalf("parse int error: %v", err)
		return nil
	}
	c.pos += n

	if c.Command.Start == 0 {
		c.Command.Start = int(x)
		return c.rng
	}

	c.Command.End = int(x)
	return c.command
}

func (c *commandParser) rng() commandState {
	if c.next() == ',' {
		c.pos++
		return c.num
	}
	c.Command.End = c.Command.Start
	return c.command
}

func (c *commandParser) command() commandState {
	char := c.next()
	switch char {
	case 'a':
		c.Command.Mode = ModeAdd
	case 'c':
		c.Command.Mode = ModeChange
	case 'd':
		c.Command.Mode = ModeDelete
	default:
		c.fatalf("unexpected command type: %q", string(char))
	}
	return nil
}

func (c *commandParser) fatalf(f string, args ...interface{}) {
	c.err = fmt.Errorf(
		"ed: error parsing %q at %d: %s",
		c.bts, c.pos, fmt.Sprintf(f, args...),
	)
}

func numbers(p []byte) (n int) {
	for ; n < len(p); n++ {
		c := p[n]
		if '0' <= c && c <= '9' {
			continue
		}
		return n
	}
	return n
}
