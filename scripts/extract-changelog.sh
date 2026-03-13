#!/bin/bash
# Extract the changelog section for a specific version from CHANGELOG.md
# Usage: extract-changelog.sh <version>

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version>"
    exit 1
fi

# Remove 'v' prefix if present
VERSION=${VERSION#v}

# Extract the section for this version
# Matches headings like: ## v0.0.2 (2026-03-13)
awk -v version="$VERSION" '
/^## v/ {
    if (found) exit
    if ($0 ~ "v" version) {
        found=1
        next
    }
}
found && /^## v/ { exit }
found { print }
' CHANGELOG.md
