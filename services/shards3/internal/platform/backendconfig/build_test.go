package backendconfig

import (
	"strings"
	"testing"

	"shards3/services/shards3/internal/modules/storage/interfaces"
)

func TestBuildBackends_UnknownKind(t *testing.T) {
	_, err := BuildBackends([]BackendDef{
		{ID: "mystery", Kind: "does-not-exist", Enabled: true},
	})
	if err == nil {
		t.Fatal("BuildBackends() error = nil, want error for unknown kind")
	}
}

func TestBuildBackends_MissingSecretDoesNotLeakValue(t *testing.T) {
	t.Setenv("SHARDS3_BACKEND_TG_TEST_BOT_TOKEN", "")

	_, err := BuildBackends([]BackendDef{
		{ID: "tg-test", Kind: "telegram", Enabled: true},
	})
	if err == nil {
		t.Fatal("BuildBackends() error = nil, want error for missing secret")
	}
	if !strings.Contains(err.Error(), "bot_token") {
		t.Fatalf("error = %q, want it to name the missing secret key", err.Error())
	}
}

func TestBuildBackends_DisabledSkipsSecretResolution(t *testing.T) {
	// No SHARDS3_BACKEND_TG_DISABLED_BOT_TOKEN is set; a disabled backend
	// must not fail just because its secret is missing.
	ids, err := BuildBackends([]BackendDef{
		{ID: "tg-disabled", Kind: "telegram", Enabled: false},
	})
	if err != nil {
		t.Fatalf("BuildBackends() error = %v, want nil for a disabled backend", err)
	}
	if len(ids) != 0 {
		t.Fatalf("ids = %v, want empty for a disabled backend", ids)
	}
}

func TestBuildBackends_EnabledRegistersInstance(t *testing.T) {
	t.Setenv("SHARDS3_BACKEND_FILE_TEST_STORAGE_DIR", "unused")

	ids, err := BuildBackends([]BackendDef{
		{ID: "file-test", Kind: "file", Enabled: true, Settings: map[string]any{"storage_dir": t.TempDir()}},
	})
	if err != nil {
		t.Fatalf("BuildBackends() error: %v", err)
	}
	if len(ids) != 1 || ids[0] != interfaces.BackendType("file-test") {
		t.Fatalf("ids = %v, want [file-test]", ids)
	}

	if _, err := interfaces.GetMaxShardSize(ids[0]); err != nil {
		t.Fatalf("GetMaxShardSize(%v) error = %v, want the instance to be registered", ids[0], err)
	}
}

func TestSecretEnvName(t *testing.T) {
	got := secretEnvName("my-backend.1", "bot_token")
	want := "SHARDS3_BACKEND_MY_BACKEND_1_BOT_TOKEN"
	if got != want {
		t.Fatalf("secretEnvName() = %q, want %q", got, want)
	}
}
