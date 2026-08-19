package interfaces

import (
	"bytes"
	"testing"
)

func allBackends() []BackendType {
	telegramID := BackendType("telegram")
	discordID := BackendType("discord")
	RegisterInstance(telegramID, &TelegramService{})
	RegisterInstance(discordID, &DiscordService{})
	return []BackendType{telegramID, discordID}
}

func assertEquivalentErrors(t *testing.T, got error, want error) {
	t.Helper()

	if (got == nil) != (want == nil) {
		t.Fatalf("error mismatch: got=%v want=%v", got, want)
	}

	if got != nil && want != nil && got.Error() != want.Error() {
		t.Fatalf("error text mismatch: got=%q want=%q", got.Error(), want.Error())
	}
}

func TestGetService_SupportedBackends(t *testing.T) {
	for _, backend := range allBackends() {
		backend := backend
		t.Run(string(backend), func(t *testing.T) {
			svc, err := getService(backend)
			if err != nil {
				t.Fatalf("getService(%v) error = %v", backend, err)
			}
			if svc == nil {
				t.Fatalf("getService(%v) returned nil service", backend)
			}
		})
	}
}

func TestGetService_UnsupportedBackend(t *testing.T) {
	_, err := getService(BackendType("nonexistent"))
	if err == nil {
		t.Fatal("getService() error = nil, want unsupported backend error")
	}
}

func TestGetMaxShardSize_DelegatesToBackendService(t *testing.T) {
	for _, backend := range allBackends() {
		backend := backend
		t.Run(string(backend), func(t *testing.T) {
			svc, err := getService(backend)
			if err != nil {
				t.Fatalf("getService(%v) error = %v", backend, err)
			}

			want := svc.GetMaxObjectSize()
			got, err := GetMaxShardSize(backend)
			if err != nil {
				t.Fatalf("GetMaxShardSize(%v) error = %v", backend, err)
			}
			if got != want {
				t.Fatalf("GetMaxShardSize(%v) = %d, want %d", backend, got, want)
			}
		})
	}
}

func TestGetShard_DelegatesToBackendService(t *testing.T) {
	const location = "test-location"

	for _, backend := range allBackends() {
		backend := backend
		t.Run(string(backend), func(t *testing.T) {
			svc, err := getService(backend)
			if err != nil {
				t.Fatalf("getService(%v) error = %v", backend, err)
			}

			wantData, wantErr := svc.GetObject(location)
			gotData, gotErr := GetShard(backend, location)

			assertEquivalentErrors(t, gotErr, wantErr)
			if !bytes.Equal(gotData, wantData) {
				t.Fatalf("GetShard(%v) data mismatch: got=%v want=%v", backend, gotData, wantData)
			}
		})
	}
}

func TestPutShard_DelegatesToBackendService(t *testing.T) {
	const location = "test-location"
	data := []byte("test-data")

	for _, backend := range allBackends() {
		backend := backend
		t.Run(string(backend), func(t *testing.T) {
			svc, err := getService(backend)
			if err != nil {
				t.Fatalf("getService(%v) error = %v", backend, err)
			}

			wantLocation, wantErr := svc.PutObject(data)
			gotLocation, gotErr := PutShard(backend, data)

			assertEquivalentErrors(t, gotErr, wantErr)
			if gotLocation != wantLocation {
				t.Fatalf("PutShard(%v) location = %v, want %v", backend, gotLocation, wantLocation)
			}
		})
	}
}

func TestDeleteShard_DelegatesToBackendService(t *testing.T) {
	const location = "test-location"

	for _, backend := range allBackends() {
		backend := backend
		t.Run(string(backend), func(t *testing.T) {
			svc, err := getService(backend)
			if err != nil {
				t.Fatalf("getService(%v) error = %v", backend, err)
			}

			wantErr := svc.DeleteObject(location)
			gotErr := DeleteShard(backend, location)

			assertEquivalentErrors(t, gotErr, wantErr)
		})
	}
}

func TestWrapperFunctions_UnsupportedBackend(t *testing.T) {
	invalidBackend := BackendType("nonexistent")

	if _, err := GetMaxShardSize(invalidBackend); err == nil {
		t.Fatal("GetMaxShardSize() error = nil, want unsupported backend error")
	}

	if _, err := GetShard(invalidBackend, "test-location"); err == nil {
		t.Fatal("GetShard() error = nil, want unsupported backend error")
	}

	if _, err := PutShard(invalidBackend, []byte("test-data")); err == nil {
		t.Fatal("PutShard() error = nil, want unsupported backend error")
	}

	if err := DeleteShard(invalidBackend, "test-location"); err == nil {
		t.Fatal("DeleteShard() error = nil, want unsupported backend error")
	}
}

func TestLifecycle(t *testing.T) {
	// This test will check the full lifecycle of a shard: Put, Get, Delete
	data := []byte("test-data")

	for _, backend := range allBackends() {
		backend := backend
		t.Run(string(backend), func(t *testing.T) {
			// Put
			location, err := PutShard(backend, data)
			if err != nil {
				t.Fatalf("PutShard(%v) error = %v", backend, err)
			}

			// Get
			gotData, err := GetShard(backend, location)
			if err != nil {
				t.Fatalf("GetShard(%v) error = %v", backend, err)
			}
			if !bytes.Equal(gotData, data) {
				t.Fatalf("GetShard(%v) data mismatch: got=%v want=%v", backend, gotData, data)
			}

			// Delete
			err = DeleteShard(backend, location)
			if err != nil {
				t.Fatalf("DeleteShard(%v) error = %v", backend, err)
			}
		})
	}
}
