package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var _ = func() bool {
	// Disable colordiff when testing because its output various from system to system(!)
	os.Setenv("COLORDIFF", "false")

	isTest = true
	return true
}()

func TestSearchOutput(t *testing.T) {
	type testT struct {
		input *searchResultsImproved
		want  *string
	}

	tests := map[string]*testT{}

	dataDir := "testdata/search_formatting"
	infos, err := ioutil.ReadDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range infos {
		path := filepath.Join(dataDir, f.Name())
		if strings.HasSuffix(f.Name(), ".got.txt") {
			os.Remove(path)
		}

		isTestInput := strings.HasSuffix(f.Name(), ".test.json")
		isTestResult := strings.HasSuffix(f.Name(), ".want.txt")
		if !isTestInput && !isTestResult {
			continue
		}
		testName := strings.TrimSuffix(f.Name(), ".test.json")
		testName = strings.TrimSuffix(testName, ".want.txt")
		if _, ok := tests[testName]; !ok {
			tests[testName] = &testT{}
		}

		data, err := ioutil.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if isTestInput {
			if err := json.Unmarshal(data, &tests[testName].input); err != nil {
				t.Fatal(err)
			}
		} else {
			tmp := string(data)
			tests[testName].want = &tmp
		}
	}

	for testName, tst := range tests {
		if tst.input == nil {
			t.Fatalf("mytest.want.txt exists, but mytest.test.json file doesn't")
			if tst.want == nil {
				t.Fatalf("test is missing a .want.txt file, please create an empty one")
			}
		}
		if tst.want == nil {
			// Create the initial (empty) .want.txt file.
			wantFile := filepath.Join(dataDir, testName+".want.txt")
			if err := ioutil.WriteFile(wantFile, nil, 0600); err != nil {
				t.Fatal(err)
			}
			tmp := ""
			tst.want = &tmp
		}
	}

	for testName, tst := range tests {
		t.Run(testName, func(t *testing.T) {
			tmpl, err := parseTemplate(searchResultsTemplate)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, tst.input); err != nil {
				t.Fatal(err)
			}
			got := buf.String()
			if got != *tst.want {
				t.Logf("'%s.want.txt' does not match '%s.got.txt'", testName, testName)
				gotFile := filepath.Join(dataDir, testName+".got.txt")
				wantFile := filepath.Join(dataDir, testName+".want.txt")

				err := ioutil.WriteFile(gotFile, []byte(got), 0600)
				if err != nil {
					t.Fatal(err)
				}

				cmd := exec.Command("git", "diff", "--no-index", wantFile, gotFile)
				out, _ := cmd.CombinedOutput()
				t.Fatalf("\n%s\nTo accept these changes, run:\n$ mv %s %s", string(out), gotFile, wantFile)
			}
		})
	}
}
