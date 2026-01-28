---
title: "Api Reference Generation"
type: docs
weight: 6
description: Generating Porch API Reference Documentation
---

# Generating Porch API Reference Documentation

This guide explains how to automatically generate API reference documentation for Porch using the Elastic `crd-ref-docs` tool.

## Overview

The Porch project uses `crd-ref-docs` to generate markdown documentation from Go source code. This documentation is then integrated into the Hugo-based documentation site.

## Prerequisites

- Go 1.25+ installed
- Access to the Porch repository
- Basic familiarity with command line

## Step 1: Install crd-ref-docs

Install the tool using Go:

```bash
go install github.com/elastic/crd-ref-docs@latest
```

Verify installation:

```bash
crd-ref-docs --help
```

The binary will be installed in `$(go env GOPATH)/bin/crd-ref-docs` (typically `~/go/bin/crd-ref-docs`).

## Step 2: Create Configuration File

Create a configuration file `crd-ref-docs-config.yaml` in the Porch root directory:

```yaml
processor:
  # Exclude List types from documentation
  ignoreTypes:
    - ".*List$"
    # Exclude OCI-related types (not supported in Porch)
    - "OciPackage$"
    - "OciRepository$"
  # Exclude common Kubernetes metadata fields
  ignoreFields:
    - "TypeMeta$"
    - "ObjectMeta$"
    - "ListMeta$"
    # Exclude OCI-related fields (not supported in Porch)
    - "^oci$"

render:
  # Kubernetes version for API documentation links
  kubernetesVersion: 1.28
```

**Configuration Options:**

- `processor.ignoreTypes`: Regular expressions for types to exclude (e.g., List types)
- `processor.ignoreFields`: Regular expressions for fields to exclude (e.g., standard Kubernetes metadata)
- `render.kubernetesVersion`: Version used for generating links to Kubernetes API docs

## Step 3: Generate Documentation

Navigate to the Porch repository root:

```bash
cd /path/to/porch
```

### Generate porch.kpt.dev/v1alpha1 API Documentation

```bash
crd-ref-docs \
    --source-path=./api/porch/v1alpha1 \
    --config=crd-ref-docs-config.yaml \
    --renderer=markdown \
    --output-path=porch-api-v1alpha1-docs.md
```

### Generate config.porch.kpt.dev/v1alpha1 API Documentation

```bash
crd-ref-docs \
    --source-path=./api/porchconfig/v1alpha1 \
    --config=crd-ref-docs-config.yaml \
    --renderer=markdown \
    --output-path=porchconfig-api-v1alpha1-docs.md
```

**Command Options:**

- `--source-path`: Path to the Go package containing API types
- `--config`: Path to the configuration YAML file
- `--renderer`: Output format (`markdown` or `asciidoctor`)
- `--output-path`: Where to save the generated documentation

## Step 4: Integrate with Hugo Documentation

The generated markdown files need to be integrated into the Hugo documentation site.

### Combining the Generated Files

The two generated files need to be combined into a single Hugo page with proper front matter.

#### Manual Merge (Recommended for first time)

1. **Backup the existing file** (if it exists):
   ```bash
   cp docs/content/en/docs/7_cli_api/api-ref.md docs/content/en/docs/7_cli_api/api-ref.md.backup
   ```

2. **Create the combined file** with Hugo front matter:
   ```bash
   cat > docs/content/en/docs/7_cli_api/api-ref.md << 'EOF'
   ---
   title: "API Reference"
   type: docs
   weight: 2
   description: API reference documentation for Porch resources
   ---
   
   # API Reference
   
   ## Packages
   - [porch.kpt.dev/v1alpha1](#porchkptdevv1alpha1)
   - [config.porch.kpt.dev/v1alpha1](#configporchkptdevv1alpha1)
   
   ---
   
   EOF
   ```

3. **Append the porch.kpt.dev API** (skip the header):
   ```bash
   tail -n +3 porch-api-v1alpha1-docs.md >> docs/content/en/docs/7_cli_api/api-ref.md
   ```

4. **Add a separator**:
   ```bash
   echo -e "\n---\n" >> docs/content/en/docs/7_cli_api/api-ref.md
   ```

5. **Append the config.porch.kpt.dev API** (skip the header):
   ```bash
   tail -n +3 porchconfig-api-v1alpha1-docs.md >> docs/content/en/docs/7_cli_api/api-ref.md
   ```

#### One-Command Merge

For a quick one-liner:

```bash
{ echo '---'; echo 'title: "API Reference"'; echo 'type: docs'; echo 'weight: 2'; echo 'description: API reference documentation for Porch resources'; echo '---'; echo ''; echo '# API Reference'; echo ''; echo '## Packages'; echo '- [porch.kpt.dev/v1alpha1](#porchkptdevv1alpha1)'; echo '- [config.porch.kpt.dev/v1alpha1](#configporchkptdevv1alpha1)'; echo ''; echo '---'; echo ''; tail -n +3 porch-api-v1alpha1-docs.md; echo ''; echo '---'; echo ''; tail -n +3 porchconfig-api-v1alpha1-docs.md; } > docs/content/en/docs/7_cli_api/api-ref.md
```

### Manual Integration

If you prefer manual control:

1. Open `docs/content/en/docs/7_cli_api/api-ref.md` in your editor
2. Keep the Hugo front matter at the top
3. Copy content from `porch-api-v1alpha1-docs.md` (skip the "# API Reference" header)
4. Add a separator: `---`
5. Copy content from `porchconfig-api-v1alpha1-docs.md` (skip the "# API Reference" header)
6. Save the file

### File Structure

```
porch/
├── api/
│   ├── porch/v1alpha1/              # Source: porch.kpt.dev API types
│   └── porchconfig/v1alpha1/        # Source: config.porch.kpt.dev API types
├── docs/
│   └── content/en/docs/7_cli_api/
│       └── api-ref.md               # Target: Combined API documentation
├── crd-ref-docs-config.yaml         # Configuration file
├── porch-api-v1alpha1-docs.md       # Generated (intermediate)
└── porchconfig-api-v1alpha1-docs.md # Generated (intermediate)
```

## Step 5: View Documentation

Start the Hugo development server:

```bash
cd docs
hugo server
```

Navigate to `http://localhost:1313/docs/7_cli_api/api-ref/` to view the generated API reference.

## Updating Documentation

When API types change, regenerate the documentation:

```bash
# From the Porch root directory
crd-ref-docs --source-path=./api/porch/v1alpha1 --config=crd-ref-docs-config.yaml --renderer=markdown --output-path=porch-api-v1alpha1-docs.md

crd-ref-docs --source-path=./api/porchconfig/v1alpha1 --config=crd-ref-docs-config.yaml --renderer=markdown --output-path=porchconfig-api-v1alpha1-docs.md

# Then update the Hugo documentation file
# docs/content/en/docs/7_cli_api/api-ref.md
```

## Automation Options

### Makefile Target

Add to your `Makefile`:

```makefile
.PHONY: generate-api-docs
generate-api-docs:
	@echo "Generating API reference documentation..."
	crd-ref-docs \
		--source-path=./api/porch/v1alpha1 \
		--config=crd-ref-docs-config.yaml \
		--renderer=markdown \
		--output-path=porch-api-v1alpha1-docs.md
	crd-ref-docs \
		--source-path=./api/porchconfig/v1alpha1 \
		--config=crd-ref-docs-config.yaml \
		--renderer=markdown \
		--output-path=porchconfig-api-v1alpha1-docs.md
	@echo "Documentation generated. Update docs/content/en/docs/7_cli_api/api-ref.md"
```

Usage:

```bash
make generate-api-docs
```

### Shell Script

Create `scripts/generate-api-docs.sh`:

```bash
#!/bin/bash
set -e

echo "Generating Porch API documentation..."

crd-ref-docs \
    --source-path=./api/porch/v1alpha1 \
    --config=crd-ref-docs-config.yaml \
    --renderer=markdown \
    --output-path=porch-api-v1alpha1-docs.md

crd-ref-docs \
    --source-path=./api/porchconfig/v1alpha1 \
    --config=crd-ref-docs-config.yaml \
    --renderer=markdown \
    --output-path=porchconfig-api-v1alpha1-docs.md

echo "✓ Documentation generated successfully"
echo "  - porch-api-v1alpha1-docs.md"
echo "  - porchconfig-api-v1alpha1-docs.md"
echo ""
echo "Next: Update docs/content/en/docs/7_cli_api/api-ref.md"
```

Make it executable:

```bash
chmod +x scripts/generate-api-docs.sh
./scripts/generate-api-docs.sh
```

## Troubleshooting

### Tool Not Found

If `crd-ref-docs` is not found, ensure `$(go env GOPATH)/bin` is in your PATH:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Go Module Issues

If you encounter Go module errors, ensure dependencies are up to date:

```bash
go mod download
go mod tidy
```

### Empty Output

If the generated documentation is empty, verify:
- The source path contains Go files with exported types
- Types have proper godoc comments
- The package is properly structured

## Additional Resources

- [crd-ref-docs GitHub Repository](https://github.com/elastic/crd-ref-docs)
- [Porch API Source Code](https://github.com/nephio-project/porch/tree/main/api)
- [Hugo Documentation](https://gohugo.io/documentation/)
