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

package driver

import (
	"reflect"

	"gocloud.dev/internal/docstore/internal/fields"
)

var fieldCache = fields.NewCache(nil, nil, nil)

type Document struct {
	m      map[string]interface{} // nil if it's a *struct
	s      reflect.Value          // the struct reflected
	fields fields.List            // for structs
}

func NewDocument(doc interface{}) (Document, error) {
	// TODO: handle a nil *struct?
	if m, ok := doc.(map[string]interface{}); ok {
		return Document{m: m}, nil
	}
	v := reflect.ValueOf(doc)
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return Document{}, Errorf(InvalidArgument, nil, "expecting struct or map[string]interface{}, got %s", t)
	}
	fields, err := fieldCache.Fields(t)
	if err != nil {
		return Document{}, err
	}
	return Document{s: v, fields: fields}, nil
}

func (d Document) GetField(field string) (interface{}, error) {
	if d.m != nil {
		x, ok := d.m[field]
		if !ok {
			return nil, Errorf(NotFound, nil, "field %q not found in map", field)
		}
		return x, nil
	} else {
		v, err := d.structField(field)
		if err != nil {
			return nil, err
		}
		return v.Interface(), nil
	}
}

func (d Document) getDocument(fp []string) (Document, error) {
	if len(fp) == 0 {
		return d, nil
	}
	x, err := d.GetField(fp[0])
	if err != nil {
		return Document{}, err
	}
	d2, err := NewDocument(x)
	if err != nil {
		return Document{}, err
	}
	return d2.getDocument(fp[1:])
}

func (d Document) Get(fp []string) (interface{}, error) {
	d2, err := d.getDocument(fp[:len(fp)-1])
	if err != nil {
		return nil, err
	}
	return d2.GetField(fp[len(fp)-1])
}

func (d Document) structField(name string) (reflect.Value, error) {
	f := d.fields.Match(name)
	if f == nil {
		return reflect.Value{}, Errorf(NotFound, nil, "field %q not found in struct type %s", name, d.s.Type())
	}
	fv, ok := fieldByIndex(d.s, f.Index)
	if !ok {
		return reflect.Value{}, Errorf(InvalidArgument, nil, "nil embedded pointer; cannot get field %q from %s",
			name, d.s.Type())
	}
	return fv, nil
}

// Does this need to create sub-maps, etc. as necessary?
func (d Document) set(fp []string, val interface{}) error {
	d2, err := d.getDocument(fp[:len(fp)-1])
	if err != nil {
		return err
	}
	return d2.SetField(fp[len(fp)-1], val)
}

func (d Document) SetField(field string, val interface{}) error {
	if d.m != nil {
		d.m[field] = val
		return nil
	}
	v, err := d.structField(field)
	if err != nil {
		return err
	}
	if !v.CanSet() {
		return Errorf(InvalidArgument, nil, "cannot set field %s in struct of type %s: not addressable",
			field, d.s.Type())
	}
	v.Set(reflect.ValueOf(val))
	return nil
}

func (d Document) Encode(e Encoder) error {
	if d.m != nil {
		return encodeMap(reflect.ValueOf(d.m), e)
	}
	return encodeStructWithFields(d.s, d.fields, e)
}

func (d Document) Decode(dec Decoder) error {
	if d.m != nil {
		return Decode(reflect.ValueOf(d.m), dec)
	}
	return Decode(d.s, dec)
}
