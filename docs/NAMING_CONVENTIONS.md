# pairDB: Naming Conventions

This document outlines the naming conventions used throughout the pairDB project to ensure consistency and production-readiness.

## Directory Naming Conventions

### Root Level Directories

- **Multi-word directories**: Use **kebab-case** (hyphens)
  - `api-gateway/` - API Gateway service implementation
  - `storage-node/` - Storage Node service implementation
  - `local-setup/` - Local development setup scripts and configurations

- **Single-word directories**: Use **lowercase**
  - `coordinator/` - Coordinator service implementation
  - `docs/` - Documentation directory

### Documentation Structure

The `docs/` directory follows consistent naming:

```
docs/
├── api-gateway/          # API Gateway design documents (kebab-case)
│   ├── class-diagram.md
│   ├── design.md
│   ├── low-level-design.md
│   ├── sequence-diagrams.md
│
├── coordinator/          # Coordinator design documents (lowercase)
│   ├── api-contracts.md
│   ├── class-diagram.md
│   ├── design.md
│   ├── low-level-design.md
│   └── sequence-diagrams.md
│
├── storage-node/         # Storage Node design documents (kebab-case)
│   ├── api-contracts.md
│   ├── class-diagram.md
│   ├── design.md
│   ├── low-level-design.md
│   └── sequence-diagrams.md
│
├── api-contracts.md      # Global API contracts
├── high-level-design.md  # High-level system design
├── requirements.md       # System requirements
├── use-case-diagram.md   # Global use case diagram
└── README.md             # Documentation index
```

## File Naming Conventions

### Markdown Files

All markdown files use **kebab-case** (hyphens):
- `api-contracts.md`
- `high-level-design.md`
- `low-level-design.md`
- `sequence-diagrams.md`
- `class-diagram.md`
- `use-case-diagram.md`

### Configuration Files

- YAML files: `config.yaml`, `deployment.yaml`
- Shell scripts: `deploy.sh`, `cleanup.sh`, `build-images.sh`

## Naming Standards Summary

| Type | Convention | Examples |
|------|------------|----------|
| Multi-word directories | kebab-case | `api-gateway/`, `storage-node/`, `local-setup/` |
| Single-word directories | lowercase | `coordinator/`, `docs/` |
| Markdown files | kebab-case | `class-diagram.md`, `use-case-diagram.md` |
| Go packages | lowercase | `internal/`, `pkg/`, `cmd/` |
| Kubernetes resources | kebab-case | `deployment.yaml`, `service.yaml` |

## Rationale

1. **Consistency**: All multi-word identifiers use kebab-case for uniformity
2. **Readability**: Kebab-case is easier to read than snake_case in URLs and file paths
3. **Cross-platform**: Kebab-case works well across all operating systems
4. **Industry Standard**: Common practice in Go projects and Kubernetes ecosystems
5. **URL-friendly**: Kebab-case is URL-friendly and doesn't require encoding

## Implementation Folders

The following implementation folders maintain their existing structure and are not modified:
- `api-gateway/` - Service implementation
- `coordinator/` - Service implementation  
- `storage-node/` - Service implementation

These folders contain production code and follow Go project conventions internally.

