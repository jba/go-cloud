// Copyright 2019 The Go Cloud Authors
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

package firedocstore

import (
	"fmt"
	"reflect"
	"time"

	"gocloud.dev/internal/docstore/driver"

	pb "google.golang.org/genproto/googleapis/firestore/v1beta1"
	"google.golang.org/genproto/googleapis/type/latlng"
)

func encodeDoc(doc driver.Document) (map[string]*pb.Value, error) {
	var e encoder
	if err := doc.Encode(&e); err != nil {
		return nil, err
	}
	return e.pv.GetMapValue().Fields, nil
}

func encodeValue(x interface{}) (*pb.Value, error) {
	var e encoder
	if err := driver.Encode(reflect.ValueOf(x), &e); err != nil {
		return nil, err
	}
	return e.pv, nil
}

type encoder struct {
	pv *pb.Value
}

var nullValue = &pb.Value{ValueType: &pb.Value_NullValue{}}

var (
	typeOfGoTime         = reflect.TypeOf(time.Time{})
	typeOfLatLng         = reflect.TypeOf((*latlng.LatLng)(nil))
	typeOfProtoTimestamp = reflect.TypeOf((*ts.Timestamp)(nil))
)

func isLeafType(t reflect.Type) bool {
	return t == typeOfGoTime || t == typeOfLatLng || t == typeOfProtoTimestamp
}

func (e *encoder) EncodeNull()           { e.pv = nullValue }
func (e *encoder) EncodeBool(x bool)     { e.pv = &pb.Value{ValueType: &pb.Value_BooleanValue{x}} }
func (e *encoder) EncodeInt(x int64)     { e.pv = &pb.Value{ValueType: &pb.Value_IntegerValue{x}} }
func (e *encoder) EncodeUint(x uint64)   { e.pv = &pb.Value{ValueType: &pb.Value_IntegerValue{int64(x)}} }
func (e *encoder) EncodeBytes(x []byte)  { e.pv = &pb.Value{ValueType: &pb.Value_BytesValue{x}} }
func (e *encoder) EncodeFloat(x float64) { e.pv = floatval(x) }
func (e *encoder) EncodeString(x string) { e.pv = &pb.Value{ValueType: &pb.Value_StringValue{x}} }

func (e *encoder) ListIndex(int) { panic("impossible") }
func (e *encoder) MapKey(string) { panic("impossible") }

func (e *encoder) EncodeComplex(x complex128) {
	vals := []*pb.Value{floatval(real(x)), floatval(imag(x))}
	e.pv = &pb.Value{ValueType: &pb.Value_ArrayValue{&pb.ArrayValue{Values: vals}}}
}

func (e *encoder) EncodeList(n int) driver.Encoder {
	s := make([]*pb.Value, n)
	e.pv = &pb.Value{ValueType: &pb.Value_ArrayValue{&pb.ArrayValue{Values: s}}}
	return &listEncoder{s: s}
}

func (e *encoder) EncodeStruct(s reflect.Value) (bool, error) {
}

func (e *encoder) EncodeMap(n int) driver.Encoder {
	m := make(map[string]*pb.Value, n)
	e.pv = mapval(m)
	return &mapEncoder{m: m}
}

type listEncoder struct {
	s []*pb.Value
	encoder
}

func (e *listEncoder) ListIndex(i int) { e.s[i] = e.pv }

type mapEncoder struct {
	m map[string]*pb.Value
	encoder
}

func (e *mapEncoder) MapKey(k string) { e.m[k] = e.pv }

func floatval(x float64) *pb.Value { return &pb.Value{ValueType: &pb.Value_DoubleValue{x}} }

func mapval(m map[string]*pb.Value) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_MapValue{&pb.MapValue{Fields: m}}}
}

////////////////////////////////////////////////////////////////

func decodeDoc(ddoc driver.Document, pdoc *pb.Document) error {
	return ddoc.Decode(decoder{mapval(pdoc.Fields)})
}

type decoder struct {
	pv *pb.Value
}

func (d decoder) String() string {
	return fmt.Sprint(d.pv)
}

func (d decoder) AsNull() bool {
	_, ok := d.pv.ValueType.(*pb.Value_NullValue)
	return ok
}

func (d decoder) AsBool() (bool, bool) {
	b, ok := d.pv.ValueType.(*pb.Value_BooleanValue)
	return b, ok
}

func (d decoder) AsString() (string, bool) {
	s, ok := d.pv.ValueType.(*pb.Value_StringValue)
	return s, ok
}

func (d decoder) AsInt() (int64, bool) {
	i, ok := d.pv.ValueType.(*pb.Value_IntegerValue)
	return i, ok
}

func (d decoder) AsUint() (uint64, bool) {
	i, ok := d.pv.ValueType.(*pb.Value_IntegerValue)
	return uint64(i), ok
}

func (d decoder) AsFloat() (float64, bool) {
	f, ok := d.pv.ValueType.(*pb.Value_DoubleValue)
	return f, ok
}

func (d decoder) AsBytes() ([]byte, bool) {
	bs, ok := d.pv.ValueType.(*pb.Value_BytesValue)
	return bs, ok
}

func (d decoder) AsComplex() (complex128, bool) {
	a := d.pv.GetArrayValue()
	if a == nil {
		return 0, false
	}
	vs = a.Values
	if len(vs) != 2 {
		return 0, false
	}
	r, okr := vs[0].ValueType.(*pb.Value_DoubleValue)
	i, oki := vs[1].ValueType.(*pb.Value_DoubleValue)
	if !okr || !oki {
		return 0, false
	}
	return complex(r, i), true
}
