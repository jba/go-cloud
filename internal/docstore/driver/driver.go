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

// A Collection is a set of documents.
type Collection interface {
	// RunActions executes a sequence of actions.
	// Implementations are free to execute the actions however they wish, but it must
	// appear as if they were executed in order. The actions need not happen
	// atomically. The first return value is the number of actions successfully
	// executed. It is returned even if err != nil.
	RunActions(context.Context, []*Action) (int, error)

	// ErrorCode should return a code that describes the error, which was returned by
	// one of the other methods in this interface.
	ErrorCode(error) gcerr.ErrorCode

	RunQuery(context.Context, *Query) error
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
