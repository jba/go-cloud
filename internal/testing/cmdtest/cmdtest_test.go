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
	"io/ioutil"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReadTestFile(t *testing.T) {
	got, err := ReadTestFile("testdata/read.ct")
	if err != nil {
		t.Fatal(err)
	}
	got.Commands = nil
	want := &TestFile{
		filename: "testdata/read.ct",
		cases: []*TestCase{
			{
				before: []string{
					"# A sample test file.",
					"",
					"#   Prefix stuff.",
					"",
				},
				startLine:  5,
				commands:   []string{"command arg1 arg2", "cmd2"},
				wantOutput: []string{"out1", "out2"},
			},
			{
				before:     []string{"", "# start of the next case"},
				startLine:  11,
				commands:   []string{"c3"},
				wantOutput: nil,
			},
			{
				before:     []string{"", "# start of the third", ""},
				startLine:  15,
				commands:   []string{"c4 --> FAIL"},
				wantOutput: []string{"out3"},
			},
		},
		suffix: []string{"", "", "# end"},
	}
	if diff := cmp.Diff(got, want, cmp.AllowUnexported(TestFile{}, TestCase{})); diff != "" {
		t.Error(diff)
	}

}

func TestRun(t *testing.T) {
	tf := mustReadTestFile(t, "run-1")
	if err := tf.run(); err != nil {
		t.Fatal(err)
	}
}

func TestCompare(t *testing.T) {
	tf := mustReadTestFile(t, "run-1")
	if diff := tf.Compare(); diff != "" {
		t.Errorf("got\n%s\nwant empty", diff)
	}

	tf = mustReadTestFile(t, "run-2")
	if diff := tf.Compare(); diff == "" {
		t.Error("got no diff, want diff")
	}
}

func TestExpand(t *testing.T) {
	lookup := func(name string) (string, bool) {
		switch name {
		case "A":
			return "1", true
		case "B_C":
			return "234", true
		default:
			return "", false
		}
	}
	for _, test := range []struct {
		in, want string
	}{
		{"", ""},
		{"${A}", "1"},
		{"${A}${B_C}", "1234"},
		{" x${A}y  ${B_C}z ", " x1y  234z "},
		{" ${A${B_C}", " ${A234"},
	} {
		got, err := expand(test.in, lookup)
		if err != nil {
			t.Errorf("%q: %v", test.in, err)
			continue
		}
		if got != test.want {
			t.Errorf("%q: got %q, want %q", test.in, got, test.want)
		}
	}

	// Unknown variable is an error.
	if _, err := expand("x${C}y", lookup); err == nil {
		t.Error("got nil, want error")
	}
}

func TestUpdateToTemp(t *testing.T) {
	tf := mustReadTestFile(t, "run-1")
	fname, err := tf.updateToTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fname)
	want, err := ioutil.ReadFile("testdata/run-1.ct")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ioutil.ReadFile(fname)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(string(got), string(want)); diff != "" {
		t.Error(diff)
	}
}

func mustReadTestFile(t *testing.T, basename string) *TestFile {
	t.Helper()
	tf, err := ReadTestFile("testdata/" + basename + ".ct")
	if err != nil {
		t.Fatal(err)
	}
	return tf
}
