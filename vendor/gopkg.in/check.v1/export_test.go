package check

import "io"

func PrintLine(filename string, line int) (string, error) {
	return printLine(filename, line)
}

func Indent(s, with string) string {
	return indent(s, with)
}

func NewOutputWriter(writer io.Writer, stream, verbose bool) *outputWriter {
	return newOutputWriter(writer, stream, verbose)
}

func (c *C) FakeSkip(reason string) {
	c.reason = reason
}
