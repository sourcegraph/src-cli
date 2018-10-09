package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"text/template"
)

type VerifyEmailJson struct {
	ID    string `json:"ID"`
	Email string `json:"Email"`
}

func init() {
	usage := `
Examples:

  Verify one or more user account(s):

    	$ src users emails verify-json='[{"ID": "ID1", "Email": "email1"},...]'

`

	flagSet := flag.NewFlagSet("emails", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src users %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		verifyJsonFlag = flagSet.String("verify-json", "",
			`The JSON array of id-email descriptors of those user/emails that should be marked as verified. (required)`)
		apiFlags = newAPIFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		var verifyList []VerifyEmailJson
		if err := json.Unmarshal([]byte(*verifyJsonFlag), &verifyList); err != nil {
			return err
		}

		graphQlTemplate := template.New("setUserEmailVerifiedQL")
		if _, err := graphQlTemplate.Parse(`
mutation VerifyUserEmails(
{{- range $idx, $_ := . -}}
$user{{ printf "%09d" $idx }} ID!, $email{{ printf "%09d" $idx }} String!, 
{{- end -}}
) {
{{- range $idx, $_ := . }}
  verify{{ printf "%09d" $idx }}: setUserEmailVerified(user: $user{{ printf "%09d" $idx }}, email: $email{{ printf "%09d" $idx }}, verified: true) {
    alwaysNil
  }
{{- end }}
}
`); err != nil {
			log.Fatal(`setUserVerifiedEmail template failed to parse;
please file an issue at https://github.com/sourcegraph/sourcegraph/issues/new/choose`, err)
		}
		queryBuf := new(bytes.Buffer)
		if err := graphQlTemplate.Execute(queryBuf, verifyList); err != nil {
			return err
		}
		query := queryBuf.String()

		vars := map[string]interface{}{}

		for idx, verifyObj := range verifyList {
			// be careful if you change the template, since these keys need to match
			userKey := fmt.Sprintf("user%09d", idx)
			emailKey := fmt.Sprintf("email%09d", idx)
			vars[userKey] = verifyObj.ID
			vars[emailKey] = verifyObj.Email
		}

		var result struct {
			VerifyUserEmails struct{}
		}
		return (&apiRequest{
			query:  query,
			vars:   vars,
			result: &result,
			done: func() error {
				fmt.Printf("Verified %d emails.\n", len(verifyList))
				return nil
			},
			flags: apiFlags,
		}).do()
	}

	// Register the command.
	usersCommands = append(usersCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
