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

package driver

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"

	"github.com/golang/protobuf/proto"
	"gocloud.dev/internal/docstore/internal/fields"
	"gocloud.dev/internal/gcerr"
)

var (
	binaryMarshalerType   = reflect.TypeOf(new(encoding.BinaryMarshaler)).Elem()
	binaryUnmarshalerType = reflect.TypeOf(new(encoding.BinaryUnmarshaler)).Elem()
	textMarshalerType     = reflect.TypeOf(new(encoding.TextMarshaler)).Elem()
	textUnmarshalerType   = reflect.TypeOf(new(encoding.TextUnmarshaler)).Elem()
	protoMessageType      = reflect.TypeOf(new(proto.Message)).Elem()
)

type Encoder interface {
	EncodeNull()
	EncodeBool(bool)
	EncodeString(string)
	EncodeInt(int64)
	EncodeUint(uint64)
	EncodeFloat(float64)
	EncodeComplex(complex128)
	EncodeBytes([]byte)
	EncodeList(n int) Encoder
	ListIndex(i int)
	EncodeMap(n int) Encoder
	MapKey(string)

	// If the encoder wants to encode a struct in a special way (other than as a map
	// from its exported fields to values), it should do so here and return true along
	// with any error from the encoding.
	// Otherwise, it should return false.
	EncodeStruct(s reflect.Value) (bool, error)
}

// TODO: support struct tags.
// TODO: for efficiency, enable encoding of only a subset of field paths.

func Encode(v reflect.Value, e Encoder) error {
	return wrap(encode(v, e), gcerr.InvalidArgument)
}

func encode(v reflect.Value, enc Encoder) error {
	if !v.IsValid() {
		enc.EncodeNull()
		return nil
	}
	if v.Type().Implements(binaryMarshalerType) {
		bytes, err := v.Interface().(encoding.BinaryMarshaler).MarshalBinary()
		if err != nil {
			return err
		}
		enc.EncodeBytes(bytes)
		return nil
	}
	if v.Type().Implements(protoMessageType) {
		if v.IsNil() {
			enc.EncodeNull()
		} else {
			bytes, err := proto.Marshal(v.Interface().(proto.Message))
			if err != nil {
				return err
			}
			enc.EncodeBytes(bytes)
		}
		return nil
	}
	if v.Type().Implements(textMarshalerType) {
		bytes, err := v.Interface().(encoding.TextMarshaler).MarshalText()
		if err != nil {
			return err
		}
		enc.EncodeString(string(bytes))
		return nil
	}
	switch v.Kind() {
	case reflect.Bool:
		enc.EncodeBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		enc.EncodeInt(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		enc.EncodeUint(v.Uint())
	case reflect.Float32, reflect.Float64:
		enc.EncodeFloat(v.Float())
	case reflect.Complex64, reflect.Complex128:
		enc.EncodeComplex(v.Complex())
	case reflect.String:
		enc.EncodeString(v.String())
	case reflect.Slice:
		if v.IsNil() {
			enc.EncodeNull()
			return nil
		}
		fallthrough
	case reflect.Array:
		return encodeList(v, enc)
	case reflect.Map:
		return encodeMap(v, enc)
	case reflect.Ptr:
		if v.IsNil() {
			enc.EncodeNull()
			return nil
		}
		return encode(v.Elem(), enc)
	case reflect.Interface:
		if v.IsNil() {
			enc.EncodeNull()
			return nil
		}
		return encode(v.Elem(), enc)

	case reflect.Struct:
		return encodeStruct(v, enc)

	default:
		return gcerr.Newf(gcerr.InvalidArgument, nil, "cannot encode type %s", v.Type())
	}
	return nil
}

// Encode an array or non-nil slice.
func encodeList(v reflect.Value, enc Encoder) error {
	if v.Type().Elem().Kind() == reflect.Uint8 {
		enc.EncodeBytes(v.Bytes())
		return nil
	}
	n := v.Len()
	enc2 := enc.EncodeList(n)
	for i := 0; i < n; i++ {
		if err := encode(v.Index(i), enc2); err != nil {
			return err
		}
		enc2.ListIndex(i)
	}
	return nil
}

func encodeMap(v reflect.Value, enc Encoder) error {
	if v.IsNil() {
		enc.EncodeNull()
		return nil
	}
	keys := v.MapKeys()
	enc2 := enc.EncodeMap(len(keys))
	for _, k := range keys {
		sk, err := stringifyMapKey(k)
		if err != nil {
			return err
		}
		if err := encode(v.MapIndex(k), enc2); err != nil {
			return err
		}
		enc2.MapKey(sk)
	}
	return nil
}

func stringifyMapKey(k reflect.Value) (string, error) {
	// This is basically reflectWithString.resolve, from encoding/json/encode.go.
	if k.Kind() == reflect.String {
		return k.String(), nil
	}
	if tm, ok := k.Interface().(encoding.TextMarshaler); ok {
		b, err := tm.MarshalText()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	switch k.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(k.Uint(), 10), nil
	default:
		return "", gcerr.Newf(gcerr.InvalidArgument, nil, "cannot encode key %s of type %s", k, k.Type())
	}
}

func encodeStruct(v reflect.Value, e Encoder) error {
	didIt, err := e.EncodeStruct(v)
	if didIt {
		return err
	}
	fields, err := fieldCache.Fields(v.Type())
	if err != nil {
		return err
	}
	return encodeStructWithFields(v, fields, e)
}

func encodeStructWithFields(v reflect.Value, fields fields.List, e Encoder) error {
	e2 := e.EncodeMap(len(fields))
	for _, f := range fields {
		fv, ok := fieldByIndex(v, f.Index)
		if ok {
			if err := encode(fv, e2); err != nil {
				return err
			}
			e2.MapKey(f.Name)
		}
	}
	return nil
}

// fieldByIndex retrieves the the field of v at the given index if present.
// v must be a struct. index must refer to a valid field of v's type.
// The second return value is false if there is a nil embedded pointer
// along the path denoted by index.
//
// From encoding/json/encode.go.
func fieldByIndex(v reflect.Value, index []int) (reflect.Value, bool) {
	for _, i := range index {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v, true
}

////////////////////////////////////////////////////////////////

// TODO: consider a fast path: if we are decoding into a struct, assume the same struct
// was used to encode. Then we can build a map from field names to functions, where each
// function avoids all the tests of Decode and contains just the code for setting the field.

// TODO: encoding/json says:"Because null is often used in JSON to mean
// ``not present,'' unmarshaling a JSON null into any other Go type has no effect
// on the value and produces no error."
// Do we want to do that? I don't think so.

// TODO: provide a way to override the check on missing fields.

// TODO: understand the "indirect" method in encoding/json/decode.go and why it's necessary.

type Decoder interface {
	AsString() (string, bool)
	AsInt() (int64, bool)
	AsUint() (uint64, bool)
	AsFloat() (float64, bool)
	AsComplex() (complex128, bool)
	AsBytes() ([]byte, bool)
	AsBool() (bool, bool)
	AsNull() bool

	ListLen() (int, bool)
	DecodeList(func(int, Decoder) bool)
	MapLen() (int, bool)
	DecodeMap(func(string, Decoder) bool)

	// Decode a value without knowing anything about the type.
	AsInterface() (interface{}, error)

	String() string // for error messages
}

// Decode d into v.
func Decode(v reflect.Value, d Decoder) error {
	return wrap(decode(v, d), gcerr.InvalidArgument)
}

func decode(v reflect.Value, d Decoder) error {
	// A Null value sets anything nullable to nil, and has no effect
	// on anything else.
	if d.AsNull() {
		switch v.Kind() {
		case reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice:
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
	}

	// Handle implemented interfaces first.
	if reflect.PtrTo(v.Type()).Implements(binaryUnmarshalerType) {
		if b, ok := d.AsBytes(); ok {
			return v.Addr().Interface().(encoding.BinaryUnmarshaler).UnmarshalBinary(b)
		}
		return decodingError(v, d)
	}
	if reflect.PtrTo(v.Type()).Implements(protoMessageType) {
		if b, ok := d.AsBytes(); ok {
			return proto.Unmarshal(b, v.Addr().Interface().(proto.Message))
		}
		return decodingError(v, d)
	}
	if reflect.PtrTo(v.Type()).Implements(textUnmarshalerType) {
		if s, ok := d.AsString(); ok {
			return v.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s))
		}
		return decodingError(v, d)
	}

	switch v.Kind() {
	case reflect.Bool:
		if b, ok := d.AsBool(); ok {
			v.SetBool(b)
			return nil
		}

	case reflect.String:
		if s, ok := d.AsString(); ok {
			v.SetString(s)
			return nil
		}

	case reflect.Float32, reflect.Float64:
		if f, ok := d.AsFloat(); ok {
			v.SetFloat(f)
			return nil
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, ok := d.AsInt()
		if !ok {
			// Accept a floating-point number with integral value.
			if f, ok := d.AsFloat(); ok {
				i = int64(f)
				if float64(i) != f {
					return fmt.Errorf("docstore: float %f does not fit into %s", f, v.Type())
				}
			}
		}
		if v.OverflowInt(i) {
			return overflowError(i, v.Type())
		}
		v.SetInt(i)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u, ok := d.AsUint()
		if !ok {
			// Accept a floating-point number with integral value.
			if f, ok := d.AsFloat(); ok {
				u = uint64(f)
				if float64(u) != f {
					return fmt.Errorf("docstore: float %f does not fit into %s", f, v.Type())
				}
			}
		}
		if v.OverflowUint(u) {
			return overflowError(u, v.Type())
		}
		v.SetUint(u)
		return nil

	case reflect.Complex64, reflect.Complex128:
		if c, ok := d.AsComplex(); ok {
			v.SetComplex(c)
			return nil
		}

	case reflect.Slice, reflect.Array:
		return decodeList(v, d)

	case reflect.Map:
		return decodeMap(v, d)

	case reflect.Ptr:
		// If the pointer is nil, set it to a zero value.
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return decode(v.Elem(), d)

	case reflect.Struct:
		return decodeStruct(v, d)

	case reflect.Interface:
		if v.NumMethod() == 0 { // empty interface
			// If v holds a pointer, set the pointer.
			if !v.IsNil() && v.Elem().Kind() == reflect.Ptr {
				return decode(v.Elem(), d)
			}
			// Otherwise, create a fresh value.
			x, err := d.AsInterface()
			if err != nil {
				return err
			}
			v.Set(reflect.ValueOf(x))
			return nil
		}
		// Any other kind of interface is an error???
	}

	return decodingError(v, d)
}

func decodeList(v reflect.Value, d Decoder) error {
	if v.Type().Elem().Kind() == reflect.Uint8 {
		if b, ok := d.AsBytes(); ok {
			v.SetBytes(b)
			return nil
		}
		// Fall through to decode the []byte as an ordinary slice.
	}
	dlen, ok := d.ListLen()
	if !ok {
		return decodingError(v, d)
	}
	err := prepareLength(v, dlen)
	if err != nil {
		return err
	}
	d.DecodeList(func(i int, vd Decoder) bool {
		if err != nil || i >= dlen {
			return false
		}
		err = decode(v.Index(i), vd)
		return err == nil
	})
	return err
}

// v must be a slice or array. We want it to be of length wantLen. Prepare it as
// necessary, and return its resulting length.
// If an array is too short, return an error. This behavior differs from
// encoding/json, which can lose data.
func prepareLength(v reflect.Value, wantLen int) error {
	vLen := v.Len()
	if v.Kind() == reflect.Slice {
		// Construct a slice of the right size, avoiding allocation if possible.
		switch {
		case vLen < wantLen: // v too short
			if v.Cap() >= wantLen { // extend its length if there's room
				v.SetLen(wantLen)
			} else { // else make a new one
				v.Set(reflect.MakeSlice(v.Type(), wantLen, wantLen))
			}
		case vLen > wantLen: // v too long; truncate it
			v.SetLen(wantLen)
		}
	} else { // array
		switch {
		case vLen < wantLen: // v too short
			return gcerr.Newf(gcerr.InvalidArgument, nil, "array length %d is too short for incoming list of length %d",
				vLen, wantLen)
		case vLen > wantLen: // v too long; set extra elements to zero
			z := reflect.Zero(v.Type().Elem())
			for i := wantLen; i < vLen; i++ {
				v.Index(i).Set(z)
			}
		}
	}
	return nil
}

// Since a map value is not settable, this function always creates a new
// element for each corresponding map key. Existing values of vm are
// overwritten. This happens even if the map value is something like a pointer
// to a struct, where we could in theory populate the existing struct value
// instead of discarding it. This behavior matches encoding/json.
//
// TODO: what is encoding/json's behavior if a map key already exists?
func decodeMap(v reflect.Value, d Decoder) error {
	mapLen, ok := d.MapLen()
	if !ok {
		return decodingError(v, d)
	}
	t := v.Type()
	if v.IsNil() {
		v.Set(reflect.MakeMapWithSize(t, mapLen))
	}
	et := t.Elem()
	var err error
	kt := v.Type().Key()
	d.DecodeMap(func(key string, vd Decoder) bool {
		if err != nil {
			return false
		}
		el := reflect.New(et).Elem()
		err = decode(el, vd)
		if err != nil {
			return false
		}
		vk, e := unstringifyMapKey(key, kt)
		if e != nil {
			err = e
			return false
		}
		v.SetMapIndex(vk, el)
		return err == nil
	})
	return err
}

func unstringifyMapKey(key string, keyType reflect.Type) (reflect.Value, error) {
	// This code is mostly from the middle of decodeState.object in encoding/json/decode.go.
	// Except for literalStore, which I don't understand.
	switch {
	case keyType.Kind() == reflect.String:
		return reflect.ValueOf(key).Convert(keyType), nil
	case reflect.PtrTo(keyType).Implements(textUnmarshalerType):
		tu := reflect.New(keyType)
		if err := tu.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(key)); err != nil {
			return reflect.Value{}, err
		}
		return tu.Elem(), nil
	case keyType.Kind() == reflect.Interface && keyType.NumMethod() == 0:
		// TODO: remove this case? encoding/json doesn't support it.
		return reflect.ValueOf(key), nil
	default:
		switch keyType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(key, 10, 64)
			if err != nil {
				return reflect.Value{}, err
			}
			if reflect.Zero(keyType).OverflowInt(n) {
				return reflect.Value{}, overflowError(n, keyType)
			}
			return reflect.ValueOf(n).Convert(keyType), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			n, err := strconv.ParseUint(key, 10, 64)
			if err != nil {
				return reflect.Value{}, err
			}
			if reflect.Zero(keyType).OverflowUint(n) {
				return reflect.Value{}, overflowError(n, keyType)
			}
			return reflect.ValueOf(n).Convert(keyType), nil
		default:
			return reflect.Value{}, gcerr.Newf(gcerr.InvalidArgument, nil, "invalid key type %s", keyType)
		}
	}
}

func decodeStruct(v reflect.Value, d Decoder) error {
	fields, err := fieldCache.Fields(v.Type())
	if err != nil {
		return err
	}
	d.DecodeMap(func(key string, d2 Decoder) bool {
		if err != nil {
			return false
		}
		f := fields.Match(key)
		if f == nil {
			err = gcerr.Newf(gcerr.InvalidArgument, nil, "no field matching %q in %s", key, v.Type())
			return false
		}
		fv, ok := fieldByIndexCreate(v, f.Index)
		if !ok {
			err = gcerr.Newf(gcerr.InvalidArgument, nil,
				"setting field %q in %s: cannot create embedded pointer field of unexported type",
				key, v.Type())
			return false
		}
		err = decode(fv, d2)
		return err == nil
	})
	return err
}

// fieldByIndexCreate retrieves the the field of v at the given index if present,
// creating embedded struct pointers where necessary.
// v must be a struct. index must refer to a valid field of v's type.
// The second return value is false If there is a nil embedded pointer of unexported
// type along the path denoted by index.
func fieldByIndexCreate(v reflect.Value, index []int) (reflect.Value, bool) {
	for _, i := range index {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				if !v.CanSet() {
					return reflect.Value{}, false
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v, true
}

func decodingError(v reflect.Value, d Decoder) error {
	return gcerr.Newf(gcerr.InvalidArgument, nil, "cannot set type %s to %s", v.Type(), d)
}

func overflowError(x interface{}, t reflect.Type) error {
	return gcerr.Newf(gcerr.InvalidArgument, nil, "value %v overflows type %s", x, t)
}

func wrap(err error, code gcerr.ErrorCode) error {
	if _, ok := err.(*gcerr.Error); !ok && err != nil {
		err = gcerr.New(code, err, 2, err.Error())
	}
	return err
}
