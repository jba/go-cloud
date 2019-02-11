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

package dynamodocstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dyn "github.com/aws/aws-sdk-go/service/dynamodb"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
)

type collection struct {
	db           *dyn.DynamoDB
	name         string // DynamoDB table name
	partitionKey string
	sortKey      string
	ean          map[string]*string
}

func OpenCollection(db *dyn.DynamoDB, tableName, partitionKey, sortKey string) *docstore.Collection {
	return docstore.NewCollection(newCollection(db, tableName, partitionKey, sortKey))
}

var (
	existsCond    = "attribute_exists(#pk)"
	notExistsCond = "attribute_not_exists(#pk)"
)

func newCollection(db *dyn.DynamoDB, tableName, partitionKey, sortKey string) *collection {
	c := &collection{
		db:           db,
		name:         tableName,
		partitionKey: partitionKey,
		sortKey:      sortKey,
		ean: map[string]*string{
			"#pk": &partitionKey,
		},
	}
	return c

}

func (c *collection) KeyFields() []string {
	if c.sortKey == "" {
		return []string{c.partitionKey}
	} else {
		return []string{c.partitionKey, c.sortKey}
	}
}

func (c *collection) RunActions(ctx context.Context, actions []driver.Action) (int, error) {
	for i, a := range actions {
		var err error
		switch a.Kind {
		case driver.Create:
			err = c.put(ctx, a.Doc, &notExistsCond)
		case driver.Replace:
			err = c.put(ctx, a.Doc, &existsCond)
		case driver.Put:
			err = c.put(ctx, a.Doc, nil)
		case driver.Delete:
			err = c.delete(ctx, a.Doc)
		case driver.Get:
			err = c.get(ctx, a.Doc, a.FieldPaths)
		case driver.Update:
			err = c.update(ctx, a.Doc, a.Mods)
		default:
			panic("unimp")
		}
		if err != nil {
			return i, err
		}
	}
	return len(actions), nil
}

func (c *collection) missingKeyField(m map[string]*dyn.AttributeValue) string {
	if _, ok := m[c.partitionKey]; !ok {
		return c.partitionKey
	}
	if _, ok := m[c.sortKey]; !ok && c.sortKey != "" {
		return c.sortKey
	}
	return ""
}

func (c *collection) put(ctx context.Context, doc interface{}, condition *string) error {
	item, err := encodeDoc(doc)
	if err != nil {
		return err
	}
	mf := c.missingKeyField(item)
	if condition != &notExistsCond && mf != "" {
		return fmt.Errorf("missing key field %q", mf)
	}
	if mf == c.partitionKey {
		item[c.partitionKey] = new(dyn.AttributeValue).SetS(driver.UniqueString())
	}
	if c.sortKey != "" && mf == c.sortKey {
		// It doesn't make sense to generate a random sort key.
		return fmt.Errorf("missing sort key %q", c.sortKey)
	}
	in := &dyn.PutItemInput{
		TableName:           &c.name,
		Item:                item,
		ConditionExpression: condition,
	}
	if condition != nil {
		in.ExpressionAttributeNames = c.ean
	}
	_, err = c.db.PutItemWithContext(ctx, in)
	return err
}

func (c *collection) get(ctx context.Context, doc interface{}, fieldpaths []string) error {
	key, err := encodeDocKeyFields(doc, c.partitionKey, c.sortKey)
	if err != nil {
		return err
	}
	if len(fieldpaths) > 0 {
		return errors.New("Get with field paths unimplemented")
	}
	in := &dyn.GetItemInput{
		TableName: &c.name,
		Key:       key,
	}
	out, err := c.db.GetItemWithContext(ctx, in)
	if err != nil {
		return err
	}
	return decodeDoc(doc, out.Item)
}

func (c *collection) delete(ctx context.Context, doc interface{}) error {
	key, err := encodeDocKeyFields(doc, c.partitionKey, c.sortKey)
	if err != nil {
		return err
	}
	in := &dyn.DeleteItemInput{
		TableName:                &c.name,
		Key:                      key,
		ConditionExpression:      &existsCond,
		ExpressionAttributeNames: c.ean,
	}
	_, err = c.db.DeleteItemWithContext(ctx, in)
	return err
}

func (c *collection) update(ctx context.Context, doc interface{}, mods driver.Mods) error {
	key, err := encodeDocKeyFields(doc, c.partitionKey, c.sortKey)
	if err != nil {
		return err
	}
	eav := map[string]*dyn.AttributeValue{}
	var setActions, delPaths []string
	for fp, v := range mods {
		if v == nil {
			delPaths = append(delPaths, fp)
		} else {
			av, err := encodeValue(v)
			if err != nil {
				return err
			}
			vn := fmt.Sprintf(":%d", len(eav))
			setActions = append(setActions, fmt.Sprintf("%s = %s", fp, vn))
			eav[vn] = av
		}
	}
	var setexp, delexp string
	if len(setActions) > 0 {
		setexp = "SET " + strings.Join(setActions, ", ")
	}
	if len(delPaths) > 0 {
		delexp = "REMOVE " + strings.Join(delPaths, ", ")
	}
	uexp := setexp + " " + delexp
	in := &dyn.UpdateItemInput{
		TableName:                 &c.name,
		Key:                       key,
		ConditionExpression:       &existsCond,
		UpdateExpression:          &uexp,
		ExpressionAttributeNames:  c.ean,
		ExpressionAttributeValues: eav,
	}
	_, err = c.db.UpdateItemWithContext(ctx, in)
	return err
}

func (c *collection) RunQuery(ctx context.Context, q *driver.Query) error {
	return errors.New("unimp")
}
