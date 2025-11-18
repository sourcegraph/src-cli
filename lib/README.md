# Vendored lib packages

This directory contains vendored packages from `github.com/sourcegraph/sourcegraph/lib`.

## Source

- **Repository**: https://github.com/sourcegraph/sourcegraph
- **Commit**: 2ee2b8e77de9663b08ce5f6e5a2c7d2217ce721a
- **Date**: 2025-11-17 19:49:42 -0800

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
