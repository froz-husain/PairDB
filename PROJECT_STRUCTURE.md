# pairDB: Project Structure

This document outlines the production-ready project structure and naming conventions for pairDB.

## Root Directory Structure

```
pairDB/
├── api-gateway/          # API Gateway service (kebab-case)
├── coordinator/          # Coordinator service (lowercase)
├── storage-node/         # Storage Node service (kebab-case)
├── docs/                 # Documentation (lowercase)
├── local-setup/          # Local development setup (kebab-case)
├── README.md             # Project overview
└── PROJECT_STRUCTURE.md  # This file
```

## Naming Conventions

### Directory Naming
- **Multi-word directories**: Use **kebab-case** (hyphens)
  - ✅ `api-gateway/`
  - ✅ `storage-node/`
  - ✅ `local-setup/`
  
- **Single-word directories**: Use **lowercase**
  - ✅ `coordinator/`
  - ✅ `docs/`

### File Naming
- **Markdown files**: Use **kebab-case**
  - ✅ `class-diagram.md`
  - ✅ `sequence-diagrams.md`
  - ✅ `use-case-diagram.md`
  - ✅ `high-level-design.md`
  - ✅ `low-level-design.md`

## Documentation Structure

```
docs/
├── README.md                    # Documentation index
├── NAMING_CONVENTIONS.md        # Naming conventions guide
├── requirements.md              # System requirements
├── high-level-design.md         # High-level system design
├── api-contracts.md             # Global API contracts
├── use-case-diagram.md          # Global use case diagram
│
├── api-gateway/                 # API Gateway design docs
│   ├── design.md
│   ├── low-level-design.md
│   ├── class-diagram.md
│   └── sequence-diagrams.md
│
├── coordinator/                 # Coordinator design docs
│   ├── design.md
│   ├── low-level-design.md
│   ├── api-contracts.md
│   ├── class-diagram.md
│   └── sequence-diagrams.md
│
└── storage-node/                 # Storage Node design docs
    ├── design.md
    ├── low-level-design.md
    ├── api-contracts.md
    ├── class-diagram.md
    └── sequence-diagrams.md
```

## Service Implementation Structure

Each service follows a standard Go project structure:

```
{service-name}/
├── cmd/                    # Application entry points
├── internal/               # Private application code
├── pkg/                    # Public library code
├── deployments/            # Deployment configurations
│   ├── docker/            # Dockerfiles
│   └── k8s/               # Kubernetes manifests
├── tests/                  # Test suites
│   ├── unit/              # Unit tests
│   └── integration/       # Integration tests
├── config.yaml             # Configuration file
├── Makefile                # Build automation
└── README.md               # Service documentation
```

## Local Setup Structure

```
local-setup/
├── README.md               # Local setup guide
├── deploy.sh               # Main deployment script
├── cleanup.sh              # Cleanup script
├── k8s/                    # Kubernetes manifests
│   ├── namespace.yaml
│   ├── postgres/          # PostgreSQL deployment
│   ├── redis/             # Redis deployment
│   ├── api-gateway/       # API Gateway manifests
│   ├── coordinator/       # Coordinator manifests
│   └── storage-node/      # Storage Node manifests
└── scripts/                # Helper scripts
    ├── build-images.sh
    └── init-db.sh
```

## Refactoring Summary

### Changes Made

1. **Directory Renaming**:
   - `local_setup/` → `local-setup/` (snake_case to kebab-case)

2. **Documentation Updates**:
   - Updated all references to `local_setup` → `local-setup`
   - Fixed duplicate numbering in docs/README.md
   - Created `NAMING_CONVENTIONS.md` for reference

3. **Structure Verification**:
   - ✅ All root directories follow consistent naming
   - ✅ All docs subdirectories follow consistent naming
   - ✅ All markdown files use kebab-case
   - ✅ No implementation folders modified (as requested)

## Production-Ready Standards

The project structure now follows production-ready conventions:

- ✅ **Consistent naming**: All multi-word directories use kebab-case
- ✅ **Clear organization**: Logical grouping of related files
- ✅ **Standard patterns**: Follows Go project conventions
- ✅ **Documentation**: Comprehensive docs with diagrams
- ✅ **Deployment ready**: Includes local and production deployment configs

## Verification

To verify the structure is correct:

```bash
# Check root directories
ls -d */ | grep -E "(api-gateway|coordinator|storage-node|local-setup|docs)"

# Check docs structure
find docs -type d | sort

# Check file naming
find docs -name "*.md" | grep -v README | xargs -I {} basename {} | sort
```

All directories and files should follow the naming conventions outlined above.

