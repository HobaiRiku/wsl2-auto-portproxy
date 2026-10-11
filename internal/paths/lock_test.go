package paths

import "testing"

func TestDataRootLockReleased(t *testing.T) {
	home := t.TempDir()
	release, err := Lock(home)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Lock(home); err == nil {
		other()
		t.Fatal("second instance acquired same root")
	}
	release()
	release, err = Lock(home)
	if err != nil {
		t.Fatal("lock not released:", err)
	}
	release()
}
