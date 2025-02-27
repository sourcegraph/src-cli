package testing

import (
    "encoding/json"
    "os"
    "text/template"
)

// ExecTemplateWithParsing formats and prints data using the provided template format.
// It handles both template parsing and execution in a single call.
func ExecTemplateWithParsing(format string, data interface{}) error {
    funcMap := template.FuncMap{
        "json": func(v interface{}) string {
            b, err := json.Marshal(v)
            if err != nil {
                return err.Error()
            }
            return string(b)
        },
    }

    // Use a generic template name or allow it to be passed as parameter
    tmpl, err := template.New("template").Funcs(funcMap).Parse(format)
    if err != nil {
        return err
    }
    return tmpl.Execute(os.Stdout, data)
}