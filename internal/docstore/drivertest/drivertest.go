// Copyright 2018 The Go Cloud Authors
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

// Package drivertest provides a conformance test for implementations of
// driver.
package drivertest // import "gocloud.dev/internal/docstore/drivertest"

import (
	"context"
	//	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	ds "gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
)

// Harness descibes the functionality test harnesses must provide to run
// conformance tests.
type Harness interface {
	// MakeCollection makes a driver.Collection for testing.
	MakeCollection(context.Context) (driver.Collection, error)

	// Close closes resources used by the harness.
	Close()
}

// HarnessMaker describes functions that construct a harness for running tests.
// It is called exactly once per test; Harness.Close() will be called when the test is complete.
type HarnessMaker func(ctx context.Context, t *testing.T) (Harness, error)

// RunConformanceTests runs conformance tests for provider implementations of docstore.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker) {
	t.Run("Create", func(t *testing.T) { withCollection(t, newHarness, testCreate) })
	t.Run("Put", func(t *testing.T) { withCollection(t, newHarness, testPut) })
	t.Run("Replace", func(t *testing.T) { withCollection(t, newHarness, testReplace) })
	t.Run("Get", func(t *testing.T) { withCollection(t, newHarness, testGet) })
	t.Run("Delete", func(t *testing.T) { withCollection(t, newHarness, testDelete) })
	t.Run("Update", func(t *testing.T) { withCollection(t, newHarness, testUpdate) })
	t.Run("Data", func(t *testing.T) { withCollection(t, newHarness, testData) })
}

const keyField = "_id"

func withCollection(t *testing.T, newHarness HarnessMaker, f func(*testing.T, *ds.Collection)) {
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	dc, err := h.MakeCollection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	coll := ds.NewCollection(dc)
	f(t, coll)
}

var nonexistentDoc = ds.Document{keyField: "doesNotExist"}

func testCreate(t *testing.T, coll *ds.Collection) {
	named := ds.Document{keyField: "testCreate1", "b": true}
	unnamed := ds.Document{"b": false}
	// Attempt to clean up
	defer func() {
		_, _ = coll.Actions().Delete(named).Delete(unnamed).Do(context.Background())
	}()

	mustRunAndCompare(t, coll, named, coll.Actions().Create(named))
	mustRunAndCompare(t, coll, unnamed, coll.Actions().Create(unnamed))
	// Can't create an existing doc.
	shouldFail(t, coll.Actions().Create(named))
}

func testPut(t *testing.T, coll *ds.Collection) {
	named := ds.Document{keyField: "testPut1", "b": true}
	mustRunAndCompare(t, coll, named, coll.Actions().Put(named)) // create new
	named["b"] = false
	mustRunAndCompare(t, coll, named, coll.Actions().Put(named)) // replace existing
}

func testReplace(t *testing.T, coll *ds.Collection) {
	doc1 := ds.Document{keyField: "testReplace", "s": "a"}
	mustRun(t, coll.Actions().Put(doc1))

	doc1["s"] = "b"
	mustRunAndCompare(t, coll, doc1, coll.Actions().Replace(doc1))

	// Can't replace a nonexistent doc.
	shouldFail(t, coll.Actions().Replace(nonexistentDoc))
}

func testGet(t *testing.T, coll *ds.Collection) {
	doc := ds.Document{
		keyField: "testGet1",
		"s":      "a string",
		"i":      int64(95),
		"f":      32.3,
	}
	mustRun(t, coll.Actions().Put(doc))

	checkGet := func(got, want ds.Document) {
		t.Helper()
		mustRun(t, coll.Actions().Get(got))
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("got=-, want=+: %s", diff)
		}
	}

	// If only the key fields are present, the full document is populated.
	checkGet(ds.Document{keyField: doc[keyField]}, doc)

	// If other fields are present, only they are populated.
	// checkGet(
	// 	ds.Document{keyField: doc[keyField], "s": nil},
	// 	ds.Document{keyField: doc[keyField], "s": doc["s"]},
	// )

	// // If fields not in the original doc are present, they may or may not be present in the answer.
	// got := ds.Document{keyField: doc[keyField], "i": nil, "junk": 3}
	// mustRun(t, coll.Actions().Get(got))
	// want := ds.Document{keyField: doc[keyField], "i": doc["i"]}
	// if !cmp.Equal(got, want) {
	// 	want["junk"] = 3
	// 	if !cmp.Equal(got, want) {
	// 		t.Errorf("got  %v\nwant %v\n(or without 'junk')", got, want)
	// 	}
	// }
}

func testDelete(t *testing.T, coll *ds.Collection) {
	doc := ds.Document{keyField: "testDelete"}
	mustRun(t, coll.Actions().Put(doc).Delete(doc))
	shouldFail(t, coll.Actions().Get(doc))
	shouldFail(t, coll.Actions().Delete(nonexistentDoc))
}

func testUpdate(t *testing.T, coll *ds.Collection) {
	doc := ds.Document{keyField: "testUpdate", "a": "A", "b": "B"}
	mustRun(t, coll.Actions().Put(doc))

	got := ds.Document{keyField: doc[keyField]}
	mustRun(t, coll.Actions().Update(doc, ds.Mods{
		"a": "X",
		"b": nil,
		"c": "C",
	}).Get(got))
	want := ds.Document{
		keyField: doc[keyField],
		"a":      "X",
		"c":      "C",
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Can't update a nonexistent doc
	shouldFail(t, coll.Actions().Update(nonexistentDoc, ds.Mods{}))
}

func testData(t *testing.T, coll *ds.Collection) {
	// All Go integer types except uint64 are supported, but they all come back as int64.
	for _, test := range []struct {
		in, want interface{}
	}{
		{int(-1), int64(-1)},
		{int8(-8), int64(-8)},
		{int16(-16), int64(-16)},
		{int32(-32), int64(-32)},
		{int64(-64), int64(-64)},
		//		{uint(1), int64(1)}, TODO: support uint in firestore
		{uint8(8), int64(8)},
		{uint16(16), int64(16)},
		{uint32(32), int64(32)},
		// TODO: support uint64
		{float32(3.5), float64(3.5)},
		{[]byte{0, 1, 2}, []byte{0, 1, 2}},
	} {
		doc := ds.Document{keyField: "testData", "val": test.in}
		got := ds.Document{keyField: doc[keyField]}
		mustRun(t, coll.Actions().Put(doc).Get(got))
		want := ds.Document{keyField: doc[keyField], "val": test.want}
		if len(got) != len(want) {
			t.Errorf("%v: got %v, want %v", test.in, got, want)
		} else if g := got["val"]; !cmp.Equal(g, test.want) {
			t.Errorf("%v: got %v (%T), want %v (%T)", test.in, g, g, test.want, test.want)
		}
	}

	// TODO: strings: valid vs. invalid unicode

}

func mustRun(t *testing.T, al *ds.ActionList) {
	t.Helper()
	if _, err := al.Do(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func mustRunAndCompare(t *testing.T, coll *ds.Collection, doc ds.Document, al *ds.ActionList) {
	t.Helper()
	mustRun(t, al)
	got := ds.Document{keyField: doc[keyField]}
	mustRun(t, coll.Actions().Get(got))
	if diff := cmp.Diff(got, doc); diff != "" {
		t.Fatalf(diff)
	}
}

func shouldFail(t *testing.T, al *ds.ActionList) {
	t.Helper()
	if _, err := al.Do(context.Background()); err == nil {
		t.Error("got nil, want error")
	}
}
