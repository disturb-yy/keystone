package execution

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGuardEvidenceUsesStableGitFacts(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	calls := make([][]string, 0)
	// fake runner 提供文件系统事实，本测试只验证 Git 命令边界，不依赖真实仓库。
	guard := &Guard{RunGit: func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if len(args) >= 4 && args[len(args)-1] == "--show-toplevel" {
			return []byte(workspace + "\n"), nil
		}
		if len(args) >= 4 && args[2] == "rev-parse" && args[len(args)-1] == "HEAD" {
			return []byte("abc\n"), nil
		}
		if len(args) >= 3 && args[2] == "diff" {
			return []byte("diff\n"), nil
		}
		return []byte(" M b.go\x00?? a.go\x00"), nil
	}}
	// ValidateWorkspace 会检查 os.Stat；先创建可写目录。
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := guard.ValidateWorkspace(context.Background(), workspace)
	if err != nil {
		t.Fatalf("ValidateWorkspace() error = %v", err)
	}
	if root != workspace {
		t.Fatalf("root = %q, want %q", root, workspace)
	}
	evidence, err := guard.Evidence(context.Background(), workspace, DefaultLimits())
	if err != nil {
		t.Fatalf("Evidence() error = %v", err)
	}
	if evidence.Revision != "abc" || !reflect.DeepEqual(evidence.ChangedFiles, []string{"a.go", "b.go"}) {
		t.Fatalf("evidence = %#v", evidence)
	}
	if len(calls) != 4 {
		t.Fatalf("Git call count = %d, want 4", len(calls))
	}
}

func TestGuardCheckRevisionFindsHeadChange(t *testing.T) {
	guard := NewGuard()
	if got := guard.CheckRevision("a", "b"); len(got) != 1 {
		t.Fatalf("CheckRevision() = %#v, want one finding", got)
	}
	if got := guard.CheckRevision("a", "a"); got != nil {
		t.Fatalf("CheckRevision() = %#v, want nil", got)
	}
}

func TestGuardPinsRuntimeTemporaryEnvironmentToControlledDirectory(t *testing.T) {
	base := t.TempDir()
	guard := NewGuard()
	environment, cleanup, err := guard.PrepareEnvironmentAt(context.Background(), []string{
		"PATH=" + os.Getenv("PATH"),
		"TMPDIR=/source/repository/tmp",
		"TEMP=/source/repository/temp",
		"TMP=/source/repository/tmp-windows",
	}, base)
	if err != nil {
		t.Fatal(err)
	}
	runtimeTemp := lookupEnv(environment, "TMPDIR")
	if runtimeTemp == "" || filepath.Dir(runtimeTemp) != base {
		t.Fatalf("TMPDIR = %q, want a child of %q", runtimeTemp, base)
	}
	if got := lookupEnv(environment, "TEMP"); got != runtimeTemp {
		t.Fatalf("TEMP = %q, want %q", got, runtimeTemp)
	}
	if got := lookupEnv(environment, "TMP"); got != runtimeTemp {
		t.Fatalf("TMP = %q, want %q", got, runtimeTemp)
	}
	cleanup()
	if _, err := os.Stat(runtimeTemp); !os.IsNotExist(err) {
		t.Fatalf("runtime temporary directory survived cleanup: %v", err)
	}
}
