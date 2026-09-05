package domain

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestProjectIDValidation(t *testing.T) {
	id := NewProjectID()
	if err := id.Validate(); err != nil {
		t.Fatalf("NewProjectID().Validate() error = %v", err)
	}
	if err := ProjectID("0191a6c0-0000-7000-8000-000000000000").Validate(); err != nil {
		t.Fatalf("canonical UUIDv7 validation error = %v", err)
	}
	if err := ProjectID("0191a6c0-0000-4000-8000-000000000000").Validate(); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("UUIDv4 validation error = %v, want ErrManifestInvalid", err)
	}
}

func TestRepositoryBindingValidation(t *testing.T) {
	binding := RepositoryBinding{Root: filepath.Join("/tmp", "repo"), ManifestPath: "/tmp/repo/.keystone/project.yaml"}
	if err := binding.Validate(); err != nil {
		t.Fatalf("valid binding error = %v", err)
	}
	if err := (RepositoryBinding{Root: "/tmp/../repo", ManifestPath: "/tmp/repo/.keystone/project.yaml"}).Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unclean binding error = %v, want ErrInvalidRequest", err)
	}
}
