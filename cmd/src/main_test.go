package main

import (
	"io/ioutil"
	"os"
	"testing"
)

type FlagTestCaseInput struct {
	testState *testing.T
	// any env-var contents for the access token
	accessTokenEnv string
	// any env-var contents for the endpoint
	endpointEnv string
	// any value to be provided to the endpoint flag
	endpointFlag string
	// any contents to be serialized to a file and specified as an env-var
	configJsonEnvContents string
	// any contents to be serialized to a file and specified as a flag
	configJsonFlagContents string
}

// Access Token Env | Access Token in Config(Flag) | Access Token in Config(Env)
// Endpoint Env     | Endpoint in Config(Env)      | Endpoint in Flag
func TestReadConfig(parentT *testing.T) {

	// it seemed less opaque to put these steps here than in a top-level func
	runAndAssert := func(expectedConfig config, t *testing.T) {
		var actualConfig *config
		if c, cfgErr := readConfig(); cfgErr == nil {
			actualConfig = c
		} else {
			t.Fatal(cfgErr)
		}

		assertEquals(actualConfig.AccessToken, expectedConfig.AccessToken, t)
		assertEquals(actualConfig.Endpoint, expectedConfig.Endpoint, t)
	}

	parentT.Run("pure defaults", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState: t,
			// intentionally blank
		}
		expected := config{
			AccessToken: "",
			Endpoint:    defaultEndpoint,
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Access Token Env [X] | Access Token in Config(Flag) | Access Token in Config(Env)
	parentT.Run("accessToken in the environment", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState: t,

			accessTokenEnv: "abcdef-ghi",
		}
		expected := config{
			AccessToken: "abcdef-ghi",
			Endpoint:    defaultEndpoint,
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Access Token Env | Access Token in Config(Flag) [X] | Access Token in Config(Env)
	parentT.Run("accessToken in the -config flag", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState:              t,
			configJsonFlagContents: `{"accessToken": "secret-file-value"}`,
		}
		expected := config{
			AccessToken: "secret-file-value",
			Endpoint:    defaultEndpoint,
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Access Token Env | Access Token in Config(Flag) | Access Token in Config(Env) [X]
	parentT.Run("accessToken in a file identified by the SRC_CONFIG env", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState: t,

			configJsonEnvContents: `{"accessToken": "secret-value-here"}`,
		}
		expected := config{
			AccessToken: "secret-value-here",
			Endpoint:    defaultEndpoint,
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Access Token Env [X] | Access Token in Config(Flag) [X] | Access Token in Config(Env)
	parentT.Run("accessToken in the env, superseding -config file", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState: t,

			accessTokenEnv:         "supersede-access-token",
			configJsonFlagContents: `{"accessToken": "old-value-here"}`,
		}
		expected := config{
			AccessToken: "supersede-access-token",
			Endpoint:    defaultEndpoint,
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Access Token Env [X] | Access Token in Config(Flag) | Access Token in Config(Env) [X]
	parentT.Run("accessToken in the env, superseding SRC_CONFIG file", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState: t,

			accessTokenEnv:        "this-access-token",
			configJsonEnvContents: `{"accessToken": "not-this-one"}`,
		}
		expected := config{
			AccessToken: "this-access-token",
			Endpoint:    defaultEndpoint,
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Access Token Env [X] | Access Token in Config(Flag) [X] | Access Token in Config(Env) [X]
	parentT.Run("accessToken in the env, superseding -config, with SRC_CONFIG set", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState: t,

			accessTokenEnv:         "highest-priority-access-token",
			configJsonEnvContents:  `{"accessToken": "not-this-one"}`,
			configJsonFlagContents: `{"accessToken": "or-this-one"}`,
		}
		expected := config{
			AccessToken: "highest-priority-access-token",
			Endpoint:    defaultEndpoint,
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Access Token Env | Access Token in Config(Flag) [X] | Access Token in Config(Env) [X]
	parentT.Run("accessToken in -config, with SRC_CONFIG set", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState: t,

			configJsonEnvContents:  `{"accessToken": "not-this-one"}`,
			configJsonFlagContents: `{"accessToken": "flag-config-should-win"}`,
		}
		expected := config{
			AccessToken: "flag-config-should-win",
			Endpoint:    defaultEndpoint,
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Endpoint Env [X] | Endpoint in Config(Env) | Endpoint in Flag
	parentT.Run("endpoint in SRC_ENDPOINT", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState: t,

			// go ahead and toss in a "make sure it trims the same" case, too
			endpointEnv: "urn:uuid:hello-from-env/",
		}
		expected := config{
			Endpoint: "urn:uuid:hello-from-env",
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Endpoint Env [X] | Endpoint in Config(Env) [X] | Endpoint in Flag
	parentT.Run("endpoint in SRC_ENDPOINT, superseding -config", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState:   t,
			endpointEnv: "urn:uuid:hello-from-env/",
		}
		expected := config{
			AccessToken: "",
			Endpoint:    "urn:uuid:hello-from-env",
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Endpoint Env [X] | Endpoint in Config(Env) [X] | Endpoint in Flag [X]
	parentT.Run("endpoint in SRC_ENDPOINT, and -config, -endpoint wins", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState:             t,
			endpointEnv:           "urn:hello-from-env",
			endpointFlag:          "urn:this-endpoint-wins",
			configJsonEnvContents: `{"endpoint": "urn:not-this-one"}`,
		}
		expected := config{
			AccessToken: "",
			Endpoint:    "urn:this-endpoint-wins",
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	// Endpoint Env | Endpoint in Config(Env) [X] | Endpoint in Flag [X]
	parentT.Run("endpoint in -config, but -endpoint wins", func(t *testing.T) {
		testCase := FlagTestCaseInput{
			testState: t,

			endpointFlag:          "urn:look-at-me-instead",
			configJsonEnvContents: `{"endpoint": "urn:nothing-to-see-here"}`,
		}
		expected := config{
			AccessToken: "",
			Endpoint:    "urn:look-at-me-instead",
		}
		testCase.Run(func() {
			runAndAssert(expected, t)
		})
	})

	parentT.Run("ensure env-vars do not leak", func(t *testing.T) {
		assertEquals(os.Getenv(accessTokenEnvVar), "", t)
		assertEquals(os.Getenv(configEnvVar), "", t)
		assertEquals(os.Getenv(endpointEnvVar), "", t)
	})
}

// will run my setup logic, then the provided function, and then clean up after myself
// any encountered `error`s are handled via the testing.T functions for that purpose
func (testCase *FlagTestCaseInput) Run(inner func()) {
	cleanupFuncs := testCase.setUp()
	inner()
	for _, fn := range cleanupFuncs {
		if e := fn(); e != nil {
			testCase.testState.Fatal(e)
		}
	}
}

// Evaluate this test case input, which may involve altering the `os.Setenv`
// but the returned callbacks are cleanup functions that will tear down any changes made by setUp
func (testCase *FlagTestCaseInput) setUp() []func() error {
	// any post-test cleanup action
	var cleanupFuncs []func() error

	if testCase.accessTokenEnv != "" {
		cleanup := pushSetenv(accessTokenEnvVar, testCase.accessTokenEnv)
		cleanupFuncs = append(cleanupFuncs, cleanup)
	}
	if testCase.endpointEnv != "" {
		cleanup := pushSetenv(endpointEnvVar, testCase.endpointEnv)
		cleanupFuncs = append(cleanupFuncs, cleanup)
	}

	if testCase.endpointFlag != "" {
		oldEndpoint := endpoint
		endpoint = &testCase.endpointFlag
		cleanupFuncs = append(cleanupFuncs, func() error {
			endpoint = oldEndpoint
			return nil
		})
	}

	if testCase.configJsonEnvContents != "" {
		configFile := writeTempConfigFile(testCase.configJsonEnvContents, testCase.testState)
		configFilename := configFile.Name()
		cleanup := pushSetenv(configEnvVar, configFilename)
		cleanupFuncs = append(cleanupFuncs, cleanup)
		cleanupFuncs = append(cleanupFuncs, func() error {
			return os.Remove(configFilename)
		})
	}

	if testCase.configJsonFlagContents != "" {
		configFile := writeTempConfigFile(testCase.configJsonFlagContents, testCase.testState)
		configFilename := configFile.Name()
		oldConfigPath := configPath
		configPath = &configFilename
		cleanupFuncs = append(cleanupFuncs, func() error {
			// cheat and piggy back on this cleanup
			configPath = oldConfigPath

			return os.Remove(configFilename)
		})
	}

	return cleanupFuncs
}

// serialize the provided string into a new temp file named `src-config-something.json`
// so the user can clearly identify any stragglers
// You are responsible for deleting the file when you are done with it.
func writeTempConfigFile(contents string, t *testing.T) os.File {
	t.Helper()
	tmpFile, tmpErr := ioutil.TempFile("", "src-config-*.json")
	if tmpErr != nil {
		t.Fatal(tmpErr)
	}
	tmpFile.WriteString(contents)
	tmpFile.Close()
	return *tmpFile
}

func pushSetenv(envName, envValue string) func() error {
	existingEnvValue := os.Getenv(envName)
	os.Setenv(envName, envValue)
	return func() error {
		return os.Setenv(envName, existingEnvValue)
	}
}

func assertEquals(actual, wanted string, t *testing.T) {
	t.Helper()
	if actual != wanted {
		t.Logf("Wanted \"%s\" but got \"%s\"", wanted, actual)
	}
}
