package httputil

import "fmt"

type HTTPError struct {
	Status int   // HTTP status code.
	Err    error // Optional reason for the HTTP error.
}

func (err *HTTPError) Error() string {
	if err.Err != nil {
		return fmt.Sprintf("status %d, reason %s", err.Status, err.Err.Error())
	}
	return fmt.Sprintf("Status %d", err.Status)
}
