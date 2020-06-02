package main

import (
	"flag"
	"fmt"
)

func init() {
	usage := `
Examples:

  List settings for the current user (authenticated by the src CLI's access token, if any):

    	$ src config list

  List settings for the user with username alice:

    	$ src config list -subject=$(src users get -f '{{.ID}}' -username=alice)

`

	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src config %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		subjectFlag = flagSet.String("subject", "", "The ID of the settings subject whose settings to list. (default: authenticated user)")
		formatFlag  = flagSet.String("f", "", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.|json}}")`)
		apiFlags    = newAPIFlags(flagSet)
	)

	handler := func(args []string) error {
		err := flagSet.Parse(args)
		if err != nil {
			return err
		}

		var formatStr string
		if *formatFlag != "" {
			formatStr = *formatFlag
		} else {
			// Set default here instead of in flagSet.String because it is very long and makes the usage message ugly.
			formatStr = `{{range .Subjects -}}
# {{.SettingsURL}}:{{with .LatestSettings}}
{{.Contents}}
{{- else}} (empty){{- end}}
{{end}}`
		}
		tmpl, err := parseTemplate(formatStr)
		if err != nil {
			return err
		}

		var query string
		var queryVars map[string]interface{}
		if *subjectFlag == "" {
			query = viewerSettingsQuery
		} else {
			query = settingsSubjectCascadeQuery
			queryVars = map[string]interface{}{
				"subject": nullString(*subjectFlag),
			}
		}

		var result struct {
			ViewerSettings  *SettingsCascade
			SettingsSubject *SettingsSubject
		}
		return (&apiRequest{
			query:  query,
			vars:   queryVars,
			result: &result,
			done: func() error {
				var cascade *SettingsCascade
				if result.ViewerSettings != nil {
					cascade = result.ViewerSettings
				} else if result.SettingsSubject != nil {
					cascade = &result.SettingsSubject.SettingsCascade
				}
				return execTemplate(tmpl, cascade)
			},
			flags: apiFlags,
		}).do()
	}

	// Register the command.
	configCommands = append(configCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
