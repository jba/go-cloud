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

package mongodocstore // import "gocloud.dev/internal/docstore/mongodocstore"

import (
	"context"
	"errors"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
)

type collection struct {
	coll *mgo.Collection
}

func OpenCollection(db *mgo.Database, name string) *docstore.Collection {
	return docstore.NewCollection(newCollection(db, name))
}

func newCollection(db *mgo.Database, name string) *collection {
	return &collection{coll: db.C(name)}
}

func (c *collection) KeyFields() []string {
	return []string{"_id"}
}

const id = "_id"

func (c *collection) RunActions(ctx context.Context, actions []driver.Action) (int, error) {
	for i, a := range actions {
		var mdoc map[string]interface{}
		var err error
		switch a.Kind {
		case driver.Create, driver.Put:
			if _, ok := a.Doc[id]; !ok {
				a.Doc[id] = bson.NewObjectId()
			}
			fallthrough

		case driver.Replace:
			mdoc = convertDocument(a.Doc)
			if err != nil {
				return 0, err
			}
		}

		switch a.Kind {
		case driver.Create:
			err = c.coll.Insert(mdoc)
		case driver.Put:
			// TODO: check that mdoc has no update operators
			_, err = c.coll.UpsertId(a.Doc[id], mdoc)
		case driver.Replace:
			// TODO: check that mdoc has no update operators
			err = c.coll.UpdateId(a.Doc[id], mdoc)
		case driver.Delete:
			err = c.coll.RemoveId(a.Doc[id])

		case driver.Get:
			q := c.coll.FindId(a.Doc[id])
			if len(a.Doc) > 1 { // fields other than ID
				// See https://docs.mongodb.com/manual/tutorial/query-documents/#projection
				p := bson.M{}
				for k := range a.Doc {
					p[k] = 1
				}
				q.Select(p)
			}
			err = q.One(a.Doc)

		case driver.Update:
			err = c.update(a.Doc, a.Mods)
		default:
			panic("bad kind")
		}
		if err != nil {
			return i, err
		}
	}
	return len(actions), nil
}

func (c *collection) update(doc driver.Document, mods driver.Mods) error {
	const (
		set   = "$set"
		unset = "$unset"
	)
	mdoc := bson.M{}

	// TODO: nested maps. Use dot notation; see e.g.
	// https://docs.mongodb.com/manual/reference/operator/update/unset/#up._S_unset
	add := func(key, field string, value interface{}) {
		m, ok := mdoc[key]
		if !ok {
			m = bson.M{}
			mdoc[key] = m
		}
		m.(bson.M)[field] = value
	}
	for k, v := range mods {
		if v == nil {
			add(unset, k, "")
		} else {
			add(set, k, v)
		}
	}
	return c.coll.UpdateId(doc[id], mdoc)
}

func convertDocument(doc driver.Document) map[string]interface{} {
	return doc
}

func (c *collection) RunQuery(context.Context, *driver.Query) error {
	return errors.New("unimp")
}
