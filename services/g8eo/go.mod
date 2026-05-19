module github.com/g8e-ai/g8e/services/g8eo

go 1.25.0

toolchain go1.25.9

replace github.com/g8e-ai/g8e/protocol => ../../protocol

require (
	github.com/g8e-ai/g8e/protocol v0.0.0-00010101000000-000000000000
	github.com/go-webauthn/webauthn v0.17.3
	github.com/gorilla/websocket v1.5.3
	github.com/stretchr/testify v1.11.1
	golang.org/x/crypto v0.51.0
	google.golang.org/grpc v1.81.0
	modernc.org/sqlite v1.50.0
)

require (
	github.com/fxamacker/cbor/v2 v2.9.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/x v0.2.5 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/x448/float16 v0.8.4 // indirect
)

require (
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260504160031-60b97b32f348 // indirect
	google.golang.org/protobuf v1.36.11
)

require (
	// test-only (not compiled into the operator binary)
	github.com/davecgh/go-spew v1.1.1 // indirect
	// compiled into the operator binary (runtime deps of direct dependencies)
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	golang.org/x/sys v0.44.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/libc v1.72.2 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
