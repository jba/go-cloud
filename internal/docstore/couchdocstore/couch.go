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

// Issues:
// - CouchDB requires a revision to delete or replace. Docstore doesn't insist on one.
// - Any nontrivial query requires a javascript "view" to be written and installed.

package couchdocstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

const (
	couchIDField       = "_id"
	couchRevisionField = "_rev"
)

// Options are optional arguments to the OpenCollection functions.
type Options struct{}

// OpenCollection creates a *docstore.Collection for CouchDB. idField is the
// document field holding the primary key of the collection.
func OpenCollection(url, collName, idField string, _ *Options) (*docstore.Collection, error) {
	c, err := newCollection(url, collName, idField, nil)
	if err != nil {
		return nil, err
	}
	return docstore.NewCollection(c), nil
}

// OpenCollectionWithKeyFunc creates a *docstore.Collection for CouchDB. idFunc takes
// a document and returns the document's primary key. It should return "" if the
// document is missing the information to construct a key. This will cause all
// actions, even Create, to fail.
// func OpenCollectionWithKeyFunc(collName string, idFunc func(docstore.Document) interface{}, _ *Options) (*docstore.Collection, error) {
// 	c, err := newCollection(collName, "", idFunc)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return docstore.NewCollection(c), nil
// }

func newCollection(url, collName, idField string, idFunc func(docstore.Document) string) (driver.Collection, error) {
	if idField == "" && idFunc == nil {
		idField = couchIDField
	}
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	return &collection{
		urlPrefix: url,
		name:      collName,
		idField:   idField,
		idFunc:    idFunc,
		client:    http.DefaultClient,
	}, nil
}

type collection struct {
	urlPrefix string
	name      string
	idField   string
	idFunc    func(docstore.Document) string
	client    *http.Client
}

func (c *collection) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerrors.Code(err)
}

// RunActions implements driver.RunActions.
func (c *collection) RunActions(ctx context.Context, actions []*driver.Action, unordered bool) driver.ActionListError {
	if unordered {
		panic("unordered unimplemented")
	}
	// Run each action in order, stopping at the first error.
	for i, a := range actions {
		if err := c.runAction(ctx, a); err != nil {
			return driver.ActionListError{{i, err}}
		}
	}
	return nil
}

// runAction executes a single action.
func (c *collection) runAction(ctx context.Context, action *driver.Action) error {
	var err error
	switch action.Kind {
	case driver.Get:
		err = c.get(ctx, action)

	case driver.Create:
		err = c.create(ctx, action)

	case driver.Replace, driver.Put:
		err = c.put(ctx, action, action.Kind == driver.Replace)

	case driver.Delete:
		err = c.delete(ctx, action)

	case driver.Update:
		err = c.update(ctx, action)

	default:
		err = gcerr.Newf(gcerr.Internal, nil, "bad action %+v", action)
	}
	return err
}

func (c *collection) get(ctx context.Context, a *driver.Action) error {
	id, err := c.docID(a.Doc, true)
	if err != nil {
		return err
	}
	return c.getAtID(ctx, id, a.Doc)
}

// getInto uses id to get a document, which is decoded into doc.
func (c *collection) getAtID(ctx context.Context, id string, doc driver.Document) error {
	body, err := c.call(ctx, "GET", id, nil)
	if err != nil {
		return err
	}
	return decodeDoc(body, doc, c.idField)
}

func (c *collection) create(ctx context.Context, a *driver.Action) error {
	id, err := c.docID(a.Doc, false)
	if err != nil {
		return err
	}
	bytes, err := encodeDoc(a.Doc, id)
	if err != nil {
		return err
	}
	_, err = c.call(ctx, "POST", "", bytes)
	return err
}

func (c *collection) put(ctx context.Context, a *driver.Action, mustExist bool) error {
	//XXXXXXXXXXXXXXXX mustExist
	id, err := c.docID(a.Doc, true)
	if err != nil {
		return err
	}
	bytes, err := encodeDoc(a.Doc, id)
	if err != nil {
		return err
	}
	_, err = c.call(ctx, "PUT", id, bytes)
	return err
}

func (c *collection) delete(ctx context.Context, a *driver.Action) error {
	id, err := c.docID(a.Doc, true)
	if err != nil {
		return err
	}
	rev, err := a.Doc.GetField(docstore.RevisionField)
	if err != nil {
		return err
	}
	_, err = c.call(ctx, "DELETE", fmt.Sprintf("%v?rev=%v", id, rev), nil)
	return err
}

func (c *collection) update(ctx context.Context, a *driver.Action) error {
	// CouchDB doesn't have in-place update, so we must perform a read-modify-write cycle.
	id, err := c.docID(a.Doc, true)
	if err != nil {
		return err
	}
	m := map[string]interface{}{}
	doc, err := driver.NewDocument(m)
	if err != nil {
		return err
	}
	if err := c.getAtID(ctx, id, doc); err != nil {
		return err
	}
	for _, mod := range a.Mods {
		if err := modifyDoc(doc, mod); err != nil {
			return err
		}
	}
	bytes, err := encodeDoc(doc, id)
	if err != nil {
		return err
	}
	_, err = c.call(ctx, "PUT", id, bytes)
	return err
}

func modifyDoc(doc driver.Document, mod driver.Mod) error {
	if mod.Value == nil {
		var parent map[string]interface{}
		if len(mod.FieldPath) == 1 {
			parent = doc.Origin.(map[string]interface{})
		} else {
			v, err := doc.Get(mod.FieldPath[:len(mod.FieldPath)-1])
			// ignore deletes of non-existent fields
			if err != nil {
				return nil
			}
			var ok bool
			parent, ok = v.(map[string]interface{})
			if !ok {
				return nil
			}
		}
		delete(parent, mod.FieldPath[len(mod.FieldPath)-1])
	} else {
		if err := doc.Set(mod.FieldPath, mod.Value); err != nil {
			return err
		}
	}
	return nil
}

func (c *collection) call(ctx context.Context, method, urlSuffix string, body []byte) ([]byte, error) {
	url := c.urlPrefix + c.name
	if urlSuffix != "" {
		url += "/" + urlSuffix
	}
	return callCouch(ctx, c.client, method, url, body)
}

func callCouch(ctx context.Context, client *http.Client, method, url string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	outBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode/100 != 2 {
		return nil, gcerr.Newf(httpCode(res.StatusCode), nil, "(http code %d) %s", res.StatusCode, string(outBody))
	}
	return outBody, nil
}

func (c *collection) docID(doc driver.Document, required bool) (string, error) {
	// TODO(jba): can we factor this out into something in driver?
	if c.idField != "" {
		id, err := doc.GetField(c.idField)
		if err != nil && required {
			return "", gcerr.New(gcerr.InvalidArgument, err, 2, "document missing ID")
		}
		if id == nil {
			return "", nil
		}
		// Check that the reflect kind is String so we can support any type whose underlying type
		// is string. E.g. "type DocName string".
		vn := reflect.ValueOf(id)
		if vn.Kind() != reflect.String {
			return "", gcerr.Newf(gcerr.InvalidArgument, nil, "ID field %q with value %v is not a string",
				c.idField, id)
		}
		return vn.String(), nil
	}
	id := c.idFunc(doc.Origin)
	if id == "" && required {
		return "", gcerr.New(gcerr.InvalidArgument, nil, 2, "document missing ID")
	}
	return id, nil

}

var httpCodes = map[int]gcerr.ErrorCode{
	http.StatusBadRequest: gcerr.InvalidArgument,
	http.StatusNotFound:   gcerr.NotFound,
	http.StatusConflict:   gcerr.FailedPrecondition,
}

func httpCode(code int) gcerr.ErrorCode {
	if gcode, ok := httpCodes[code]; ok {
		return gcode
	}
	return gcerr.Unknown
}

////////////////////////////////////////////////////////////////

func (c *collection) RunGetQuery(context.Context, *driver.Query) (driver.DocumentIterator, error) {
	return nil, errors.New("unimp")
}

func (c *collection) QueryPlan(*driver.Query) (string, error) {
	return "", errors.New("unimp")
}

func (c *collection) As(i interface{}) bool { return false }
