# Vendored lib packages

This directory contains vendored packages from `github.com/sourcegraph/sourcegraph/lib`.

## Source

- **Repository**: https://github.com/sourcegraph/sourcegraph
- **Commit**: de6bd07264df18d8dc66e2aebeaad18ac504390f
- **Date**: 2026-06-11 13:59:53 +0000

## Updating

To update these vendored packages, run:

```bash
./dev/vendor-lib.sh
```

The script will:
1. Validate that `../sourcegraph` is on the `main` branch with no uncommitted changes
2. Discover all direct and transitive dependencies on `lib` packages
3. Copy only the needed packages
4. Update this README with the new commit information
