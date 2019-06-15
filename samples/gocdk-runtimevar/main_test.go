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

package main_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"gocloud.dev/internal/testing/cmdtest"
)

func Test(t *testing.T) {
	if err := exec.Command("go", "build").Run(); err != nil {
		t.Fatal(err)
	}
	tf, err := cmdtest.ReadTestFile("runtimevar.ct")
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("PATH", cwd)

	type result struct {
		out []byte
		err error
	}

	var (
		cancel func()
		outc   chan result
	)

	// "go" starts a command in a separate goroutine.
	tf.Commands["go"] = func(args []string) ([]byte, error) {
		if len(args) == 0 {
			return nil, errors.New("need at least one arg")
		}
		if cancel != nil {
			return nil, errors.New("go already in progress")
		}

		outc = make(chan result, 1)
		ctx := context.Background()
		ctx, cancel = context.WithCancel(context.Background())
		go func() {
			out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
			outc <- result{out, err}
		}()
		return nil, nil
	}

	// "stop" stops a command started with "go" and returns its output and error.
	tf.Commands["stop"] = func(args []string) ([]byte, error) {
		if cancel == nil {
			return nil, errors.New("no 'go' in progress")
		}
		cancel()
		res := <-outc
		if res.err == context.Canceled {
			res.err = nil
		}
		if ee, ok := res.err.(*exec.ExitError); ok && ee.ExitCode() == -1 { // killed by signal
			res.err = nil
		}
		return res.out, res.err
	}

	tf.Commands["sleep"] = func(args []string) ([]byte, error) {
		if len(args) != 1 {
			return nil, errors.New("need exactly 1 argument")
		}
		secs, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, err
		}
		time.Sleep(time.Duration(secs) * time.Second)
		return nil, nil
	}
	if diff := tf.Compare(); diff != "" {
		t.Error(diff)
	}
}
