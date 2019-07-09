module gocloud.dev/samples

require (
	contrib.go.opencensus.io/exporter/stackdriver v0.12.1
	github.com/Azure/azure-pipeline-go v0.1.9
	github.com/Azure/azure-storage-blob-go v0.6.0
	github.com/aws/aws-sdk-go v1.19.45
	github.com/go-sql-driver/mysql v1.4.1
	github.com/google/go-cmp v0.3.0
	github.com/google/subcommands v1.0.1
	github.com/google/uuid v1.1.1
	github.com/google/wire v0.3.0
	github.com/gorilla/mux v1.7.2
	github.com/streadway/amqp v0.0.0-20190404075320-75d898a42a94
	go.opencensus.io v0.22.0
	gocloud.dev v0.15.0
	gocloud.dev/docstore/mongodocstore v0.0.0-00010101000000-000000000000
	gocloud.dev/pubsub/kafkapubsub v0.15.0
	gocloud.dev/pubsub/natspubsub v0.15.0
	gocloud.dev/pubsub/rabbitpubsub v0.15.0
	gocloud.dev/runtimevar/etcdvar v0.15.0
	gocloud.dev/secrets/hashivault v0.15.0
	google.golang.org/genproto v0.0.0-20190605220351-eb0b1bdb6ae6
	gopkg.in/pipe.v2 v2.0.0-20140414041502-3c2ca4d52544
)

replace gocloud.dev => ../

replace gocloud.dev/docstore/mongodocstore => ../docstore/mongodocstore

replace gocloud.dev/pubsub/kafkapubsub => ../pubsub/kafkapubsub

replace gocloud.dev/pubsub/natspubsub => ../pubsub/natspubsub

replace gocloud.dev/pubsub/rabbitpubsub => ../pubsub/rabbitpubsub

replace gocloud.dev/runtimevar/etcdvar => ../runtimevar/etcdvar

replace gocloud.dev/secrets/hashivault => ../secrets/hashivault

replace golang.org/x/xerrors => ../../../go/src/golang.org/x/xerrors

go 1.13
