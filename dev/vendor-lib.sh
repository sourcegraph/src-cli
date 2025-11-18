#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SRC_REPO="$ROOT_DIR/../sourcegraph"
SRC_LIB="$SRC_REPO/lib"
DEST_LIB="$ROOT_DIR/lib"

# Validate source repository
echo "Validating source repository..."
cd "$SRC_REPO"

# Check if on main branch
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$CURRENT_BRANCH" != "main" ]; then
    echo "Error: Source repository is not on main branch (currently on: $CURRENT_BRANCH)"
    exit 1
fi

# Check for uncommitted changes
if ! git diff-index --quiet HEAD -- ; then
    echo "Error: Source repository has uncommitted changes"
    git status --short
    exit 1
fi

# Get commit info
COMMIT_HASH=$(git rev-parse HEAD)
COMMIT_DATE=$(git show -s --format=%ci HEAD)

echo "Source repository validated:"
echo "  Branch: $CURRENT_BRANCH"
echo "  Commit: $COMMIT_HASH"
echo "  Date:   $COMMIT_DATE"

cd "$ROOT_DIR"

echo ""
echo "Discovering packages to vendor..."

# Get direct imports (including test imports)
DIRECT_IMPORTS=$(go list -f '{{ join .Imports "\n" }}
{{ join .TestImports "\n" }}
{{ join .XTestImports "\n" }}' ./... | grep 'github.com/sourcegraph/sourcegraph/lib' | sort | uniq || true)

if [ -z "$DIRECT_IMPORTS" ]; then
    echo "Error: No lib imports found"
    exit 1
fi

echo "Direct imports found (including test imports):"
echo "$DIRECT_IMPORTS"

# Get transitive dependencies within lib
echo ""
echo "Finding transitive dependencies..."
cd "$SRC_REPO"
TRANSITIVE=$(echo "$DIRECT_IMPORTS" | xargs go list -f '{{ join .Deps "\n" }}' 2>/dev/null | grep 'github.com/sourcegraph/sourcegraph/lib' | sort | uniq || true)
cd "$SCRIPT_DIR"

# Combine and deduplicate
ALL_PACKAGES=$(echo -e "$DIRECT_IMPORTS\n$TRANSITIVE" | sort | uniq)

echo ""
echo "All packages to vendor:"
echo "$ALL_PACKAGES"

# Remove existing lib directory
if [ -d "$DEST_LIB" ]; then
    echo ""
    echo "Removing existing $DEST_LIB directory..."
    rm -rf "$DEST_LIB"
fi

# Create lib directory
mkdir -p "$DEST_LIB"

# Copy each package
echo ""
echo "Copying packages..."
for pkg in $ALL_PACKAGES; do
    # Extract the path after github.com/sourcegraph/sourcegraph/lib/
    pkg_path=${pkg#github.com/sourcegraph/sourcegraph/lib/}

    src_dir="$SRC_LIB/$pkg_path"
    dest_dir="$DEST_LIB/$pkg_path"

    if [ ! -d "$src_dir" ]; then
        echo "Warning: Source directory not found: $src_dir"
        continue
    fi

    echo "  $pkg_path"
    mkdir -p "$dest_dir"

    # Copy Go files (excluding tests) and other relevant files
    for gofile in "$src_dir"/*.go; do
        [ -e "$gofile" ] || continue
        if [[ ! "$gofile" =~ _test\.go$ ]]; then
            cp "$gofile" "$dest_dir/" 2>/dev/null || true
        fi
    done
    cp -r "$src_dir"/*.mod "$dest_dir/" 2>/dev/null || true
    cp -r "$src_dir"/*.sum "$dest_dir/" 2>/dev/null || true

    # Copy any subdirectories that might contain embedded files or test data
    for item in "$src_dir"/*; do
        if [ -d "$item" ] && [ "$(basename "$item")" != "." ] && [ "$(basename "$item")" != ".." ]; then
            subdir=$(basename "$item")
            # Skip if it's a package we're already copying separately
            if ! echo "$ALL_PACKAGES" | grep -q "lib/$pkg_path/$subdir"; then
                if ls "$item"/*.go 2>/dev/null | grep -v '_test\.go$' >/dev/null 2>&1; then
                    # Contains non-test Go files, skip it (it's a separate package)
                    continue
                fi
                # Copy non-package directories (e.g., testdata, embedded files)
                cp -r "$item" "$dest_dir/" 2>/dev/null || true
            fi
        fi
    done
done

# Create go.mod file
echo ""
echo "Creating go.mod file..."
cat > "$DEST_LIB/go.mod" <<EOF
module github.com/sourcegraph/sourcegraph/lib

go 1.23
EOF

# Create README.md
echo "Creating README.md file..."
cat > "$DEST_LIB/README.md" <<EOF
# Vendored lib packages

This directory contains vendored packages from \`github.com/sourcegraph/sourcegraph/lib\`.

## Source

- **Repository**: https://github.com/sourcegraph/sourcegraph
- **Commit**: $COMMIT_HASH
- **Date**: $COMMIT_DATE

## Updating

To update these vendored packages, run:

\`\`\`bash
./dev/vendor-lib.sh
\`\`\`

The script will:
1. Validate that \`../sourcegraph\` is on the \`main\` branch with no uncommitted changes
2. Discover all direct and transitive dependencies on \`lib\` packages
3. Copy only the needed packages
4. Update this README with the new commit information
EOF

echo ""
echo "Vendoring complete! Packages copied to $DEST_LIB"

cd "$DEST_LIB"
go mod tidy
