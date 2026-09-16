package state_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Y91R/chromectl/internal/state"
)

func sample() state.State {
	return state.State{
		Port:         9222,
		PID:          4242,
		UserDataDir:  "/tmp/chromectl-profile",
		BrowserWSURL: "ws://127.0.0.1:9222/devtools/browser/abc",
		SelectedPage: "TARGET-1",
		SnapshotSeq:  3,
		UIDs: map[string]state.NodeRef{
			"3_1": {PageID: "TARGET-1", TargetID: "IFRAME-1", FrameID: "IFRAME-1", LoaderID: "L1", BackendNodeID: 42},
		},
		PendingDialog: &state.DialogPolicy{Accept: true, PromptText: "Иван"},
		Emulation: map[string]state.Emulation{
			"TARGET-1": {
				Viewport:    &state.Viewport{Width: 390, Height: 844, DeviceScaleFactor: 3, Mobile: true},
				ColorScheme: "dark",
				Network:     "Slow 3G",
				CPURate:     4,
				Geolocation: &state.Geolocation{Latitude: 55.75, Longitude: 37.62},
				Headers:     map[string]string{"X-Test": "1"},
			},
		},
	}
}

func TestStore_DirIsPerPort(t *testing.T) {
	t.Parallel()
	base := t.TempDir()

	assert.Equal(t, filepath.Join(base, "9222"), state.NewStore(base, 9222).Dir())
}

func TestLoad_WithoutStateReturnsErrNotRunning(t *testing.T) {
	t.Parallel()

	_, err := state.NewStore(t.TempDir(), 9222).Load()

	require.Error(t, err)
	assert.True(t, errors.Is(err, state.ErrNotRunning), "ожидалась ErrNotRunning, получено %v", err)
	assert.Equal(t, "браузер не запущен, выполните chromectl browser start", state.ErrNotRunning.Error())
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	t.Parallel()
	store := state.NewStore(t.TempDir(), 9222)

	require.NoError(t, store.Save(sample()))
	got, err := store.Load()

	require.NoError(t, err)
	want := sample()
	want.Version = 1
	assert.Equal(t, want, got)
}

func TestSave_WritesVersionOne(t *testing.T) {
	t.Parallel()
	store := state.NewStore(t.TempDir(), 9222)

	require.NoError(t, store.Save(sample()))

	data, err := os.ReadFile(filepath.Join(store.Dir(), "state.json"))
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, float64(1), raw["version"])
}

func TestSave_RestrictsPermissions(t *testing.T) {
	t.Parallel()
	store := state.NewStore(t.TempDir(), 9222)

	require.NoError(t, store.Save(sample()))

	dirInfo, err := os.Stat(store.Dir())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	fileInfo, err := os.Stat(filepath.Join(store.Dir(), "state.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
}

func TestSave_LeavesOnlyStateFile(t *testing.T) {
	t.Parallel()
	store := state.NewStore(t.TempDir(), 9222)

	require.NoError(t, store.Save(sample()))
	require.NoError(t, store.Save(sample()))

	entries, err := os.ReadDir(store.Dir())
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"state.json"}, names)
}

func TestLoad_NewerVersionFails(t *testing.T) {
	t.Parallel()
	store := state.NewStore(t.TempDir(), 9222)
	require.NoError(t, os.MkdirAll(store.Dir(), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(store.Dir(), "state.json"), []byte(`{"version":2,"port":9222}`), 0o600))

	_, err := store.Load()

	require.Error(t, err)
	assert.False(t, errors.Is(err, state.ErrNotRunning))
	assert.Contains(t, err.Error(), "версия state 2 новее поддерживаемой 1")
}

func TestLoad_OlderStateWithoutNewFieldsReads(t *testing.T) {
	t.Parallel()
	store := state.NewStore(t.TempDir(), 9222)
	require.NoError(t, os.MkdirAll(store.Dir(), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(store.Dir(), "state.json"), []byte(`{"version":1,"port":9222,"pid":7}`), 0o600))

	got, err := store.Load()

	require.NoError(t, err)
	assert.Equal(t, state.State{Version: 1, Port: 9222, PID: 7}, got)
}

func TestRemove_ThenLoadReturnsErrNotRunning(t *testing.T) {
	t.Parallel()
	store := state.NewStore(t.TempDir(), 9222)
	require.NoError(t, store.Save(sample()))

	require.NoError(t, store.Remove())
	_, err := store.Load()

	assert.True(t, errors.Is(err, state.ErrNotRunning), "ожидалась ErrNotRunning, получено %v", err)
}

// Регрессия: команды на одном порту идут параллельно, и каждая сохраняла state,
// прочитанный в начале, — изменения соседней команды пропадали.
func TestUpdate_ConcurrentUpdatesAreNotLost(t *testing.T) {
	t.Parallel()
	store := state.NewStore(t.TempDir(), 9222)
	require.NoError(t, store.Save(state.State{Port: 9222}))

	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- store.Update(func(st *state.State) { st.SnapshotSeq++ })
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, 40, got.SnapshotSeq)
}

func TestUpdate_WithoutStateReturnsErrNotRunning(t *testing.T) {
	t.Parallel()
	store := state.NewStore(t.TempDir(), 9222)

	err := store.Update(func(st *state.State) { st.SelectedPage = "TARGET-1" })

	assert.True(t, errors.Is(err, state.ErrNotRunning), "ожидалась ErrNotRunning, получено %v", err)
	_, statErr := os.Stat(filepath.Join(store.Dir(), "state.json"))
	assert.True(t, errors.Is(statErr, os.ErrNotExist), "Update без state не должен его создавать")
}

func TestClearPendingDialog(t *testing.T) {
	t.Parallel()
	used := state.DialogPolicy{Accept: false}
	cases := map[string]struct {
		pending *state.DialogPolicy
		want    *state.DialogPolicy
	}{
		"израсходованная снимается":         {pending: &state.DialogPolicy{Accept: false}, want: nil},
		"заданная другой командой остаётся": {pending: &state.DialogPolicy{Accept: true, PromptText: "Иван"}, want: &state.DialogPolicy{Accept: true, PromptText: "Иван"}},
		"без политики ничего не происходит": {pending: nil, want: nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			st := state.State{PendingDialog: tc.pending}

			st.ClearPendingDialog(used)

			assert.Equal(t, tc.want, st.PendingDialog)
		})
	}
}

func TestRemove_WithoutStateIsNotAnError(t *testing.T) {
	t.Parallel()

	assert.NoError(t, state.NewStore(t.TempDir(), 9222).Remove())
}
