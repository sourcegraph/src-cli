package instancehealth

import (
	"fmt"
	"strings"
	"time"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/sourcegraph/lib/output"
)

// NewChecks returns a set of checks against the given inputs to validate the
// health of a Sourcegraph application. It is designed primarily to validate
// information that can be provided via the GraphQL API - see GetSummary for
// more details.
//
// Each test errors with only a brief summary of what went wrong, and only if
// the error is critical. Detailed output should be written to out.
func NewChecks(
	out *output.Output,
	since time.Duration,
	instanceHealth Summary,
) []func() error {
	return []func() error{
		func() error {
			b := out.Block(output.Styled(output.StyleBold, "Site alerts"))
			defer b.Close()
			return checkSiteAlerts(b, instanceHealth)
		},
		func() error {
			b := out.Block(output.Styled(output.StyleBold, "Site configuration"))
			defer b.Close()
			return checkSiteConfiguration(b, instanceHealth)
		},
		func() error {
			b := out.Block(output.Styled(output.StyleBold, "Monitoring alerts"))
			defer b.Close()
			return checkMonitoringAlerts(b, since, instanceHealth)
		},
		func() error {
			b := out.Block(output.Styled(output.StyleBold, "External services"))
			defer b.Close()
			return checkExternalServices(b, since, instanceHealth)
		},
		func() error {
			b := out.Block(output.Styled(output.StyleBold, "Permissions syncing"))
			defer b.Close()
			return checkPermissionsSyncing(b, since, instanceHealth)
		},
	}
}

// checkSiteAlerts indicates if there are any alerts issued by the application
func checkSiteAlerts(
	out output.Writer,
	instanceHealth Summary,
) error {
	if len(instanceHealth.Site.Alerts) > 0 {
		out.WriteLine(output.Linef(output.EmojiWarning, output.StyleWarning,
			"Found site-level alerts:"))
		for _, a := range instanceHealth.Site.Alerts {
			out.Writef("\t%s: %q", a.Type, a.Message)
		}
		return errors.New("site-level alerts")
	}
	out.WriteLine(output.Emoji(output.EmojiSuccess, "No site-level alerts!"))
	return nil
}

// checkSiteAlerts indicates if there are any alerts issued by the application regarding
// configuration validation
func checkSiteConfiguration(
	out output.Writer,
	instanceHealth Summary,
) error {
	if len(instanceHealth.Site.Configuration.ValidationMessages) > 0 {
		out.WriteLine(output.Linef(output.EmojiWarning, output.StyleWarning,
			"Found configuration validation alerts:"))
		for _, m := range instanceHealth.Site.Configuration.ValidationMessages {
			out.Writef("\t%s", m)
		}
	} else {
		out.WriteLine(output.Emoji(output.EmojiSuccess, "No site configuration issues!"))
	}
	// never error, just issue printed warning, since the application should still work
	// even with validation messages
	return nil
}

// checkMonitoringAlerts indicates if there are any alerts issued by monitoring infra
func checkMonitoringAlerts(
	out output.Writer,
	since time.Duration,
	instanceHealth Summary,
) error {
	var criticalAlerts int
	for _, a := range instanceHealth.Site.MonitoringStatistics.Alerts {
		if a.Average == 0 || !strings.Contains(strings.ToLower(a.Name), "critical") {
			continue
		}
		// average is ratio of 12h windows that alert was active, so we set the threshold
		// if it's possible this alert could have been active in this 'since' window.
		if a.Average*0.5 >= since.Hours()/12 {
			criticalAlerts += 1
			out.WriteLine(output.Linef(output.EmojiWarning, output.StyleWarning,
				"Found recently active alert: %q", a.Name))
		}
	}
	if criticalAlerts == 0 {
		out.WriteLine(output.Emoji(output.EmojiSuccess, "No critical monitoring alerts!"))
	}
	// never error, just issue printed warning, since critical alerts aren't all _that_
	// reliable today, though they provide a potentially useful signal.
	return nil
}

// checkExternalServices checks the health of external service syncing
func checkExternalServices(
	out output.Writer,
	since time.Duration,
	instanceHealth Summary,
) error {
	if len(instanceHealth.ExternalServices.Nodes) == 0 {
		out.WriteLine(output.Linef(output.EmojiWarning, output.StyleWarning,
			"No external services found"))
		return nil
	}

	var hasExtsvcIssue bool
	for _, extsvc := range instanceHealth.ExternalServices.Nodes {
		var jobCount int
		for _, job := range extsvc.SyncJobs.Nodes {
			if job.FinishedAt.Before(time.Now().Add(-since)) {
				continue
			}
			jobCount++
		}

		if extsvc.LastSyncError != nil {
			hasExtsvcIssue = true
			out.WriteLine(output.Linef(output.EmojiFailure, output.StyleFailure,
				"External service %s %q encountered sync error: %q",
				extsvc.Kind, extsvc.ID, *extsvc.LastSyncError))
		} else if jobCount == 0 {
			// not critical, this is somewhat normal behaviour
			out.WriteLine(output.Linef(output.EmojiInfo, output.StyleSuggestion,
				"External service %s %q had no sync jobs in last %s",
				extsvc.Kind, extsvc.ID, since.String()))
		} else {
			out.WriteLine(output.Emojif(output.EmojiSuccess,
				"External service %s %q healthy",
				extsvc.Kind, extsvc.ID))
		}
	}
	if hasExtsvcIssue {
		return errors.New("encountered external service issues")
	}
	out.WriteLine(output.Emoji(output.EmojiSuccess, "No external service issues!"))
	return nil
}

// checkPermissionsSyncing checks the health of permissions syncing
func checkPermissionsSyncing(
	out output.Writer,
	since time.Duration,
	instanceHealth Summary,
) error {
	var syncCount int
	var syncErrors []string
	var seenProviders = make(map[string]map[string]string) // provider : state : message
	for _, sync := range instanceHealth.PermissionsSyncJobs.Nodes {
		if sync.CompletedAt.Before(time.Now().Add(-since)) {
			continue
		}
		syncCount += 1
		if sync.Status == "ERROR" {
			syncErrors = append(syncErrors, sync.Message)
		}
		for _, p := range sync.Providers {
			key := fmt.Sprintf("%s - %s", p.Type, p.ID)
			if _, ok := seenProviders[key]; !ok {
				seenProviders[key] = make(map[string]string)
			}
			// Just track one message per state for reference
			seenProviders[key][p.Status] = p.Message
		}
	}

	if syncCount == 0 {
		out.WriteLine(output.Linef(output.EmojiWarning, output.StyleWarning,
			"No permissions sync jobs since %s ago", since.String()))
		return nil // there may be no permissions sync configured
	}

	// Summarize results by provider
	if len(seenProviders) == 0 {
		out.WriteLine(output.Linef(output.EmojiWarning, output.StyleWarning,
			"No authz providers running since %s ago", since.String()))
	} else {
		for key, messages := range seenProviders {
			for state := range messages {
				switch state {
				case "SUCCESS":
					out.WriteLine(output.Emojif(output.EmojiSuccess,
						"Authz provider %q healthy", key))
				default:
					out.WriteLine(output.Linef(output.EmojiWarning, output.StyleWarning,
						"Authz provider %q state %s: %q", key, state, messages[state]))
				}
			}
		}
	}

	// Note if syncing is failing
	if len(syncErrors) > 0 {
		out.WriteLine(output.Linef(output.EmojiFailure, output.StyleFailure,
			"Encountered permissions sync errors:"))
		for i, msg := range syncErrors {
			out.Writef("\t%q", msg)
			if i > 3 && len(syncErrors)-i > 0 {
				out.Writef("\t... %d more", len(syncErrors)-i)
				break
			}
		}
		return errors.New("permissions sync errors")
	} else if syncCount > 0 {
		out.WriteLine(output.Emoji(output.EmojiSuccess,
			"Permissions syncing healthy!"))
	}

	return nil
}
