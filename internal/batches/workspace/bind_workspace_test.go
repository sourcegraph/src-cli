var repo = &graphql.Repository{
	ID:            "src-cli",
	Name:          "github.com/sourcegraph/src-cli",
	DefaultBranch: &graphql.Branch{Name: "main", Target: graphql.Target{OID: "d34db33f"}},
}
func zipUpFiles(t *testing.T, dir string, files map[string]string) string {
	f, err := ioutil.TempFile(dir, "repo-zip-*")
	for name, body := range files {
	return archivePath
}

func workspaceTmpDir(t *testing.T) string {
	testTempDir, err := ioutil.TempDir("", "bind-workspace-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(testTempDir) })

	return testTempDir
}

func TestDockerBindWorkspaceCreator_Create(t *testing.T) {
	// Create a zip file for all the other tests to use.
	fakeFilesTmpDir := workspaceTmpDir(t)
	filesInZip := map[string]string{
		"README.md": "# Welcome to the README\n",
	}
	archivePath := zipUpFiles(t, fakeFilesTmpDir, filesInZip)

func TestDockerBindWorkspace_ApplyDiff(t *testing.T) {
	// Create a zip file for all the other tests to use.
	fakeFilesTmpDir := workspaceTmpDir(t)
	filesInZip := map[string]string{
		"README.md": "# Welcome to the README\n",
	}
	archivePath := zipUpFiles(t, fakeFilesTmpDir, filesInZip)

	t.Run("success", func(t *testing.T) {
		diff := `diff --git README.md README.md
index 02a19af..a84667f 100644
--- README.md
+++ README.md
@@ -1 +1,3 @@
 # Welcome to the README
+
+This is a new line
diff --git new-file.txt new-file.txt
new file mode 100644
index 0000000..7bb2542
--- /dev/null
+++ new-file.txt
@@ -0,0 +1,2 @@
+check this out. this is a new file.
+written on a computer. what a blast.
`

		wantFiles := map[string]string{
			"README.md":    "# Welcome to the README\n\nThis is a new line\n",
			"new-file.txt": "check this out. this is a new file.\nwritten on a computer. what a blast.\n",
		}
		testTempDir := workspaceTmpDir(t)

		archive := &fakeRepoArchive{mockPath: archivePath}
		creator := &dockerBindWorkspaceCreator{Dir: testTempDir}
		workspace, err := creator.Create(context.Background(), repo, nil, archive)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		err = workspace.ApplyDiff(context.Background(), []byte(diff))
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		haveFiles, err := readWorkspaceFiles(workspace)
		if err != nil {
			t.Fatalf("error walking workspace: %s", err)
		}

		if !cmp.Equal(wantFiles, haveFiles) {
			t.Fatalf("wrong files in workspace:\n%s", cmp.Diff(wantFiles, haveFiles))
		}
	})

	t.Run("failure", func(t *testing.T) {
		diff := `lol this is not a diff but the computer doesn't know it yet, watch`

		testTempDir := workspaceTmpDir(t)

		archive := &fakeRepoArchive{mockPath: archivePath}
		creator := &dockerBindWorkspaceCreator{Dir: testTempDir}
		workspace, err := creator.Create(context.Background(), repo, nil, archive)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		err = workspace.ApplyDiff(context.Background(), []byte(diff))
		if err == nil {
			t.Fatalf("error is nil")
		}
	})
}
