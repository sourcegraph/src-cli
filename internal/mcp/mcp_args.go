package mcp

import (
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
	s.vals = append(s.vals, v)
	return nil
}

func (s *strSliceFlag) String() string {
	return strings.Join(s.vals, ",")
}

func DerefFlagValues(vars map[string]any) {
	for k, v := range vars {
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
		}
	}

	return fs, flagVars, nil
}
