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

// Package natspubsub provides a pubsub implementation for NATS.
// TODO(jba): more detail
//
// TODO(jba): implement and document As
package natspubsub

import (
	"context"
	"errors"

	"github.com/google/go-cloud/pubsub/driver"
	stan "github.com/nats-io/go-nats-streaming"
)

type topic struct {
	conn    stan.Conn
	subject string
}

func NewTopic(conn stan.Conn, name string) driver.Topic {
	return &topic{conn: conn, subject: name}
}

func (t *topic) SendBatch(ctx context.Context, msgs []*driver.Message) error {
	errc := make(chan error)
	ah := func(_ string, err error) { errc <- err }
	for _, m := range msgs {
		if len(m.Metadata) != 0 {
			return errors.New("natpubsub: SendBatch: attributes are not empty")
		}
		guid, err := t.conn.PublishAsync(t.subject, m.Body, ah)
		if err != nil {
			return err
		}
		m.AckID = guid
	}
	for range msgs {
		if err := <-errc; err != nil {
			return err
		}
	}
	return nil
}

func (t *topic) Close() error {
	// TODO: who will close the stan.Conn?
	return nil
}

func NewSubscription(conn stan.Conn) {
}
