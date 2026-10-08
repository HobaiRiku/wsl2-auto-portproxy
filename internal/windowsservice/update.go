package windowsservice

import (
	"errors"
	"fmt"
)

// The transaction is independent of SCM so failure recovery is exercised on
// every development platform; Windows supplies the actual operations.
type updateOperations struct {
	stop, start, backup, replace, restore, healthy func() error
	cleanup                                        func()
}

func updateBinary(o updateOperations) error {
	if err := o.stop(); err != nil {
		return err
	}
	if err := o.backup(); err != nil {
		return errors.Join(err, o.start())
	}
	if err := o.replace(); err != nil {
		return errors.Join(err, o.start())
	}
	failure := o.start()
	if failure == nil {
		failure = o.healthy()
	}
	if failure == nil {
		o.cleanup()
		return nil
	}
	if err := o.stop(); err != nil {
		return fmt.Errorf("update failed (%v); unable to stop for rollback: %w", failure, err)
	}
	if err := o.restore(); err != nil {
		return fmt.Errorf("update failed (%v); rollback binary: %w", failure, err)
	}
	if err := o.start(); err != nil {
		return fmt.Errorf("update failed (%v); rollback restart: %w", failure, err)
	}
	return fmt.Errorf("update failed; previous binary restored: %w", failure)
}
