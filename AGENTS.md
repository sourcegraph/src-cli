# Command Guidelines

- When adding new commands, use `urfave/cli` instead of the legacy `commander` pattern.
- Register new `urfave/cli` commands in the `migratedCommands` map in `cmd/src/run_migration_compat.go`.
