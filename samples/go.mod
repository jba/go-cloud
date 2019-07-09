module gocloud.dev/samples

require (
	github.com/google/go-cmp v0.3.0
	github.com/google/subcommands v1.0.1
	gocloud.dev v0.15.0
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
