package mcp

import (
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

var _ flag.Value = (*strSliceFlag)(nil)

type strSliceFlag struct {
	vals []string
}

func (s *strSliceFlag) Set(v string) error {
	// The MCP Array properties accept JSON arrays so, if we get a value starting with "["
	// it's probably a JSON array
	if strings.HasPrefix(v, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			s.vals = append(s.vals, arr...)
			return nil
		}
	}
	// Otherwise treat as a single value
	s.vals = append(s.vals, v)
	return nil
}

func (s *strSliceFlag) String() string {
	return strings.Join(s.vals, ",")
}

func DerefFlagValues(fs *flag.FlagSet, vars map[string]any) {
	setFlags := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	for k, v := range vars {
		if !setFlags[k] {
			delete(vars, k)
			continue
		}
		rfl := reflect.ValueOf(v)
		if rfl.Kind() == reflect.Pointer {
			vv := rfl.Elem().Interface()
			if slice, ok := vv.(strSliceFlag); ok {
				vv = slice.vals
			}
			vars[k] = vv
		}
	}
}

func BuildArgFlagSet(tool *ToolDef) (*flag.FlagSet, map[string]any, error) {
	if tool == nil {
		return nil, nil, errors.New("cannot build flagset on nil Tool Definition")
	}
	fs := flag.NewFlagSet(tool.Name, flag.ContinueOnError)
	flagVars := map[string]any{}

	for name, pVal := range tool.InputSchema.Properties {
		switch pv := pVal.(type) {
		case *SchemaPrimitive:
			switch pv.Type {
			case "integer":
				dst := fs.Int(name, 0, pv.Description)
				flagVars[name] = dst

			case "boolean":
				dst := fs.Bool(name, false, pv.Description)
				flagVars[name] = dst
			case "string":
				dst := fs.String(name, "", pv.Description)
				flagVars[name] = dst
			default:
				return nil, nil, fmt.Errorf("unknown schema primitive kind %q", pv.Type)

			}
		case *SchemaArray:
			strSlice := new(strSliceFlag)
			fs.Var(strSlice, name, pv.Description)
			flagVars[name] = strSlice
		case *SchemaObject:
			// TODO(burmudar): we can support SchemaObject as part of stdin echo '{ stuff }' | sg mcp commit-search
			// not supported yet
			// Also support sg mcp commit-search --json '{ stuff }'
		}
	}

	return fs, flagVars, nil
}
