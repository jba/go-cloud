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
	"context"
	"testing"

	"cloud.google.com/go/firestore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/docstore/drivertest"
)

const (
	projectID      = "firestore2-jba"
	collectionName = "docstore-test"
	keyName        = "_id"
)

type harness struct {
	client *firestore.Client
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &harness{client}, nil
}

func (h *harness) MakeCollection(context.Context) (driver.Collection, error) {
	return newCollection(h.client, collectionName, keyName), nil
}

func (h *harness) Close() {
	h.client.Close()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}
