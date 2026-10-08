package windowsservice

import (
	"errors"
	"reflect"
	"testing"
)

func TestUpdateFailureRecovery(t *testing.T) {
	for _, tc := range []struct {
		failed string
		calls  []string
	}{
		{"", []string{"stop", "backup", "replace", "start", "healthy", "cleanup"}},
		{"stop", []string{"stop"}},
		{"backup", []string{"stop", "backup", "start"}},
		{"replace", []string{"stop", "backup", "replace", "start"}},
		{"start", []string{"stop", "backup", "replace", "start", "stop", "restore", "start"}},
		{"healthy", []string{"stop", "backup", "replace", "start", "healthy", "stop", "restore", "start"}},
	} {
		t.Run(tc.failed, func(t *testing.T) {
			var calls []string
			failed := false
			operation := func(name string) func() error {
				return func() error {
					calls = append(calls, name)
					if !failed && name == tc.failed {
						failed = true
						return errors.New("injected failure")
					}
					return nil
				}
			}
			err := updateBinary(updateOperations{stop: operation("stop"), start: operation("start"), backup: operation("backup"), replace: operation("replace"), restore: operation("restore"), healthy: operation("healthy"), cleanup: func() { calls = append(calls, "cleanup") }})
			if (err != nil) != (tc.failed != "") || !reflect.DeepEqual(calls, tc.calls) {
				t.Fatalf("calls %v, err %v", calls, err)
			}
		})
	}
}
func TestUpdateReportsRollbackFailure(t *testing.T) {
	ok := func() error { return nil }
	failure := errors.New("restore denied")
	err := updateBinary(updateOperations{stop: ok, start: ok, backup: ok, replace: ok, restore: func() error { return failure }, healthy: func() error { return errors.New("unhealthy") }, cleanup: func() { t.Fatal("cleaned unsuccessful update") }})
	if !errors.Is(err, failure) {
		t.Fatal("lost rollback failure:", err)
	}
}
