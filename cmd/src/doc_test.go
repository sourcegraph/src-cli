package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// legacyGroupsWithoutSubcommandPages lists legacy command groups that are
// deliberately left out of the commanders map in doc.go, so they render as a
// single page instead of a directory of subcommand pages. Every entry needs a
// reason.
var legacyGroupsWithoutSubcommandPages = map[string]string{}

// expectedDocFiles is the full set of files 'src doc' is expected to write,
// relative to the output directory. Update it deliberately when adding or
// removing commands; sourcegraph/sourcegraph's doc/cli/references/BUILD.bazel
// OUTPUT_FILES must be kept in sync with this list.
var expectedDocFiles = []string{
	"abc/index.md",
	"abc/variables/delete.md",
	"abc/variables/index.md",
	"abc/variables/set.md",
	"api.md",
	"auth/index.md",
	"auth/token.md",
	"batch/apply.md",
	"batch/exec.md",
	"batch/index.md",
	"batch/new.md",
	"batch/preview.md",
	"batch/remote.md",
	"batch/repositories.md",
	"batch/validate.md",
	"code-intel/index.md",
	"code-intel/upload.md",
	"codeowners/create.md",
	"codeowners/delete.md",
	"codeowners/get.md",
	"codeowners/index.md",
	"codeowners/update.md",
	"config/edit.md",
	"config/get.md",
	"config/index.md",
	"config/list.md",
	"debug/compose.md",
	"debug/index.md",
	"debug/kube.md",
	"debug/server.md",
	"extsvc/create.md",
	"extsvc/edit.md",
	"extsvc/index.md",
	"extsvc/list.md",
	"index.md",
	"login.md",
	"lsp.md",
	"orgs/create.md",
	"orgs/delete.md",
	"orgs/get.md",
	"orgs/index.md",
	"orgs/list.md",
	"orgs/members/add.md",
	"orgs/members/index.md",
	"orgs/members/remove.md",
	"repos/add-metadata.md",
	"repos/delete-metadata.md",
	"repos/delete.md",
	"repos/get.md",
	"repos/index.md",
	"repos/list.md",
	"repos/update-metadata.md",
	"search-jobs/cancel.md",
	"search-jobs/create.md",
	"search-jobs/delete.md",
	"search-jobs/get.md",
	"search-jobs/index.md",
	"search-jobs/list.md",
	"search-jobs/logs.md",
	"search-jobs/restart.md",
	"search-jobs/results.md",
	"search.md",
	"serve-git.md",
	"snapshot/databases.md",
	"snapshot/index.md",
	"snapshot/restore.md",
	"snapshot/summary.md",
	"snapshot/test.md",
	"snapshot/upload.md",
	"users/create.md",
	"users/delete.md",
	"users/get.md",
	"users/index.md",
	"users/list.md",
	"users/prune.md",
	"users/tag.md",
	"version.md",
}

func runDocCommand(t *testing.T) (dir string, files []string) {
	t.Helper()

	var docCmd *command
	for _, cmd := range commands {
		if cmd.flagSet.Name() == "doc" {
			docCmd = cmd
			break
		}
	}
	if docCmd == nil {
		t.Fatal("'doc' command not registered")
	}

	dir = t.TempDir()
	if err := docCmd.handler([]string{"-o", dir}); err != nil {
		t.Fatalf("src doc: %v", err)
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return dir, files
}

func TestDocGeneratesExpectedFiles(t *testing.T) {
	_, got := runDocCommand(t)

	want := append([]string(nil), expectedDocFiles...)
	sort.Strings(want)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("generated file list mismatch (-want +got):\n%s\n"+
			"If you added or removed a command, update expectedDocFiles here and "+
			"OUTPUT_FILES in sourcegraph/sourcegraph doc/cli/references/BUILD.bazel.", diff)
	}
}

// A legacy command group that is not registered in the commanders map in
// doc.go is rendered as a single leaf page containing only its group help,
// and none of its subcommands get a page. Legacy group help text lists
// subcommands under "The commands are:", so a leaf page containing that
// phrase means a group was missed.
func TestDocLegacyGroupsHaveSubcommandPages(t *testing.T) {
	dir, files := runDocCommand(t)

	for _, rel := range files {
		if strings.HasSuffix(rel, "/index.md") || rel == "index.md" {
			continue
		}
		if _, ok := legacyGroupsWithoutSubcommandPages[rel]; ok {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "The commands are:") {
			t.Errorf("%s is a leaf page but lists subcommands: add its command group to the commanders map in doc.go "+
				"(or, if intentional, to legacyGroupsWithoutSubcommandPages with a reason)", rel)
		}
	}

	for rel := range legacyGroupsWithoutSubcommandPages {
		if !slices.Contains(files, rel) {
			t.Errorf("legacyGroupsWithoutSubcommandPages entry %q was not generated; remove the stale entry", rel)
		}
	}
}

// The root index must link every top-level command, both legacy (commander)
// and migrated (urfave/cli) ones.
func TestDocRootIndexListsAllCommands(t *testing.T) {
	dir, _ := runDocCommand(t)

	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}

	var missing []string
	for _, cmd := range commands {
		name := cmd.flagSet.Name()
		if name == "doc" || name == "publish" {
			continue
		}
		if !strings.Contains(string(index), "[`"+name+"`](") {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("root index.md is missing legacy commands: %v", missing)
	}
}
