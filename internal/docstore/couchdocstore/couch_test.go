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

package couchdocstore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
)

// Start a local couchdb instance with
//   docker run -d -p 5984:5984 apache/couchdb

type docmap = map[string]interface{}

func Test(t *testing.T) {
	ctx := context.Background()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	dc, err := newCollection("http://localhost:5984", "baseball", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	coll := docstore.NewCollection(dc)

	must(coll.Put(ctx, docmap{"_id": "abc", "team": "Mets"}))
	doc := docmap{"_id": "abc"}
	must(coll.Get(ctx, doc))
	if doc["team"] != "Mets" {
		t.Fatalf("got %v", doc)
	}

	d2 := docmap{"_id": "abc"}
	must(coll.Get(ctx, d2))
	fmt.Println(d2)

	must(coll.Delete(ctx, d2))
	if err := coll.Get(ctx, d2); err == nil {
		t.Fatal("got nil, want error")
	}
	d3 := docmap{"_id": fmt.Sprintf("new%d", time.Now().Unix())}
	must(coll.Put(ctx, d3))

	d4 := docmap{
		"_id": fmt.Sprintf("create%d", time.Now().Unix()),
		"a":   1,
		"b":   docmap{"c": 2},
		"d":   3,
	}
	must(coll.Create(ctx, d4))

	must(coll.Update(ctx, d4, docstore.Mods{"a": 2, "b.c": -9, "b.g.q": 12, "d": nil, "e": 99}))
	d4u := docmap{"_id": d4["_id"]}
	must(coll.Get(ctx, d4u))
	want := docmap{
		"_id":              d4u["_id"],
		"DocstoreRevision": d4u["DocstoreRevision"],
		"a":                int64(2),
		"b":                docmap{"c": int64(-9), "g": docmap{"q": int64(12)}},
		"e":                int64(99),
	}
	if diff := cmp.Diff(d4u, want); diff != "" {
		t.Error(diff)
	}
}

func TestModifyDoc(t *testing.T) {
	for _, test := range []struct {
		mod  driver.Mod
		want docmap
	}{
		{
			mod:  driver.Mod{[]string{"a"}, -1},
			want: docmap{"a": -1, "b": 2, "c": docmap{"d": 3}},
		},
		{
			mod:  driver.Mod{[]string{"b"}, nil},
			want: docmap{"a": 1, "c": docmap{"d": 3}},
		},
		{
			mod:  driver.Mod{[]string{"c", "d"}, 17},
			want: docmap{"a": 1, "b": 2, "c": docmap{"d": 17}},
		},
		{
			mod:  driver.Mod{[]string{"c", "d"}, nil},
			want: docmap{"a": 1, "b": 2, "c": docmap{}},
		},
		{
			mod:  driver.Mod{[]string{"c", "e"}, 20},
			want: docmap{"a": 1, "b": 2, "c": docmap{"d": 3, "e": 20}},
		},
	} {
		orig := docmap{"a": 1, "b": 2, "c": docmap{"d": 3}}
		doc, err := driver.NewDocument(orig)
		if err != nil {
			t.Fatal(err)
		}
		if err := modifyDoc(doc, test.mod); err != nil {
			t.Fatal(err)
		}
		got := doc.Origin.(docmap)
		if !cmp.Equal(got, test.want) {
			t.Errorf("%v:\ngot  %v\nwant %v", test.mod, got, test.want)
		}
	}

	// It's an error to set a sub-field of a non-map field.
	doc, err := driver.NewDocument(docmap{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	err = modifyDoc(doc, driver.Mod{[]string{"a", "b"}, 2})
	if err == nil {
		t.Error("got nil, want error")
	}
}

func TestCouchDBBehavior(t *testing.T) {
	ctx := context.Background()
	client := http.DefaultClient
	urlPrefix := "http://localhost:5984/put-test"
	_, err := callCouch(ctx, client, "DELETE", urlPrefix, nil)
	_, err = callCouch(ctx, client, "PUT", urlPrefix, nil)
	if err != nil {
		t.Fatal(err)
	}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	cannot := func(err error) {
		t.Helper()
		if err == nil {
			t.Fatal("got nil, want error")
		}
	}

	put := func(id string, m docmap) error {
		t.Helper()
		bytes, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		_, err = callCouch(ctx, client, "PUT", urlPrefix+"/"+id, bytes)
		return err
	}

	del := func(id string, rev interface{}) error {
		t.Helper()
		url := fmt.Sprintf("%s/%s?rev=%v", urlPrefix, id, rev)
		_, err := callCouch(ctx, client, "DELETE", url, nil)
		return err
	}

	get := func(id string) docmap {
		bytes, err := callCouch(ctx, client, "GET", urlPrefix+"/"+id, nil)
		if err != nil {
			t.Fatal(err)
		}
		var got docmap
		if err := json.Unmarshal(bytes, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	// You can put a new document without a rev.
	must(put("p1", docmap{"a": 1}))
	initialRev := get("p1")["_rev"]
	// You can't put an existing document without a rev.
	cannot(put("p1", docmap{"a": 2}))
	// It's an error to use the wrong rev.
	cannot(put("p1", docmap{"a": 3, "_rev": "whatever"}))
	// It works with the right rev.
	got := get("p1")
	must(put("p1", docmap{"a": 4, "_rev": got["_rev"]}))

	// You can put a new document even if it was just deleted.
	must(del("p1", get("p1")["_rev"]))
	must(put("p1", docmap{"a": 5}))

	// You cannot put a new document with a rev.
	cannot(put("p2", docmap{"a": 1, "_rev": initialRev}))
}
