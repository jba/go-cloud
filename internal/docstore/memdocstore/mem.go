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

// Package memdocstore provides an in-memory implementation of the docstore
// API. It is suitable for local development and testing.
package memdocstore // import "gocloud.dev/internal/docstore/memdocstore"

import (
	"context" // Options sets options for constructing a *docstore.Collection backed by memory.

	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

type Options struct{}

// OpenCollection creates a *docstore.Collection backed by memory.
func OpenCollection(keyField string, opts *Options) *docstore.Collection {
	return docstore.NewCollection(newCollection(keyField))
}

func newCollection(keyField string) driver.Collection {
	return &collection{
		keyField: keyField,
		docs:     map[interface{}]driver.Document{},
	}
}

type collection struct {
	keyField string
	docs     map[interface{}]driver.Document
}

// RunActions implements driver.RunActions.
func (c *collection) RunActions(ctx context.Context, actions []*driver.Action) (int, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	for i, a := range actions {
		if err := c.runAction(a); err != nil {
			return i, err
		}
	}
	return len(actions), nil
}

func (c *collection) runAction(a *driver.Action) error {
	// Get the key from the doc so we can look it up in the map.
	key, err := a.Doc.GetField(c.keyField)
	// The only acceptable error case is NotFound during a Create.
	if err != nil && !(gcerrors.Code(err) == gcerr.NotFound && a.Kind == driver.Create) {
		return err
	}
	// If there is a key, get the current document.
	var (
		current driver.Document
		exists  bool
	)
	if err == nil {
		current, exists = c.docs[key]
	}

	if !exists && (a.Kind == driver.Replace || a.Kind == driver.Update || a.Kind == driver.Get) {
		return gcerr.Newf(gcerr.NotFound, nil, "document with key %v does not exist", key)
	}
	switch a.Kind {
	case driver.Create:
		if exists {
			return gcerr.Newf(gcerr.AlreadyExists, nil, "Create: document with key %v exists", key)
		}
		if key == nil {
			key = driver.UniqueString()
			a.Doc.SetField(c.keyField, key)
		}
		fallthrough

	case driver.Replace, driver.Put:
		c.docs[key] = a.Doc

	case driver.Delete:
		delete(c.docs, key)

	case driver.Update:
		if err := c.update(current, a.Mods); err != nil {
			return err
		}

	case driver.Get:
		if err := copyFields(a.Doc, current, a.FieldPaths); err != nil {
			return err
		}
	default:
		return gcerr.Newf(gcerr.Internal, nil, "unknown kind %v", a.Kind)
	}
	return nil
}

func (c *collection) update(doc driver.Document, mods []driver.Mod) error {
	for _, m := range mods {
		if m.Value == nil {
			deleteAtFieldPath(doc.Map(), m.FieldPath)
		} else if err := doc.Set(m.FieldPath, m.Value); err != nil {
			return err
		}
	}
	return nil
}

func deleteAtFieldPath(m map[string]interface{}, fp []string) {
	if len(fp) == 1 {
		delete(m, fp[0])
	} else if m2, ok := m[fp[0]].(map[string]interface{}); ok {
		deleteAtFieldPath(m2, fp[1:])
	}
	// Otherwise do nothing.
}

func copyFields(dest, src driver.Document, fps [][]string) error {
	if fps == nil {
		for k, v := range src.Map() {
			if err := dest.SetField(k, v); err != nil {
				return err
			}
		}
	} else {
		for _, fp := range fps {
			val, err := src.Get(fp)
			if err != nil {
				return err
			}
			dest.Set(fp, val)
		}
	}
	return nil
}

// ErrorCode implements driver.ErrorCOde.
func (c *collection) ErrorCode(err error) gcerr.ErrorCode {
	return gcerrors.Code(err)
}

func (*collection) RunQuery(context.Context, *driver.Query) error {
	return nil
}
