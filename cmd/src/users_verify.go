package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
)

const SubCommand = "verify-emails"

type VerifyEmailJson struct {
	ID    string `json:"ID"`
	Email string `json:"Email"`
}

func init() {
	usage := `
Examples:

  Verify one or more user account(s):

    	$ src users ` + SubCommand + ` -verify-json='[{"ID": "ID1", "Email": "email1"},...]'

`

	flagSet := flag.NewFlagSet(SubCommand, flag.ExitOnError)
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
		flagSet.Parse(args)

		dec := json.NewDecoder(strings.NewReader(*verifyJsonFlag))

		var verifyList []VerifyEmailJson
		if err := dec.Decode(&verifyList); err != nil {
			return err
		}

		vars := map[string]interface{}{}

		// translate the provided index of `verifyList` into variables for its `id` and `email` payload
		indexToKeys := func(idx int) (string, string) {
			return fmt.Sprintf("id%09d", idx), fmt.Sprintf("email%09d", idx)
		}

		for idx, verifyObj := range verifyList {
			idKey, emailKey := indexToKeys(idx)
			vars[idKey] = verifyObj.ID
			vars[emailKey] = verifyObj.Email
		}

		query := `mutation VerifyUserEmails(`
		for k := range vars {
			varType := "String!"
			if strings.HasPrefix(k, "id") {
				varType = "ID!"
			}
			query += fmt.Sprintf("$%s: %s,", k, varType)
		}
		query += ") {"
		for idx := range verifyList {
			idKey, emailKey := indexToKeys(idx)
			query += fmt.Sprintf(`verify%09d: setUserEmailVerified(user: $%s, email: $%s, verified: true) {
alwaysNil
}`, idx, idKey, emailKey)
		}
		query += "}"

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
