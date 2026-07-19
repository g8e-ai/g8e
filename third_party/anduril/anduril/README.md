# Anduril Lattice SDK Protos (Vendored)

Vendored protobuf definitions for the [Anduril Lattice](https://buf.build/anduril/lattice-sdk)
platform SDK. These `.proto` files are used by the g8e Lattice adapter
(`internal/adapters/lattice/`) to generate Go gRPC stubs for entity management
and task execution on the Lattice common operational picture (COP).

## Source

Exported from the Buf Schema Registry (BSR):

```bash
buf export buf.build/anduril/lattice-sdk:<commit> --output third_party/anduril/
```

**Pinned commit:** `7bbb3d15fc37438899af0e9738c76ef5`

All `go_package` options have been remapped from the original
`ghe.anduril.dev/anduril/andurilapis-go/` to
`github.com/g8e-ai/g8e/internal/adapters/lattice/gen/` so that generated code
lands in the g8e module.

## Directory Structure

The outer `anduril/` is the vendor namespace; the inner `anduril/` is the BSR
module path. This nesting is required because proto imports use
`import "anduril/entitymanager/v1/..."`.

```
third_party/anduril/anduril/
├── api/v1/                  # HTTP transcoding annotations + API metadata
│   ├── annotations.proto
│   └── http.proto
├── entitymanager/v1/        # Entity Manager service, COP entity lifecycle
│   ├── entity_manager_api.pub.proto   # EntityManagerAPI service definition
│   ├── entity.pub.proto               # Core entity message
│   ├── filter.pub.proto               # Entity query filters
│   └── ... (23 more message definitions)
├── ontology/v1/             # Ontology type definitions
│   └── type.pub.proto
├── taskmanager/v1/          # Task Manager service, task lifecycle & execution
│   ├── task_manager_api.pub.proto     # TaskManagerAPI service definition
│   ├── task.pub.proto                 # Core task message
│   ├── task_api.pub.proto
│   └── manual_control.pub.proto
├── tasks/v2/                # Task catalog & shared task type definitions
│   ├── catalog.pub.proto
│   ├── common.pub.proto
│   ├── objective.pub.proto
│   └── shared/              # Shared task sub-types (ISR, strike, maneuver, etc.)
│       ├── agent_plan_graph.pub.proto
│       ├── isr.pub.proto
│       ├── maneuver.pub.proto
│       ├── strike.pub.proto
│       └── transitions.pub.proto
└── type/                    # Common type definitions
    ├── color.pub.proto
    ├── coords.pub.proto
    └── orbit.pub.proto
```

## Code Generation

Generated Go stubs live in `internal/adapters/lattice/gen/` and are produced
using the adapter-local buf config:

```bash
buf generate --template internal/adapters/lattice/buf.gen.yaml third_party/anduril
```

See `internal/adapters/lattice/buf.gen.yaml` for plugin configuration.

## Updating the Vendored Protos

To update to a newer Lattice SDK commit:

```bash
# 1. Discover the latest commit
buf registry module commit list buf.build/anduril/lattice-sdk

# 2. Export and remap go_package options
buf export buf.build/anduril/lattice-sdk:<new-commit> --output third_party/anduril/
find third_party/anduril -name '*.proto' -exec sed -i \
    's|ghe.anduril.dev/anduril/andurilapis-go/|github.com/g8e-ai/g8e/internal/adapters/lattice/gen/|g' {} +

# 3. Regenerate stubs
rm -rf internal/adapters/lattice/gen
buf generate --template internal/adapters/lattice/buf.gen.yaml third_party/anduril

# 4. Update vendored Go dependencies
go mod vendor
```

Update the pinned commit hash above when doing so. Proto changes should land as
reviewable diffs; no BSR network dependency is needed at build time.

## License

These protobuf definitions are property of Anduril Industries. They are vendored
here solely for generating gRPC stubs to interoperate with the Lattice platform.
See Anduril's terms for usage restrictions.
