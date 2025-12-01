module github.com/MasaeProject/ApiNatsBridge

go 1.24.4

require (
	github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver v0.0.0-20260219121602-30228cef019a
	github.com/kagurazakayashi/libNyaruko_Go/nyanats v0.0.0-20251124125130-bcd53996db05
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/nats-io/nats.go v1.49.0 // indirect
	github.com/nats-io/nkeys v0.4.12 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
)

replace github.com/kagurazakayashi/libNyaruko_Go/nyanats => ../libNyaruko_Go/nyanats

replace github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver => ../libNyaruko_Go/nyaapiserver
