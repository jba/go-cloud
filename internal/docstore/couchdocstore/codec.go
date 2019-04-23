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
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
)

// encodeDoc encodes a driver.Document as an encoded JSON object.
func encodeDoc(doc driver.Document, id string) ([]byte, error) {
	var e encoder
	if err := doc.Encode(&e); err != nil {
		return nil, err
	}
	m := e.val.(map[string]interface{})
	if id != "" {
		m[couchIDField] = id
	}
	if rev := extract(m, docstore.RevisionField); rev != nil {
		m[couchRevisionField] = rev
	}
	return json.Marshal(m)
}

func extract(m map[string]interface{}, key string) interface{} {
	if val, ok := m[key]; ok {
		delete(m, key)
		return val
	}
	return nil
}

type encoder struct {
	val interface{}
}

func (e *encoder) EncodeNil()                                { e.val = nil }
func (e *encoder) EncodeBool(x bool)                         { e.val = x }
func (e *encoder) EncodeInt(x int64)                         { e.val = x }
func (e *encoder) EncodeUint(x uint64)                       { e.val = x }
func (e *encoder) EncodeBytes(x []byte)                      { e.val = x }
func (e *encoder) EncodeFloat(x float64)                     { e.val = x }
func (e *encoder) EncodeComplex(x complex128)                { e.val = x }
func (e *encoder) EncodeString(x string)                     { e.val = x }
func (e *encoder) ListIndex(int)                             { panic("impossible") }
func (e *encoder) MapKey(string)                             { panic("impossible") }
func (e *encoder) EncodeSpecial(reflect.Value) (bool, error) { return false, nil } // no special handling

func (e *encoder) EncodeList(n int) driver.Encoder {
	// All slices and arrays are encoded as []interface{}
	s := make([]interface{}, n)
	e.val = s
	return &listEncoder{s: s}
}

type listEncoder struct {
	s []interface{}
	encoder
}

func (e *listEncoder) ListIndex(i int) { e.s[i] = e.val }

type mapEncoder struct {
	m map[string]interface{}
	encoder
}

func (e *encoder) EncodeMap(n int, _ bool) driver.Encoder {
	m := make(map[string]interface{}, n)
	e.val = m
	return &mapEncoder{m: m}
}

func (e *mapEncoder) MapKey(k string) { e.m[k] = e.val }

////////////////////////////////////////////////////////////////

func decodeDoc(body []byte, ddoc driver.Document, idField string) error {
	jd := json.NewDecoder(bytes.NewReader(body))
	jd.UseNumber()
	var m map[string]interface{}
	if err := jd.Decode(&m); err != nil {
		return err
	}
	var id interface{}
	if idField != "" {
		id = extract(m, couchIDField)
	}
	rev := extract(m, couchRevisionField)
	if err := ddoc.Decode(decoder{m}); err != nil {
		return err
	}
	if idField != "" {
		if err := ddoc.SetField(idField, id); err != nil {
			return err
		}
	}
	return ddoc.SetField(docstore.RevisionField, rev)
}

type decoder struct {
	val interface{}
}

func (d decoder) String() string {
	return fmt.Sprint(d.val)
}

func (d decoder) AsNull() bool {
	return d.val == nil
}

func (d decoder) AsBool() (bool, bool) {
	b, ok := d.val.(bool)
	return b, ok
}

func (d decoder) AsString() (string, bool) {
	s, ok := d.val.(string)
	return s, ok
}

func (d decoder) AsInt() (int64, bool) {
	num, ok := d.val.(json.Number)
	if !ok {
		return 0, false
	}
	x, err := num.Int64()
	if err != nil {
		return 0, false
	}
	return x, true
}

func (d decoder) AsUint() (uint64, bool) {
	num, ok := d.val.(json.Number)
	if !ok {
		return 0, false
	}
	x, err := strconv.ParseUint(num.String(), 10, 64)
	if err != nil {
		return 0, false
	}
	return x, true

}

func (d decoder) AsFloat() (float64, bool) {
	num, ok := d.val.(json.Number)
	if !ok {
		return 0, false
	}
	x, err := num.Float64()
	if err != nil {
		return 0, false
	}
	return x, true
}

func (d decoder) AsComplex() (complex128, bool) {
	// TODO(jba): implement
	return 0, false
}

func (d decoder) AsBytes() ([]byte, bool) {
	// TODO(jba): implement
	return nil, false
}

func (d decoder) AsInterface() (interface{}, error) {
	return toGoValue(d.val)
}

func toGoValue(v interface{}) (interface{}, error) {
	switch v := v.(type) {
	default:
		return v, nil
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return i, nil
		}
		f, err := v.Float64()
		if err == nil {
			return f, nil
		}
		// Should be impossible to get here.
		return nil, fmt.Errorf("number %q is neither float nor int", v)

	case []interface{}:
		r := make([]interface{}, len(v))
		for i, e := range v {
			d, err := toGoValue(e)
			if err != nil {
				return nil, err
			}
			r[i] = d
		}
		return r, nil
	case map[string]interface{}:
		r := map[string]interface{}{}
		for k, e := range v {
			d, err := toGoValue(e)
			if err != nil {
				return nil, err
			}
			r[k] = d
		}
		return r, nil
	}
}

func (d decoder) ListLen() (int, bool) {
	if s, ok := d.val.([]interface{}); ok {
		return len(s), true
	}
	return 0, false
}

func (d decoder) DecodeList(f func(i int, d2 driver.Decoder) bool) {
	for i, e := range d.val.([]interface{}) {
		if !f(i, decoder{e}) {
			return
		}
	}
}

func (d decoder) MapLen() (int, bool) {
	if m, ok := d.val.(map[string]interface{}); ok {
		return len(m), true
	}
	return 0, false
}

func (d decoder) DecodeMap(f func(key string, d2 driver.Decoder) bool) {
	for k, v := range d.val.(map[string]interface{}) {
		if !f(k, decoder{v}) {
			return
		}
	}
}

func (d decoder) AsSpecial(v reflect.Value) (bool, interface{}, error) {
	return false, nil, nil
}
