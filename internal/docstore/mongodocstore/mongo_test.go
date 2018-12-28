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

package mongodocstore

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"testing"

	"github.com/globalsign/mgo"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/docstore/drivertest"
)

const (
	urlFormatString = "mongodb://admin:%s@go-cloud-shard-00-00-ngy4g.gcp.mongodb.net:27017,go-cloud-shard-00-01-ngy4g.gcp.mongodb.net:27017,go-cloud-shard-00-02-ngy4g.gcp.mongodb.net:27017/test?ssl=true&replicaSet=Go-Cloud-shard-0&authSource=admin"

	dbName         = "docstore-test"
	collectionName = "docstore-test"
)

var password = flag.String("atlas-password", "", "password for MongoDB Atlas cluster")

type harness struct {
	db *mgo.Database
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	if *password == "" {
		t.Skip("missing -atlas-password")
	}
	url := fmt.Sprintf(urlFormatString, url.PathEscape(*password))
	session, err := mgo.Dial(url)
	if err != nil {
		return nil, err
	}
	return &harness{session.DB(dbName)}, nil
}

func (h *harness) MakeCollection(context.Context) (driver.Collection, error) {
	coll := newCollection(h.db, collectionName)
	if err := coll.coll.DropCollection(); err != nil {
		return nil, err
	}
	return coll, nil
}

func (h *harness) Close() {
	h.db.Session.Close()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}
