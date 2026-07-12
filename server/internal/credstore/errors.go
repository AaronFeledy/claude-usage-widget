package credstore

import "errors"

func errorsJoin(err error, cleanup error) error {
	return errors.Join(err, cleanup)
}
