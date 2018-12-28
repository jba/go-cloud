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

import "fmt"

type ErrorCode int

const (
	// Skip zero to avoid any possible misinterpretation as OK.
	NotFound ErrorCode = iota + 1
	AlreadyExists
	PreconditionFailed
	InvalidArgument
	Unknown
)

var errorCodeString = map[ErrorCode]string{
	NotFound:           "NotFound",
	AlreadyExists:      "AlreadyExists",
	PreconditionFailed: "PreconditionFailed",
	InvalidArgument:    "InvalidArgument",
}

type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", errorCodeString[e.Code], e.Message)
	// TODO: format e.Err
}

func (e *Error) Unwrap() error {
	return e.Err
}

func Errorf(code ErrorCode, wrapped error, format string, args ...interface{}) error {
	return &Error{
		Code:    code,
		Err:     wrapped,
		Message: "docstore: " + fmt.Sprintf(format, args...),
	}
}

func Code(err error) ErrorCode {
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return Unknown
}

func wrap(err error, code ErrorCode) error {
	if _, ok := err.(*Error); !ok && err != nil {
		err = &Error{
			Code:    code,
			Message: err.Error(),
			Err:     err,
		}
	}
	return err
}

// TODO: copy MultiError from appengine.
