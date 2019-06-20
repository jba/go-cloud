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

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gocloud.dev/internal/testing/cmdtest"
)

var record = flag.Bool("record", false, "true to record the desired output for record/replay testcases")

func TestProcessContextModuleRoot(t *testing.T) {
	ctx := context.Background()
	t.Run("SameDirAsModule", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()

		got, err := pctx.ModuleRoot(ctx)
		if got != pctx.workdir || err != nil {
			t.Errorf("got %q/%v want %q/<nil>", got, err, pctx.workdir)
		}
	})
	t.Run("NoBiomesDir", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		if err := os.RemoveAll(biomesRootDir(pctx.workdir)); err != nil {
			t.Fatal(err)
		}

		if _, err = pctx.ModuleRoot(ctx); err == nil {
			t.Errorf("got nil error, want non-nil error due to missing biomes/ dir")
		}
	})
	t.Run("NoModFile", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		if err := os.Remove(filepath.Join(pctx.workdir, "go.mod")); err != nil {
			t.Fatal(err)
		}

		if _, err = pctx.ModuleRoot(ctx); err == nil {
			t.Errorf("got nil error, want non-nil error due to missing go.mod")
		}
	})
	t.Run("ParentDirectory", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()

		rootdir := pctx.workdir
		subdir := filepath.Join(rootdir, "subdir")
		if err := os.Mkdir(subdir, 0777); err != nil {
			t.Fatal(err)
		}
		pctx.workdir = subdir

		got, err := pctx.ModuleRoot(ctx)
		if got != rootdir || err != nil {
			t.Errorf("got %q/%v want %q/<nil>", got, err, rootdir)
		}
	})
}

// This test uses the cmdtest package for testing CLI programs.
// Each .ct file in testdata contains a series of commands and their
// output. This test runs the command and checks that the output matches.
func TestCLI(t *testing.T) {
	if err := exec.Command("go", "build").Run(); err != nil {
		t.Fatal(err)
	}
	testFilenames, err := filepath.Glob("testdata/*.ct")
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	gorootOut, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	goroot := strings.TrimSpace(string(gorootOut))
	os.Setenv("PATH", fmt.Sprintf("%s%c%s/bin", cwd, os.PathListSeparator, goroot))
	for _, fn := range testFilenames {
		testName := strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))
		t.Run(testName, func(t *testing.T) {
			tf, err := cmdtest.ReadTestFile(fn)
			if err != nil {
				t.Fatal(err)
			}
			// Set GOPATH to the parent of the test's root directory. That lets gocdk
			// determine a module path, but doesn't clutter the root directory with
			// the module cache (pkg/mod).
			tf.Setup = func(rootDir string) error {
				return os.Setenv("GOPATH", filepath.Dir(rootDir))
			}
			tf.Commands["ls"] = func(args []string) ([]byte, error) {
				var arg string
				if len(args) == 1 {
					arg = args[0]
					if strings.Contains(arg, "/") {
						return nil, fmt.Errorf("argument must be in the current directory (%q has a '/')", arg)
					}
				} else if len(args) > 1 {
					return nil, fmt.Errorf("takes 0-1 arguments (got %d)", len(args))
				}
				out := &bytes.Buffer{}
				cwd, err := os.Getwd()
				if err != nil {
					return nil, err
				}
				if err := doList(cwd, out, arg, ""); err != nil {
					return nil, err
				}
				return out.Bytes(), nil
			}
			if diff := tf.Compare(); diff != "" {
				t.Error(diff)
			}
		})
	}
}

// 	testcases := make([]*testCase, 0, len(testcaseDirs))
// 	for _, dir := range testcaseDirs {
// 		testcase, err := newTestCase(filepath.Join(testRoot, dir.Name()), *record)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		testcases = append(testcases, testcase)
// 	}

// 	ctx := context.Background()
// 	for _, tc := range testcases {
// 		tc := tc
// 		t.Run(filepath.Base(tc.dir), func(t *testing.T) {
// 			t.Parallel()

// 			rootDir, err := ioutil.TempDir("", testTempDirPrefix)
// 			if err != nil {
// 				t.Fatal(err)
// 			}
// 			defer os.RemoveAll(rootDir)

// 			var out bytes.Buffer
// 			curDir := rootDir
// 			pctx := newProcessContext(curDir, strings.NewReader(""), &out, &out)
// 			pctx.env = append(pctx.env, "GOPATH="+rootDir)
// 			for i, command := range tc.commands {
// 				fmt.Fprintln(&out, "$", command)
// 				args := strings.Split(command, " ")
// 				cmd := args[0]
// 				args = args[1:]

// 				// We can add support for additional commands as needed; see
// 				// https://github.com/golang/go/blob/master/src/cmd/go/testdata/script/README
// 				// for inspiration.
// 				var cmdErr error
// 				switch cmd {
// 				case "gocdk":
// 					cmdErr = run(ctx, pctx, args)
// 				case "cd":
// 					if len(args) != 1 {
// 						t.Fatalf("command #%d: cd takes exactly 1 argument", i)
// 					}
// 					if strings.Contains(args[0], "/") {
// 						t.Fatalf("command #%d: argument to cd must be in the current directory (%q has a '/')", i, args[0])
// 					}
// 					curDir = filepath.Join(curDir, args[0])
// 					pctx.workdir = curDir
// 				case "ls":
// 					var arg string
// 					if len(args) == 1 {
// 						arg = args[0]
// 						if strings.Contains(arg, "/") {
// 							t.Fatalf("command #%d: argument to ls must be in the current directory (%q has a '/')", i, arg)
// 						}
// 					} else if len(args) > 1 {
// 						t.Fatalf("command #%d: ls takes 0-1 arguments (got %d)", i, len(args))
// 					}
// 					cmdErr = doList(curDir, &out, arg, "")
// 				default:
// 					t.Fatalf("unknown command #%d (%q)", i, command)
// 				}
// 				if cmdErr == nil && tc.wantFail[i] {
// 					t.Fatalf("command #%d (%q) succeeded, but it was expected to fail", i, command)
// 				}
// 				if cmdErr != nil && !tc.wantFail[i] {
// 					t.Fatalf("command #%d (%q) failed with %v", i, command, cmdErr)
// 				}
// 				fmt.Fprintln(&out)
// 			}

// 			got := scrub(rootDir, out.Bytes())
// 			if *record {
// 				if err := ioutil.WriteFile(filepath.Join(tc.dir, "out.txt"), got, 0666); err != nil {
// 					t.Fatalf("failed to record out.txt to testdata: %v", err)
// 				}
// 			} else {
// 				// Split to string lines to make diff output cleaner.
// 				gotLines := strings.Split(string(got), "\n")
// 				wantLines := strings.Split(string(tc.want), "\n")
// 				if diff := cmp.Diff(wantLines, gotLines); diff != "" {
// 					t.Errorf("out mismatch:\n%s", diff)
// 				}
// 			}
// 		})
// 	}
// }

// doList recursively lists the files/directory in dir to out.
// If arg is not empty, it only lists arg.
func doList(dir string, out io.Writer, arg, indent string) error {
	fileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	// Sort case-insensitive. ioutil.ReadDir returns results in sorted order,
	// but it's not stable across platforms.
	sort.Slice(fileInfos, func(i, j int) bool {
		return strings.ToLower(fileInfos[i].Name()) < strings.ToLower(fileInfos[j].Name())
	})

	for _, fi := range fileInfos {
		if arg != "" && arg != fi.Name() {
			continue
		}
		if fi.IsDir() {
			fmt.Fprintf(out, "%s%s/\n", indent, fi.Name())
			if err := doList(filepath.Join(dir, fi.Name()), out, "", indent+"  "); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(out, "%s%s\n", indent, fi.Name())
		}
	}
	return nil
}
