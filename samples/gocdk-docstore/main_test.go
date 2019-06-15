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
	"os"
	"os/exec"
	"testing"

	"gocloud.dev/internal/testing/cmdtest"
)

// Requires mongo to be running. Run docstore/mongodocstore/localmongo.sh

func Test(t *testing.T) {
	if err := exec.Command("go", "build").Run(); err != nil {
		t.Fatal(err)
	}
	tf, err := cmdtest.ReadTestFile("docstore.ct")
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("PATH", cwd)
	os.Setenv("MONGO_SERVER_URL", "mongodb://localhost")
	if diff := tf.Compare(); diff != "" {
		t.Error(diff)
	}
}
