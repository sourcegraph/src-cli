package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/sourcegraph/scip/bindings/go/scip"
)

var exampleSCIPIndex = scip.Index{
	Metadata: &scip.Metadata{
		TextDocumentEncoding: scip.TextEncoding_UTF8,
		ToolInfo: &scip.ToolInfo{
			Name:    "hello",
			Version: "1.0.0",
		},
	},
}

func exampleSCIPBytes(t *testing.T) []byte {
	bytes, err := proto.Marshal(&exampleSCIPIndex)
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

func createTempSCIPFile(t *testing.T, scipFileName string) string {
	t.Helper()
	dir := t.TempDir()
	require.NotEqual(t, "", scipFileName)
	scipFilePath := filepath.Join(dir, scipFileName)
	err := os.WriteFile(scipFilePath, exampleSCIPBytes(t), 0755)
	require.NoError(t, err)
	return scipFilePath
}

func TestInferIndexerNameAndVersion(t *testing.T) {
	name, version, err := readIndexerNameAndVersion(createTempSCIPFile(t, "index.scip"))
	require.NoError(t, err)
	require.Equal(t, "hello", name)
	require.Equal(t, "1.0.0", version)
}
