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

// Package driver defines a set of interfaces that the docstore package uses to
// interact with the underlying services.
package driver // import "gocloud.dev/internal/docstore/driver"

import (
	"context"

	"gocloud.dev/internal/gcerr"
)

type Collection interface {
	KeyFields() []string
	RunActions(context.Context, []*Action) (int, error)
	RunQuery(context.Context, *Query) error
	ErrorCode(error) gcerr.ErrorCode
}

type ActionKind int

const (
	Create ActionKind = iota
	Replace
	Put
	Get
	Delete
	Update
)

type Action struct {
	Kind       ActionKind
	Doc        Document
	FieldPaths [][]string // for Get only
	Mods       []Mod      // for Update only
}

type Mod struct {
	FieldPath []string
	Value     interface{}
}

type Query struct {
	FieldPaths []string
	Filters    []Filter
	Limit      int
}

type Filter struct {
	Field string
	Op    string
	Value interface{}
}
