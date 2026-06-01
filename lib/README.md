# Vendored lib packages

This directory contains vendored packages from `github.com/sourcegraph/sourcegraph/lib`.

## Source

- **Repository**: https://github.com/sourcegraph/sourcegraph
- **Commit**: bdc2f4bb8b59f78f4fa8868b2690b673b41948d4
- **Date**: 2026-06-01 07:34:50 +0100

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
