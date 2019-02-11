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

package dynamodocstore_test

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/dynamodocstore"
)

func Example() {
	ctx := context.Background()
	sess, err := session.NewSession()
	if err != nil {
		fmt.Println(err)
		return
	}
	coll := dynamodocstore.OpenCollection(dynamodb.New(sess), "docstore-test", "_id", "")
	_, err = coll.Actions().Put(docstore.Document{"_id": "Alice", "score": 25}).Do(ctx)
	fmt.Println(err)
	// Output: <nil>
}
