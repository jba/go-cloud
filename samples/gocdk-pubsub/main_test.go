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

package main_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/streadway/amqp"
	"gocloud.dev/internal/testing/cmdtest"
)

// Requires rabbit to be running. Run pubsub/rabbitpubsub/localrabbit.sh.

func Test(t *testing.T) {
	if err := exec.Command("go", "build").Run(); err != nil {
		t.Fatal(err)
	}
	tf, err := cmdtest.ReadTestFile("pubsub.ct")
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("PATH", cwd)
	os.Setenv("RABBIT_SERVER_URL", rabbitURL)
	if err := initRabbit(); err != nil {
		t.Fatal(err)
	}
	if diff := tf.Compare(); diff != "" {
		t.Error(diff)
	}
}

const (
	rabbitURL = "amqp://guest:guest@localhost:5672/"

	// These names must match the URLs in the pubsub.ct file.
	topicName        = "sample-topic"
	subscriptionName = "sample-subscription"
)

// Set up a topic and subscription.
func initRabbit() error {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if err := ch.Confirm(false); err != nil {
		return err
	}
	err = ch.ExchangeDeclare(topicName,
		"fanout", // kind
		false,    // durable
		false,    // delete when unused
		false,    // internal
		false,    // wait for server response
		nil)      // args
	if err != nil {
		return err
	}
	q, err := ch.QueueDeclare(subscriptionName,
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // wait for server response
		nil)   // args
	if err != nil {
		return err
	}
	return ch.QueueBind(q.Name, q.Name, topicName, false, nil)
}
