package errors

// ClientError indicates that the error is due to a mistake in
// the API client's request (4xx in HTTP), it's not a server-internal
// error (5xx in HTTP).
//
// GraphQL doesn't require tracking this separately, but it's useful
// for making sure that we can set SLOs against server errors.
//
// For example, on Sourcegraph.com, people can send us arbitrary GraphQL
// requests; it wouldn't make sense for errors in processing malformed
// requests to count against our (internal) SLO.
type ClientError struct {
	Err error
}

var _ error = ClientError{nil}
var _ Wrapper = ClientError{nil}

func (e ClientError) Error() string {
	return e.Err.Error()
}

func (e ClientError) Unwrap() error {
	return e.Err
}
