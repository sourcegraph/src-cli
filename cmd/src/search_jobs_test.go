package main

import (
    "testing"
)

func TestSearchJobsCommandRegistration(t *testing.T) {
    // Command registration test
    t.Run("command registration", func(t *testing.T) {
        var found bool
        for _, cmd := range commands {
            if cmd.flagSet.Name() == "search-jobs" {
                found = true
                expectedAliases := []string{"search-job"}
                if len(cmd.aliases) != len(expectedAliases) {
                    t.Errorf("got %d aliases, want %d", len(cmd.aliases), len(expectedAliases))
                }
                for i, alias := range cmd.aliases {
                    if alias != expectedAliases[i] {
                        t.Errorf("got alias %s, want %s", alias, expectedAliases[i])
                    }
                }
                break
            }
        }
        if !found {
            t.Error("search-jobs command not registered")
        }
    })

    // Test subcommands registration
    t.Run("subcommands", func(t *testing.T) {
        expectedCommands := []string{"cancel", "create", "delete", "get", "list"}

        for _, expected := range expectedCommands {
            var found bool
            for _, cmd := range searchJobsCommands {
                if cmd.flagSet.Name() == expected {
                    found = true
                    break
                }
            }
            if !found {
                t.Errorf("subcommand %s not registered", expected)
            }
        }
    })
}
