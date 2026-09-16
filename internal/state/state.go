// Package state хранит между вызовами CLI то, что демон держал бы в памяти:
// процесс браузера, выбранную вкладку и настройки, которые нужно применить заново.
//
// ADR: docs/adr/0009-cli-bez-demona.md — состояние в файле, а не в процессе.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// CurrentVersion растёт только при несовместимом изменении формата. Новое поле
// добавляется опциональным и версию не меняет: старый state должен читаться.
const CurrentVersion = 1

const fileName = "state.json"

var ErrNotRunning = errors.New("браузер не запущен, выполните chromectl browser start")

type State struct {
	Version      int    `json:"version"`
	Port         int    `json:"port"`
	PID          int    `json:"pid"`
	UserDataDir  string `json:"userDataDir"`
	BrowserWSURL string `json:"browserWsUrl"`
	SelectedPage string `json:"selectedPage,omitempty"`

	SnapshotSeq int                `json:"snapshotSeq,omitempty"`
	UIDs        map[string]NodeRef `json:"uids,omitempty"`
	// PendingDialog — ответ на диалог для следующей команды без явного --dialog.
	PendingDialog *DialogPolicy `json:"pendingDialog,omitempty"`
	// Emulation — эмуляция по вкладкам. CDP снимает override при отключении
	// сессии, поэтому настройки применяются заново при каждом подключении.
	Emulation map[string]Emulation `json:"emulation,omitempty"`
}

type Emulation struct {
	Viewport    *Viewport         `json:"viewport,omitempty"`
	ColorScheme string            `json:"colorScheme,omitempty"`
	Network     string            `json:"network,omitempty"`
	CPURate     float64           `json:"cpuRate,omitempty"`
	UserAgent   string            `json:"userAgent,omitempty"`
	Geolocation *Geolocation      `json:"geolocation,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type Viewport struct {
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	DeviceScaleFactor float64 `json:"deviceScaleFactor"`
	Mobile            bool    `json:"mobile,omitempty"`
	Touch             bool    `json:"touch,omitempty"`
	Landscape         bool    `json:"landscape,omitempty"`
}

type Geolocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// NodeRef — маршрут от uid до DOM-узла. TargetID — target, в сессии которого
// узел разрешается: сама вкладка или cross-origin iframe.
type NodeRef struct {
	PageID        string `json:"pageId"`
	TargetID      string `json:"targetId"`
	FrameID       string `json:"frameId"`
	LoaderID      string `json:"loaderId"`
	BackendNodeID int64  `json:"backendNodeId"`
}

type DialogPolicy struct {
	Accept     bool   `json:"accept"`
	PromptText string `json:"promptText,omitempty"`
}

type Store struct {
	dir string
}

func NewStore(baseDir string, port int) *Store {
	return &Store{dir: filepath.Join(baseDir, strconv.Itoa(port))}
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) path() string {
	return filepath.Join(s.dir, fileName)
}

func (s *Store) Load() (State, error) {
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, ErrNotRunning
	}
	if err != nil {
		return State{}, fmt.Errorf("чтение state: %w", err)
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("разбор %s: %w", s.path(), err)
	}
	if st.Version > CurrentVersion {
		return State{}, fmt.Errorf("версия state %d новее поддерживаемой %d: обновите chromectl", st.Version, CurrentVersion)
	}
	return st, nil
}

// Save пишет state атомарно: команда, прерванная посреди записи, не должна
// оставить следующей команде обрезанный файл.
func (s *Store) Save(st State) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return s.save(st)
}

// Update перечитывает state под блокировкой, применяет fn и сохраняет. Команды на
// одном порту идут параллельными процессами: запись state, прочитанного в начале
// команды, затёрла бы изменения соседней.
func (s *Store) Update(fn func(*State)) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	st, err := s.Load()
	if err != nil {
		return err
	}
	fn(&st)
	return s.save(st)
}

// lock берёт исключительную блокировку каталога state; снимается закрытием дескриптора.
func (s *Store) lock() (unlock func(), err error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, fmt.Errorf("создание каталога state: %w", err)
	}
	// MkdirAll не трогает права уже существующего каталога, а в state лежат
	// захваченные заголовки и тела запросов.
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return nil, fmt.Errorf("права каталога state: %w", err)
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return nil, fmt.Errorf("блокировка state: %w", err)
	}
	if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_EX); err != nil {
		_ = dir.Close()
		return nil, fmt.Errorf("блокировка state: %w", err)
	}
	return func() { _ = dir.Close() }, nil
}

func (s *Store) save(st State) error {
	st.Version = CurrentVersion
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("сериализация state: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, fileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("временный файл state: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("запись state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("запись state: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path()); err != nil {
		return fmt.Errorf("замена state: %w", err)
	}
	return nil
}

// ClearPendingDialog снимает израсходованную политику диалога, если за время
// команды её не заменил новый `chromectl dialog`.
func (st *State) ClearPendingDialog(used DialogPolicy) {
	if st.PendingDialog != nil && *st.PendingDialog == used {
		st.PendingDialog = nil
	}
}

func (s *Store) Remove() error {
	err := os.Remove(s.path())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("удаление state: %w", err)
	}
	return nil
}
