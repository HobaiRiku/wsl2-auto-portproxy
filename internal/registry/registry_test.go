package registry

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidReloadKeepsConfigAndCanBeRepaired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"onlyPredefined":true}`), 0600)
	r := New(path)
	initial, ready, problem := r.Snapshot()
	if !ready || problem != "" {
		t.Fatal(problem)
	}
	os.WriteFile(path, []byte(`{"predefined":{"tcp":["22"]}}`), 0600)
	r.Reload()
	current, ready, problem := r.Snapshot()
	if !ready || !current.Config.OnlyPredefined || problem == "" || current.Revision != initial.Revision {
		t.Fatal("invalid reload published")
	}
	repaired, err := r.Replace(current.Revision, []byte(`{"onlyPredefined":true,"predefined":{"tcp":["666:22"]}}`))
	if err != nil || len(repaired.Config.Predefined.Tcp) != 1 {
		t.Fatalf("repair: %v", err)
	}
	os.Remove(path)
	r.Reload()
	current, ready, problem = r.Snapshot()
	if !ready || !current.Config.OnlyPredefined || problem == "" {
		t.Fatal("deleted config widened exposure")
	}
}
func TestInitialInvalidConfigBlocksUntilRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"unknown":true}`), 0600)
	r := New(path)
	doc, ready, problem := r.Snapshot()
	if ready || problem == "" {
		t.Fatal("accepted invalid initial config")
	}
	if _, err := r.Replace(doc.Revision, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	_, ready, problem = r.Snapshot()
	if !ready || problem != "" {
		t.Fatal(problem)
	}
}
func TestExternalEditAndRevisionConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	r := New(path)
	doc, _, _ := r.Snapshot()
	r.Reload()
	_, _, problem := r.Snapshot()
	if problem != "" {
		t.Fatal("initial absent config is allowed")
	}
	os.WriteFile(path, []byte(`{"onlyPredefined":true}`), 0600)
	if _, err := r.Replace(doc.Revision, []byte(`{}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("overwrote external edit: %v", err)
	}
	r.Reload()
	latest, _, _ := r.Snapshot()
	if !latest.Config.OnlyPredefined {
		t.Fatal("edit not loaded")
	}
	if _, err := r.Replace(doc.Revision, []byte(`{}`)); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted stale revision")
	}
}

func TestEmptyConfigRequiresRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	r := New(path)
	doc, ready, problem := r.Snapshot()
	if ready || problem == "" {
		t.Fatal("empty file silently accepted")
	}
	if _, err := r.Replace(doc.Revision, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
}
func TestBackupPreservedAndDeletedFileRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{/* migration */ "onlyPredefined":true}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	r := New(path)
	doc, _, _ := r.Snapshot()
	next, err := r.Replace(doc.Revision, []byte(`{"onlyPredefined":true,"udpEnabled":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	r.Reload()
	if _, err := r.Replace(next.Revision, []byte(`{"onlyPredefined":true}`)); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != string(original) {
		t.Fatalf("migration backup changed: %v", err)
	}
}
func TestRejectedTextIsKeptForRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"onlyPredefined":`), 0600)
	r := New(path)
	if text, problem := r.Rejected(); string(text) != `{"onlyPredefined":` || problem == "" {
		t.Fatalf("initial invalid file: %q %q", text, problem)
	}
	doc, _, _ := r.Snapshot()
	if _, err := r.Replace(doc.Revision, []byte(`{"onlyPredefined":true}`)); err != nil {
		t.Fatal(err)
	}
	if text, _ := r.Rejected(); text != nil {
		t.Fatal("repair kept rejected text")
	}
	os.WriteFile(path, nil, 0600)
	r.Reload()
	if text, problem := r.Rejected(); text == nil || problem == "" {
		t.Fatal("empty file not reported as rejected")
	}
	os.WriteFile(path, []byte(`{}`), 0600)
	r.Reload()
	if text, _ := r.Rejected(); text != nil {
		t.Fatal("valid external edit kept rejected text")
	}
}
