module github.com/g8e-ai/g8e

go 1.26.4

require (
	github.com/g8e-ai/g8e/protocol v0.0.0
	github.com/go-git/go-git/v5 v5.19.1
	github.com/go-webauthn/webauthn v0.17.4
	github.com/gorilla/websocket v1.5.3
	github.com/pkg/sftp v1.13.10
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/testify v1.11.1
	github.com/swaggo/http-swagger v1.3.4
	github.com/swaggo/swag v1.16.6
	golang.org/x/crypto v0.52.0
	golang.org/x/time v0.15.0
	modernc.org/sqlite v1.52.0
)

replace github.com/g8e-ai/g8e/protocol => ./protocol

require (
	dario.cat/mergo v1.0.2 // indirect
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/cyphar/filepath-securejoin v0.6.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fxamacker/cbor/v2 v2.9.2 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.9.0 // indirect
	github.com/go-openapi/jsonpointer v0.23.1 // indirect
	github.com/go-openapi/jsonreference v0.21.6 // indirect
	github.com/go-openapi/spec v0.22.5 // indirect
	github.com/go-openapi/swag/conv v0.26.1 // indirect
	github.com/go-openapi/swag/jsonname v0.26.1 // indirect
	github.com/go-openapi/swag/jsonutils v0.26.1 // indirect
	github.com/go-openapi/swag/loading v0.26.1 // indirect
	github.com/go-openapi/swag/stringutils v0.26.1 // indirect
	github.com/go-openapi/swag/typeutils v0.26.1 // indirect
	github.com/go-openapi/swag/yamlutils v0.26.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/x v0.2.6 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pjbgf/sha1cd v0.6.0 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/skeema/knownhosts v1.3.2 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/swaggo/files v1.0.1 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/tools v0.45.0 // indirect
	google.golang.org/grpc v1.81.1 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
)

require (
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260523011958-0a33c5d7ca68 // indirect
	google.golang.org/protobuf v1.36.11
)

require (
	// test-only (not compiled into the Node binary)
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	// compiled into the Node binary (runtime deps of direct dependencies)
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.46.0
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/libc v1.72.5 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
