package servegit

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

const testAddress = "test.local:3939"

var discardLogger = log.New(io.Discard, "", log.LstdFlags)

func TestReposHandler(t *testing.T) {
	cases := []struct {
		name  string
		repos []string
	}{{
		name: "empty",
	}, {
		name:  "simple",
		repos: []string{"project1", "project2"},
	}, {
		name:  "nested",
		repos: []string{"project1", "project2", "dir/project3", "dir/project4.bare"},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			root := gitInitRepos(t, tc.repos...)

			rootFS, err := os.OpenRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { rootFS.Close() })

			h := (&Serve{
				Info:   testLogger(t),
				Debug:  discardLogger,
				Addr:   testAddress,
				Root:   root,
				RootFS: rootFS,
			}).handler()

			var want []Repo
			for _, name := range tc.repos {
				isBare := strings.HasSuffix(name, ".bare")
				uri := path.Join("/repos", name)
				clonePath := uri
				if !isBare {
					clonePath += "/.git"
				}
				want = append(want, Repo{Name: name, URI: uri, ClonePath: clonePath})

			}
			testReposHandler(t, h, want)
		})
	}
}

func TestHandlerRejectsGitdirFile(t *testing.T) {
	root := t.TempDir()
	servedRoot := filepath.Join(root, "served")
	publicRepo := filepath.Join(servedRoot, "public")
	privateRepo := filepath.Join(root, "private")
	for _, repo := range []string{publicRepo, privateRepo} {
		if err := os.MkdirAll(repo, 0755); err != nil {
			t.Fatal(err)
		}
	}
	gitInit(t, publicRepo)
	gitInit(t, privateRepo)

	pointer := filepath.Join(publicRepo, "gitdir-pointer")
	if err := os.WriteFile(pointer, []byte("gitdir: ../../private/.git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, publicRepo, "git", "add", "gitdir-pointer")
	runCmd(t, publicRepo, "git", "commit", "-m", "add gitdir pointer")
	runCmd(t, privateRepo, "git", "commit", "--allow-empty", "-m", "private commit")
	// Git accepts this ordinary tracked file as a repository and follows its
	// gitdir pointer outside servedRoot.
	runCmd(t, publicRepo, "git", "upload-pack", "--strict", "--advertise-refs", pointer)

	rootFS, err := os.OpenRoot(servedRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rootFS.Close() })

	h := (&Serve{
		Info:   testLogger(t),
		Debug:  discardLogger,
		Addr:   testAddress,
		Root:   servedRoot,
		RootFS: rootFS,
	}).handler()
	req := httptest.NewRequest(http.MethodGet, "/repos/public/gitdir-pointer/info/refs?service=git-upload-pack", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("gitdir file status = %d, want %d; body: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandlerServesOnlyListedRepositories(t *testing.T) {
	root := t.TempDir()
	privateRepo := filepath.Join(root, "private")
	if err := os.MkdirAll(privateRepo, 0755); err != nil {
		t.Fatal(err)
	}
	runCmd(t, privateRepo, "git", "init", "-b", "main")
	runCmd(t, privateRepo, "git", "commit", "--allow-empty", "-m", "private commit")
	out, err := exec.Command("git", "-C", privateRepo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	privateHead := strings.TrimSpace(string(out))

	// A repository whose working tree has a directory that Git accepts as a
	// repository, made of a tracked HEAD and symlinks to the objects and refs
	// of the private repository outside the served root.
	attacker := filepath.Join(root, "attacker")
	if err := os.MkdirAll(filepath.Join(attacker, "evil"), 0755); err != nil {
		t.Fatal(err)
	}
	runCmd(t, attacker, "git", "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(attacker, "evil", "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"objects", "refs"} {
		if err := os.Symlink("../../../private/.git/"+name, filepath.Join(attacker, "evil", name)); err != nil {
			t.Fatal(err)
		}
	}
	runCmd(t, attacker, "git", "add", "evil")
	runCmd(t, attacker, "git", "commit", "-m", "evil")

	served := filepath.Join(root, "served")
	runCmd(t, root, "git", "clone", attacker, filepath.Join(served, "public"))
	runCmd(t, root, "git", "init", "--bare", filepath.Join(served, "dir", "project.bare"))
	// Git serves the private repository's refs from the embedded directory.
	runCmd(t, served, "git", "upload-pack", "--strict", "--advertise-refs", filepath.Join(served, "public", "evil"))

	newHandler := func(t *testing.T, root string) http.Handler {
		rootFS, err := os.OpenRoot(root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { rootFS.Close() })
		return (&Serve{
			Info:   testLogger(t),
			Debug:  discardLogger,
			Addr:   testAddress,
			Root:   root,
			RootFS: rootFS,
		}).handler()
	}
	get := func(h http.Handler, target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		return rec
	}

	h := newHandler(t, served)
	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/repos/public/.git/info/refs?service=git-upload-pack", want: http.StatusOK},
		{path: "/repos/dir/project.bare/info/refs?service=git-upload-pack", want: http.StatusOK},
		{path: "/repos/public/evil/info/refs?service=git-upload-pack", want: http.StatusNotFound},
		{path: "/repos/public/evil/git-upload-pack", want: http.StatusNotFound},
		{path: "/repos/dir/project.bare/objects/info/refs?service=git-upload-pack", want: http.StatusNotFound},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := get(h, tc.path)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tc.want, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), privateHead) {
				t.Error("the response has the private repository's HEAD")
			}
		})
	}

	t.Run("a root that is a repository serves only itself", func(t *testing.T) {
		h := newHandler(t, filepath.Join(served, "public"))
		if rec := get(h, "/repos/.git/info/refs?service=git-upload-pack"); rec.Code != http.StatusOK {
			t.Errorf("root repository status = %d, want %d", rec.Code, http.StatusOK)
		}
		if rec := get(h, "/repos/evil/info/refs?service=git-upload-pack"); rec.Code != http.StatusNotFound {
			t.Errorf("nested repository status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

func testReposHandler(t *testing.T, h http.Handler, repos []Repo) {
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	get := func(path string) string {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if testing.Verbose() {
			t.Logf("GET %s:\n%s", path, b)
		}
		return string(b)
	}

	// Check we have some known strings on the index page
	index := get("/")
	for _, sub := range []string{"http://" + testAddress, "/v1/list-repos", "/repos/"} {
		if !strings.Contains(index, sub) {
			t.Errorf("index page does not contain substring %q", sub)
		}
	}

	// Directory browsing under /repos/ is intentionally disabled to avoid
	// exposing arbitrary files (VULN-99). Only smart Git HTTP endpoints are
	// served; anything else returns 404.
	if res, err := http.Get(ts.URL + "/repos/"); err != nil {
		t.Fatal(err)
	} else {
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("GET /repos/ status = %d, want %d", res.StatusCode, http.StatusNotFound)
		}
	}

	// check our API response
	type Response struct{ Items []Repo }
	var want, got Response
	want.Items = repos
	if err := json.Unmarshal([]byte(get("/v1/list-repos")), &got); err != nil {
		t.Fatal(err)
	}
	opts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b Repo) bool { return a.Name < b.Name }),
	}
	if !cmp.Equal(want, got, opts...) {
		t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(want, got, opts...))
	}
}

func gitInitBare(t *testing.T, path string) {
	if err := exec.Command("git", "init", "--bare", path).Run(); err != nil {
		t.Fatal(err)
	}
}

func gitInit(t *testing.T, path string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}

func gitInitRepos(t *testing.T, names ...string) string {
	root := t.TempDir()

	// We cannot os.OpenRoot on a non-existent dir so we return tmpdir
	if len(names) == 0 {
		return root
	}

	root = filepath.Join(root, "repos-root")

	for _, name := range names {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}

		if strings.HasSuffix(p, ".bare") {
			gitInitBare(t, p)
		} else {
			gitInit(t, p)
		}
	}

	return root
}

func TestReposRelativeRoot(t *testing.T) {
	root := gitInitRepos(t, "project1", "dir/project2.bare")
	t.Chdir(filepath.Dir(root))
	rel := filepath.Base(root)

	rootFS, err := os.OpenRoot(rel)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rootFS.Close() })

	repos, err := (&Serve{
		Info:   testLogger(t),
		Debug:  discardLogger,
		Root:   rel,
		RootFS: rootFS,
	}).Repos()
	if err != nil {
		t.Fatal(err)
	}
	want := []Repo{
		{Name: "dir/project2.bare", URI: "/repos/dir/project2.bare", ClonePath: "/repos/dir/project2.bare"},
		{Name: "project1", URI: "/repos/project1", ClonePath: "/repos/project1/.git"},
	}
	if diff := cmp.Diff(want, repos); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestIgnoreGitSubmodules(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "dir"), os.ModePerm); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "dir", ".git"), []byte("ignore me please"), os.ModePerm); err != nil {
		t.Fatal(err)
	}

	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rootFS.Close() })

	repos, err := (&Serve{
		Info:   testLogger(t),
		Debug:  discardLogger,
		Root:   root,
		RootFS: rootFS,
	}).Repos()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected no repos, got %v", repos)
	}
}

func TestIsBareRepo(t *testing.T) {
	dir := t.TempDir()

	gitInitBare(t, dir)

	if !isBareRepo(dir) {
		t.Errorf("Path %s it not a bare repository", dir)
	}
}

func TestEmptyDirIsNotBareRepo(t *testing.T) {
	dir := t.TempDir()

	if isBareRepo(dir) {
		t.Errorf("Path %s it falsey detected as a bare repository", dir)
	}
}

func testLogger(t *testing.T) *log.Logger {
	return log.New(testWriter{t}, "testLogger ", log.LstdFlags)
}

type testWriter struct {
	*testing.T
}

func (tw testWriter) Write(p []byte) (n int, err error) {
	tw.Log(string(p))
	return len(p), nil
}
