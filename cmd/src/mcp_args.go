package main

import (
	"flag"
	"fmt"
	"reflect"
	"strings"
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

func derefFlagValues(vars map[string]any) {
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

func buildArgFlagSet(tool *MCPToolDef) (*flag.FlagSet, map[string]any, error) {
	fs := flag.NewFlagSet(tool.Name(), flag.ContinueOnError)
	flagVars := map[string]any{}

	for name, pVal := range tool.InputSchema.Properties {
		switch pv := pVal.(type) {
		case *SchemaPrimitive:
			switch pv.Kind {
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
				return nil, nil, fmt.Errorf("unknown schema primitive kind %q", pv.Kind)

			}
		case *SchemaArray:
			strSlice := new(strSliceFlag)
			fs.Var(strSlice, name, pv.Description)
			flagVars[name] = strSlice
		case *SchemaObject:
			// not supported yet
		}
	}

	return fs, flagVars, nil
}
