package rest2sftp

type Error struct {
	StatusCode uint64 `json:"statusCode"`
	Message string `json:"message"`
}

func (err *Error)Error() string {
	return err.Message
}

func Wrap(err error, message string) *Error {
	return &Error{
		Message: err.Error() +", " + message,
	}
}