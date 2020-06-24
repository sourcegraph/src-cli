package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/sourcegraph/jsonx"
)

// jsonxToJSON converts jsonx to plain JSON.
func jsonxToJSON(text string) ([]byte, error) {
	data, errs := jsonx.ParseWithDetailedErrors(text, jsonx.ParseOptions{Comments: true, TrailingCommas: true})
	if len(errs) > 0 {
		b := strings.Builder{}
		for _, err := range errs {
			b.WriteByte('\n')
			b.WriteString(renderJsonxParseError(text, err))
		}
		return data, fmt.Errorf("failed to parse JSON: %s", b.String())
	}
	return data, nil
}

// jsonxUnmarshal unmarshals jsonx into Go data.
//
// This process loses comments, trailing commas, formatting, etc.
func jsonxUnmarshal(text string, v interface{}) error {
	data, err := jsonxToJSON(text)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return json.Unmarshal(data, v)
}

type contextLine struct {
	Line    int
	Content string
}

type errorLine struct {
	contextLine
	Offset int
	Length int
}

// renderJsonxParseError renders a jsonx ParseError into a (hopefully) nice,
// human readable format.
func renderJsonxParseError(text string, err jsonx.ParseError) string {
	context := struct {
		Code        string
		StartOffset int
		EndOffset   int
		PreContext  []contextLine
		Error       errorLine
		PostContext []contextLine
	}{
		Code:        err.Code.String(),
		StartOffset: err.Offset,
		EndOffset:   err.Offset + err.Length,
	}

	byteCount := 0
	lines := bytes.Split([]byte(text), []byte("\n"))
	for i, line := range lines {
		if err.Offset <= byteCount+len(line)+1 {
			if i > 0 {
				context.PreContext = []contextLine{
					{
						Line:    i,
						Content: string(lines[i-1]),
					},
				}
			}

			context.Error = errorLine{
				contextLine: contextLine{
					Line:    i + 1,
					Content: string(line),
				},
				Offset: err.Offset - byteCount + 1,
				Length: err.Length,
			}

			if i < len(lines)-1 {
				context.PostContext = []contextLine{
					{
						Line:    i + 2,
						Content: string(lines[i+1]),
					},
				}
			}

			break
		}
		byteCount += len(line) + 1
	}

	b := &strings.Builder{}
	if err := jsonParseErrorTemplate.Execute(b, context); err != nil {
		panic(err)
	}
	return b.String()
}

var jsonParseErrorTemplate *template.Template

func init() {
	var err error

	if jsonParseErrorTemplate, err = parseTemplate(jsonParseErrorTemplateContent); err != nil {
		panic(err)
	}
}

const jsonParseErrorTemplateContent = `
	{{- define "context-line" -}}
		{{- color "search-line-numbers" }}{{ printf "%8d" .Line }}  {{ color "nc" }}{{ .Content }}{{ "\n" -}}
	{{- end -}}

	{{- color "warning" }}{{ .Code }} at bytes {{ .StartOffset }}-{{ .EndOffset }}{{ color "nc" }}{{ "\n" -}}
	{{- range .PreContext -}}
		{{- template "context-line" . -}}
	{{- end -}}
	{{- with .Error -}}
		{{- template "context-line" . -}}
		{{ color "warning" }}{{- printf "%8s" "" }}  {{ pad "^" .Offset " " }}{{ pad "" .Length "^" }}{{ color "nc" }}{{ "\n" -}}
		{{ color "warning" }}{{- printf "%8s" "" }} {{ pad "" .Offset " " }}{{ $.Code }}{{ color "nc" }}{{ "\n" -}}
	{{- end -}}
	{{- range .PostContext -}}
		{{- template "context-line" . -}}
	{{- end -}}
`
