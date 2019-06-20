// Copyright 2019 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmdtest

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-cmp/cmp"
)

// A TestFile describes a single file that may contain multiple test cases.
// File format:
//
// Before the first line starting with a '$', empty lines and lines beginning with
// "#" are ignored.
//
// A sequence of consecutive lines starting with '$' begin test case. These lines are
// commands to execute. See below for the valid commands.
//
// Lines following the '$' lines are command output (merged stdout and stderr).
// Output is always treated literally.
// After the command output there should be a blank line. Between that blank line
// and the next '$' line, empty lines and lines beginning with '#' are ignored.
// (Because of these rules, cmdtest cannot distinguish trailing blanks in the output.)
//
// Syntax of a line beginning with '$':
// A sequence of space-separated words (no quoting is supported). The first word is
// the command, the rest are its args. If the next-to-last word is '<', the last word
// is interpreted as a file and becomes the standard input to the command (only for
// commands run with exec.Command).
//
// By default, commands are expected to succeed, and the test will fail
// otherwise. However, commands that are expected to fail can be marked
// with a " --> FAIL" suffix.
//
// The cases of a test file are executed in order, starting from a freshly created temporary
// directory.
type TestFile struct {
	// Echo each command and its output as it's run
	Verbose bool

	// If non-nil, this function is called with the root directory after it has been made
	// the current directory.
	Setup func(string) error

	// If true, don't delete the test's temporary root directory, and print it out
	// its name for debugging.
	KeepRootDir bool

	// Special commands that are not executed via exec.Command (like shell
	// built-ins).
	Commands map[string]func(args []string) ([]byte, error)

	filename string // full filename of the test file
	cases    []*TestCase
	suffix   []string // non-output lines after last case
}

type TestCase struct {
	before    []string // lines before the commands
	startLine int      // line of first command
	// The list of commands to execute.
	commands []string

	// The STDOUT and STDERR (merged).
	gotOutput  []string // from execution
	wantOutput []string // from file
}

func ReadTestFile(filename string) (*TestFile, error) {
	// parse states
	const (
		beforeFirstCommand = iota
		inCommands
		inOutput
	)

	tf := &TestFile{
		filename: filename,
		Commands: map[string]func([]string) ([]byte, error){
			"cat":    catCmd,
			"cd":     cdCmd,
			"echo":   echoCmd,
			"echof":  echofCmd,
			"mkdir":  mkdirCmd,
			"setenv": setenvCmd,
		},
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var tc *TestCase
	lineno := 0
	var prefix []string
	state := beforeFirstCommand
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		isCommand := strings.HasPrefix(line, "$")
		switch state {
		case beforeFirstCommand:
			if isCommand {
				tc = &TestCase{startLine: lineno, before: prefix}
				tc.addCommandLine(line)
				state = inCommands
			} else {
				line = strings.TrimSpace(line)
				if line == "" || line[0] == '#' {
					prefix = append(prefix, line)
				} else {
					return nil, fmt.Errorf("%s:%d: bad line %q (should begin with '#')", filename, lineno, line)
				}
			}

		case inCommands:
			if isCommand {
				tc.addCommandLine(line)
			} else { // End of commands marks the start of the output.
				tc.wantOutput = append(tc.wantOutput, line)
				state = inOutput
			}

		case inOutput:
			if isCommand { // A command marks the end of the output.
				prefix = tf.addCase(tc)
				tc = &TestCase{startLine: lineno, before: prefix}
				tc.addCommandLine(line)
				state = inCommands
			} else {
				tc.wantOutput = append(tc.wantOutput, line)
			}
		default:
			panic("bad state")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if tc != nil {
		tf.suffix = tf.addCase(tc)
	}
	return tf, nil
}

func (tc *TestCase) addCommandLine(line string) {
	tc.commands = append(tc.commands, strings.TrimSpace(line[1:]))
}

func (tf *TestFile) addCase(tc *TestCase) []string {
	// Trim the suffix of output that consists solely of blank lines and comments,
	// and return it.
	var i int
	for i = len(tc.wantOutput) - 1; i >= 0; i-- {
		if tc.wantOutput[i] != "" && tc.wantOutput[i][0] != '#' {
			break
		}
	}
	i++
	// i is the index of the first line to ignore.
	keep, suffix := tc.wantOutput[:i], tc.wantOutput[i:]
	if len(keep) == 0 {
		keep = nil
	}
	tc.wantOutput = keep
	tf.cases = append(tf.cases, tc)
	return suffix
}

func (tf *TestFile) Compare() string {
	if err := tf.run(); err != nil {
		return err.Error()
	}
	buf := &bytes.Buffer{}
	for _, c := range tf.cases {
		if diff := cmp.Diff(c.gotOutput, c.wantOutput); diff != "" {
			fmt.Fprintf(buf, "%s:%d: got=-, want=+\n", tf.filename, c.startLine)
			c.writeCommands(buf)
			fmt.Fprintf(buf, "%s\n", diff)
		}
	}
	s := buf.String()
	if len(s) > 0 {
		s = "\n" + s
	}
	return s
}

func (tf *TestFile) Update() error {
	tmpfilename, err := tf.updateToTemp()
	if err != nil {
		os.Remove(tmpfilename)
		return err
	}
	return os.Rename(tmpfilename, tf.filename)
}

func (tf *TestFile) updateToTemp() (fname string, err error) {
	if err := tf.run(); err != nil {
		return "", err
	}

	f, err := ioutil.TempFile("", "cmdtest")
	if err != nil {
		return "", err
	}
	w := bufio.NewWriter(f)
	defer func() {
		err2 := w.Flush()
		if err == nil {
			err = err2
		}
		err2 = f.Close()
		if err == nil {
			err = err2
		}
	}()
	if err := tf.write(w); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func (tf *TestFile) run() error {
	rootDir, err := ioutil.TempDir("", "cmdtest")
	if err != nil {
		return fmt.Errorf("%s: %v", tf.filename, err)
	}
	if tf.KeepRootDir {
		fmt.Printf("test root directory: %s\n", rootDir)
	} else {
		defer os.RemoveAll(rootDir)
	}

	if err := os.Setenv("ROOTDIR", rootDir); err != nil {
		return err
	}
	defer os.Unsetenv("ROOTDIR")
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if err := os.Chdir(rootDir); err != nil {
		return fmt.Errorf("%s: %v", tf.filename, err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	if tf.Setup != nil {
		if err := tf.Setup(rootDir); err != nil {
			return fmt.Errorf("%s: calling Setup: %v", tf.filename, err)
		}
	}

	for _, tc := range tf.cases {
		if err := tc.run(tf, rootDir, tf.Verbose); err != nil {
			return fmt.Errorf("%s:%v", tf.filename, err) // no space after :, for line number
		}

	}
	return nil
}

// A fatal error stops a test.
type fatal struct{ error }

// Run the test case by executing the commands. The concatenated output from all commands
// is saved in tc.gotOutput.
// An error is returned if: a command that should succeed instead failed; a command that should
// fail instead succeeded; or if a built-in command was called incorrectly.
func (tc *TestCase) run(tf *TestFile, rootDir string, verbose bool) error {
	const failMarker = " --> FAIL"

	tc.gotOutput = nil
	var allout []byte
	var err error
	for i, cmd := range tc.commands {
		wantFail := false
		if strings.HasSuffix(cmd, failMarker) {
			cmd = strings.TrimSuffix(cmd, failMarker)
			wantFail = true
		}
		args := strings.Fields(cmd)
		for i := range args {
			args[i], err = expandVariables(args[i], os.LookupEnv)
			if err != nil {
				return err
			}
		}
		if verbose {
			fmt.Printf("$ %s\n", strings.Join(args, " "))
		}
		name := args[0]
		args = args[1:]
		var infile string
		if len(args) >= 2 && args[len(args)-2] == "<" {
			infile = args[len(args)-1]
			args = args[:len(args)-2]
		}
		f := tf.Commands[name]
		var (
			out []byte
			err error
		)
		if f != nil {
			if infile != "" {
				return fmt.Errorf("%d: command %q does not support input redirection", tc.startLine+i, cmd)
			}
			out, err = f(args)
		} else {
			out, err = execute(name, args, infile)
		}
		if _, ok := err.(fatal); ok {
			return fmt.Errorf("%d: command %q failed fatally with %v", tc.startLine+i, cmd, err)
		}
		if err == nil && wantFail {
			return fmt.Errorf("%d: %q succeeded, but it was expected to fail", tc.startLine+i, cmd)
		}
		if err != nil && !wantFail {
			return fmt.Errorf("%d: %q failed with %v. Output:\n%s", tc.startLine+i, cmd, err, out)
		}
		if verbose {
			fmt.Println(string(out))
		}
		allout = append(allout, out...)
	}
	if len(allout) > 0 {
		allout = scrub(rootDir, allout)
		// Remove final whitespace.
		s := strings.TrimRight(string(allout), " \t\n")
		tc.gotOutput = strings.Split(s, "\n")
	}
	return nil
}

func execute(name string, args []string, infile string) ([]byte, error) {
	ecmd := exec.Command(name, args...)
	var errc chan error
	if infile != "" {
		f, err := os.Open(infile)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		inpipe, err := ecmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		errc = make(chan error, 1)
		go func() {
			defer inpipe.Close()
			_, err := io.Copy(inpipe, f)
			errc <- err
		}()
	}
	out, err := ecmd.CombinedOutput()
	if err != nil {
		return out, err
	}
	if errc != nil {
		if err = <-errc; err != nil {
			return out, err
		}
	}
	return out, nil
}

var varRegexp = regexp.MustCompile(`\$\{([^${}]+)\}`)

// expandVariables replaces variable references in s with their values. A reference
// to a variable V looks like "${V}".
// lookup is called on a variable's name to find its value. Its second return value
// is false if the variable doesn't exist.
// expandVariables fails if s contains a reference to a non-existent variable.
func expandVariables(s string, lookup func(string) (string, bool)) (string, error) {
	var sb strings.Builder
	for {
		ixs := varRegexp.FindStringSubmatchIndex(s)
		if ixs == nil {
			sb.WriteString(s)
			return sb.String(), nil
		}
		varName := s[ixs[2]:ixs[3]]
		varVal, ok := lookup(varName)
		if !ok {
			return "", fmt.Errorf("variable %q not found", varName)
		}
		sb.WriteString(s[:ixs[0]])
		sb.WriteString(varVal)
		s = s[ixs[1]:]
	}
}

// scrub removes dynamic content from output.
func scrub(rootDir string, b []byte) []byte {
	const scrubbedRootDir = "${ROOTDIR}"
	rootDirWithSeparator := rootDir + string(filepath.Separator)
	scrubbedRootDirWithSeparator := scrubbedRootDir + "/"
	b = bytes.Replace(b, []byte(rootDirWithSeparator), []byte(scrubbedRootDirWithSeparator), -1)
	b = bytes.Replace(b, []byte(rootDir), []byte(scrubbedRootDir), -1)
	return b
}

func (tf *TestFile) write(w io.Writer) error {
	for _, c := range tf.cases {
		if err := c.write(w); err != nil {
			return err
		}
	}
	return writeLines(w, tf.suffix)
}

func (tc *TestCase) write(w io.Writer) error {
	if err := writeLines(w, tc.before); err != nil {
		return err
	}
	if err := tc.writeCommands(w); err != nil {
		return err
	}
	out := tc.gotOutput
	if out == nil {
		out = tc.wantOutput
	}
	return writeLines(w, out)
}

func (tc *TestCase) writeCommands(w io.Writer) error {
	for _, c := range tc.commands {
		if _, err := fmt.Fprintf(w, "$ %s\n", c); err != nil {
			return err
		}
	}
	return nil
}

func writeLines(w io.Writer, lines []string) error {
	for _, l := range lines {
		if _, err := fmt.Fprintf(w, "%s\n", l); err != nil {
			return err
		}
	}
	return nil
}

// cd DIR
// change directory
func cdCmd(args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, fatal{errors.New("need exactly 1 argument")}
	}
	if err := checkPath(args[0]); err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return nil, os.Chdir(filepath.Join(cwd, args[0]))
}

// echo ARG1 ARG2 ...
// write args to stdout
func echoCmd(args []string) ([]byte, error) {
	return []byte(strings.Join(args, " ") + "\n"), nil
}

// echof FILE ARG1 ARG2 ...
// write args to FILE
func echofCmd(args []string) ([]byte, error) {
	if len(args) < 1 {
		return nil, fatal{errors.New("need at least 1 argument")}
	}
	if err := checkPath(args[0]); err != nil {
		return nil, err
	}
	return nil, ioutil.WriteFile(args[0], []byte(strings.Join(args[1:], " ")+"\n"), 0600)
}

// cat FILE
// copy file to stdout
func catCmd(args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, fatal{errors.New("need exactly 1 argument")}
	}
	if err := checkPath(args[0]); err != nil {
		return nil, err
	}
	f, err := os.Open(args[0])
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := &bytes.Buffer{}
	_, err = io.Copy(buf, f)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// mkdir DIR
// create directory
func mkdirCmd(args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, fatal{errors.New("need exactly 1 argument")}
	}
	if err := checkPath(args[0]); err != nil {
		return nil, err
	}
	return nil, os.Mkdir(args[0], 0700)
}

// setenv VAR VALUE
// set environment variable
func setenvCmd(args []string) ([]byte, error) {
	if len(args) != 2 {
		return nil, fatal{errors.New("need exactly 2 arguments")}
	}
	return nil, os.Setenv(args[0], args[1])
}

func checkPath(path string) error {
	if strings.ContainsRune(path, '/') {
		return fatal{fmt.Errorf("argument must be in the current directory (%q has a '/')", path)}
	}
	return nil
}
