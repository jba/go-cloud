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

package docstore_test

import (
	"context"
	"io"
	"time"

	"gocloud.dev/internal/docstore"
)

func ExampleActions() {
	ctx := context.Background()

	type Player struct {
		Name  string // Name is the key
		Score int
	}

	fred := &Player{Name: "Fred", Score: 18}
	got := &Player{Name: "Fred"}
	coll := docstore.NewCollection(nil)
	_, err := coll.Actions().
		Put(fred).
		Get(got).
		Do(ctx)
	_ = err
}

func ExampleQuery() {
	ctx := context.Background()
	coll := docstore.NewCollection(nil)
	iter := coll.Query().
		Where("name", ">", "Alice").
		Where("name", "<=", "George").
		Limit(10).
		Get(ctx, "name", "score")
	for {
		var doc map[string]interface{}
		err := iter.Next(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			// TODO: handle error
		}
		_ = doc // TODO: use doc
	}
}

func ExampleQueryDelete() {
	ctx := context.Background()
	coll := docstore.NewCollection(nil)
	err := coll.Query().
		Where("lastUpdateTime", "<", time.Now().Add(-24*time.Hour)).
		Delete(ctx)
	_ = err // TODO: handle error
}
