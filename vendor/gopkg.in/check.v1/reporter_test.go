package check_test

import (
	"fmt"
	"path/filepath"
	"runtime"

	. "gopkg.in/check.v1"
)

var _ = Suite(&reporterS{})

type reporterS struct {
	testFile string
}

func (s *reporterS) SetUpSuite(c *C) {
	_, fileName, _, ok := runtime.Caller(0)
	c.Assert(ok, Equals, true)
	s.testFile = filepath.Base(fileName)
}

func (s *reporterS) TestWrite(c *C) {
	testString := "test string"
	output := String{}

	dummyStream := true
	dummyVerbose := true
	o := NewOutputWriter(&output, dummyStream, dummyVerbose)

	o.Write([]byte(testString))
	c.Assert(output.value, Equals, testString)
}

func (s *reporterS) TestWriteCallStartedWithStreamFlag(c *C) {
	testLabel := "test started label"
	stream := true
	output := String{}

	dummyVerbose := true
	o := NewOutputWriter(&output, stream, dummyVerbose)

	o.WriteCallStarted(testLabel, c)
	expected := fmt.Sprintf("%s: %s:\\d+: %s\n", testLabel, s.testFile, c.TestName())
	c.Assert(output.value, Matches, expected)
}

func (s *reporterS) TestWriteCallStartedWithoutStreamFlag(c *C) {
	stream := false
	output := String{}

	dummyLabel := "dummy"
	dummyVerbose := true
	o := NewOutputWriter(&output, stream, dummyVerbose)

	o.WriteCallStarted(dummyLabel, c)
	c.Assert(output.value, Equals, "")
}

func (s *reporterS) TestWriteCallProblemWithStreamFlag(c *C) {
	testLabel := "test problem label"
	stream := true
	output := String{}

	dummyVerbose := true
	o := NewOutputWriter(&output, stream, dummyVerbose)

	o.WriteCallProblem(testLabel, c)
	expected := fmt.Sprintf("%s: %s:\\d+: %s\n\n", testLabel, s.testFile, c.TestName())
	c.Assert(output.value, Matches, expected)
}

func (s *reporterS) TestWriteCallProblemWithoutStreamFlag(c *C) {
	testLabel := "test problem label"
	stream := false
	output := String{}

	dummyVerbose := true
	o := NewOutputWriter(&output, stream, dummyVerbose)

	o.WriteCallProblem(testLabel, c)
	expected := fmt.Sprintf(""+
		"\n"+
		"----------------------------------------------------------------------\n"+
		"%s: %s:\\d+: %s\n\n", testLabel, s.testFile, c.TestName())
	c.Assert(output.value, Matches, expected)
}

func (s *reporterS) TestWriteCallProblemWithoutStreamFlagWithLog(c *C) {
	testLabel := "test problem label"
	testLog := "test log"
	stream := false
	output := String{}

	dummyVerbose := true
	o := NewOutputWriter(&output, stream, dummyVerbose)

	c.Log(testLog)
	o.WriteCallProblem(testLabel, c)
	expected := fmt.Sprintf(""+
		"\n"+
		"----------------------------------------------------------------------\n"+
		"%s: %s:\\d+: %s\n\n%s\n", testLabel, s.testFile, c.TestName(), testLog)
	c.Assert(output.value, Matches, expected)
}

func (s *reporterS) TestWriteCallSuccessWithStreamFlag(c *C) {
	testLabel := "test success label"
	stream := true
	output := String{}

	dummyVerbose := true
	o := NewOutputWriter(&output, stream, dummyVerbose)

	o.WriteCallSuccess(testLabel, c)
	expected := fmt.Sprintf("%s: %s:\\d+: %s\t\\d\\.\\d+s\n\n", testLabel, s.testFile, c.TestName())
	c.Assert(output.value, Matches, expected)
}

func (s *reporterS) TestWriteCallSuccessWithStreamFlagAndReason(c *C) {
	testLabel := "test success label"
	testReason := "test skip reason"
	stream := true
	output := String{}

	dummyVerbose := true
	o := NewOutputWriter(&output, stream, dummyVerbose)
	c.FakeSkip(testReason)

	o.WriteCallSuccess(testLabel, c)
	expected := fmt.Sprintf("%s: %s:\\d+: %s \\(%s\\)\t\\d\\.\\d+s\n\n",
		testLabel, s.testFile, c.TestName(), testReason)
	c.Assert(output.value, Matches, expected)
}

func (s *reporterS) TestWriteCallSuccessWithoutStreamFlagWithVerboseFlag(c *C) {
	testLabel := "test success label"
	stream := false
	verbose := true
	output := String{}

	o := NewOutputWriter(&output, stream, verbose)

	o.WriteCallSuccess(testLabel, c)
	expected := fmt.Sprintf("%s: %s:\\d+: %s\t\\d\\.\\d+s\n", testLabel, s.testFile, c.TestName())
	c.Assert(output.value, Matches, expected)
}

func (s *reporterS) TestWriteCallSuccessWithoutStreamFlagWithoutVerboseFlag(c *C) {
	testLabel := "test success label"
	stream := false
	verbose := false
	output := String{}

	o := NewOutputWriter(&output, stream, verbose)

	o.WriteCallSuccess(testLabel, c)
	c.Assert(output.value, Equals, "")
}
