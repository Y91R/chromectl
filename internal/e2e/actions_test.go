//go:build e2e

// e2e Э2: снапшот с uid и действия на странице. Ожидания — из
// docs/plans/etap-2-snapshot-i-deystviya.md, раздел «Тесты».
package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const formPage = `<!doctype html><title>Форма</title>
<h1>Анкета</h1>
<input id="name" aria-label="Имя">
<input type="checkbox" id="agree" aria-label="Согласен">
<select id="city" aria-label="Город"><option>Москва</option><option>Казань</option></select>
<!-- Не name.value: внутри inline-обработчика name — свойство самой кнопки. -->
<button id="send" onclick="out.textContent = [document.getElementById('name').value, agree.checked, city.value].join('|')">Отправить</button>
<button id="del" onclick="out.textContent = String(confirm('Удалить?'))">Удалить</button>
<div id="hover" role="button" tabindex="0" onmouseenter="out.textContent = 'hovered'">Наведи</div>
<input id="keys" aria-label="Клавиши" onkeydown="if (!['Control', 'Shift', 'Alt', 'Meta'].includes(event.key)) log.textContent += (event.ctrlKey ? 'C+' : '') + (event.shiftKey ? 'S+' : '') + event.key + ';'">
<input type="file" id="file" aria-label="Файл" onchange="out.textContent = this.files[0].name">
<div draggable="true" id="drag" role="button" tabindex="0" aria-label="Перетащи"
     ondragstart="event.dataTransfer.setData('text/plain', 'груз')">Перетащи</div>
<div id="drop" role="button" tabindex="0" aria-label="Сюда"
     ondragover="event.preventDefault()" ondrop="event.preventDefault(); out.textContent = 'dropped:' + event.dataTransfer.getData('text/plain')">Сюда</div>
<div id="out"></div>
<div id="log"></div>
<iframe src="%s/frame" title="внешний" width="300" height="80"></iframe>`

func startFormFixture(t *testing.T) string {
	t.Helper()
	frame := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><input id="inner" aria-label="Внутри">`)
	}))
	t.Cleanup(frame.Close)
	// localhost и 127.0.0.1 — разные сайты: iframe уходит в отдельный процесс (OOPIF).
	frameURL := strings.Replace(frame.URL, "127.0.0.1", "localhost", 1)

	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, formPage, frameURL)
	}))
	t.Cleanup(page.Close)
	return page.URL
}

var uidLine = regexp.MustCompile(`uid=(\S+) (\S+)(?: "([^"]*)")?`)

// uidOf находит uid узла по роли и имени в тексте снапшота.
func uidOf(t *testing.T, snap, role, name string) string {
	t.Helper()
	for _, line := range strings.Split(snap, "\n") {
		m := uidLine.FindStringSubmatch(line)
		if m != nil && m[2] == role && m[3] == name {
			return m[1]
		}
	}
	t.Fatalf("в снапшоте нет %s %q:\n%s", role, name, snap)
	return ""
}

func (e *env) openForm(t *testing.T) string {
	t.Helper()
	e.startBrowser()
	e.mustRun("pages", "new", startFormFixture(t))
	return e.mustRun("snapshot").stdout
}

func (e *env) out() string {
	e.t.Helper()
	return strings.TrimSpace(e.mustRun("eval", "() => document.getElementById('out').textContent").stdout)
}

func TestSnapshot_ShowsFormAndCrossOriginIframe(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	snap := e.openForm(t)

	assert.Contains(t, snap, `RootWebArea "Форма"`)
	for _, node := range [][2]string{
		{"heading", "Анкета"},
		{"textbox", "Имя"},
		{"checkbox", "Согласен"},
		{"combobox", "Город"},
		{"button", "Отправить"},
		{"textbox", "Внутри"},
	} {
		assert.NotEmpty(t, uidOf(t, snap, node[0], node[1]))
	}

	verbose := e.mustRun("snapshot", "--verbose").stdout
	assert.Contains(t, verbose, `StaticText "Анкета"`)

	path := filepath.Join(t.TempDir(), "snap.txt")
	r := e.mustRun("snapshot", "-o", path)
	assert.Contains(t, r.stdout, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `button "Отправить"`)
}

func TestClick_TriggersHandlerAndPrintsSnapshot(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	snap := e.openForm(t)

	r := e.mustRun("click", uidOf(t, snap, "button", "Отправить"), "--snapshot")

	assert.Contains(t, r.stdout, `button "Отправить"`, "--snapshot печатает свежий снапшот")
	assert.Equal(t, `"|false|Москва"`, e.out())
}

func TestFillForm_SetsTextCheckboxAndSelect(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	snap := e.openForm(t)

	e.mustRun("fill-form",
		uidOf(t, snap, "textbox", "Имя")+"=Иван",
		uidOf(t, snap, "checkbox", "Согласен")+"=true",
		uidOf(t, snap, "combobox", "Город")+"=Казань",
	)
	e.mustRun("click", uidOf(t, snap, "button", "Отправить"))

	assert.Equal(t, `"Иван|true|Казань"`, e.out())

	r := e.run("fill", uidOf(t, snap, "checkbox", "Согласен"), "да")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, `для чекбокса и радиокнопки нужно значение "true" или "false"`)
}

func TestFill_InsideCrossOriginIframe(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	snap := e.openForm(t)

	e.mustRun("fill", uidOf(t, snap, "textbox", "Внутри"), "тест")

	assert.Contains(t, e.mustRun("snapshot").stdout, `textbox "Внутри"`)
	assert.Regexp(t, `textbox "Внутри".*value="тест"`, e.mustRun("snapshot").stdout)
}

func TestActions_StaleAndUnknownUID(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	snap := e.openForm(t)
	send := uidOf(t, snap, "button", "Отправить")

	r := e.run("click", "99_99")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "uid 99_99 не найден в последнем снапшоте")

	e.mustRun("navigate", "--reload")
	r = e.run("click", send)
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "снапшот устарел, выполните chromectl snapshot")
}

func TestClick_DialogClosedBeforeExit(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	snap := e.openForm(t)
	del := uidOf(t, snap, "button", "Удалить")

	r := e.mustRun("click", del)
	assert.Contains(t, r.stdout, "диалог confirm: Удалить? → accept")
	assert.Equal(t, `"true"`, e.out(), "страница получила accept и не заблокирована")

	e.mustRun("click", del, "--dialog", "dismiss")
	assert.Equal(t, `"false"`, e.out())

	e.mustRun("dialog", "dismiss")
	e.mustRun("click", del)
	assert.Equal(t, `"false"`, e.out(), "политика из dialog применилась к следующей команде")
	e.mustRun("click", del)
	assert.Equal(t, `"true"`, e.out(), "политика одноразовая")
}

func TestEval_ResultsArgsAndErrors(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	snap := e.openForm(t)

	assert.Equal(t, "2", strings.TrimSpace(e.mustRun("eval", "() => 1 + 1").stdout))
	assert.Equal(t, `"send"`, strings.TrimSpace(e.mustRun("eval", "(el) => el.id", "--arg", uidOf(t, snap, "button", "Отправить")).stdout))
	assert.Equal(t, `"Иван"`, strings.TrimSpace(e.mustRun("eval", "() => prompt('Имя?')", "--dialog", "Иван").stdout))

	path := filepath.Join(t.TempDir(), "eval.json")
	e.mustRun("eval", "async () => ({ok: true})", "-o", path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(data))

	r := e.run("eval", "() => { throw new Error('бум') }")
	assert.NotEqual(t, 0, r.code)
	assert.Empty(t, r.stdout)
	assert.Contains(t, r.stderr, "бум")
}

func TestWaitFor_TextAppearsOrTimesOut(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.openForm(t)

	e.mustRun("eval", "() => { setTimeout(() => { document.getElementById('out').textContent = 'Готово' }, 300) }")
	r := e.mustRun("wait-for", "Нет такого", "Готово")
	assert.Contains(t, r.stdout, "Готово")

	r = e.run("wait-for", "Никогда", "--timeout", "500ms")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, `текст "Никогда" не появился за 500ms`)
}

// Документ, из которого снят uid, сменился переходом на другую страницу: старый
// документ может жить в bfcache, и узел по backendNodeId ещё находится.
func TestActions_UIDFromPreviousDocumentIsStale(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	snap := e.openForm(t)
	send := uidOf(t, snap, "button", "Отправить")

	e.mustRun("navigate", fx.url+"/a")
	r := e.run("click", send)

	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "снапшот устарел, выполните chromectl snapshot")
}

func TestHoverTypePress(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	snap := e.openForm(t)

	e.mustRun("hover", uidOf(t, snap, "button", "Наведи"))
	assert.Equal(t, `"hovered"`, e.out())

	e.mustRun("click", uidOf(t, snap, "textbox", "Клавиши"))
	e.mustRun("type", "ab", "--submit", "Enter")
	e.mustRun("press", "Control+Shift+k")
	e.mustRun("press", "Shift+c")
	value := strings.TrimSpace(e.mustRun("eval", "() => document.getElementById('keys').value").stdout)
	log := strings.TrimSpace(e.mustRun("eval", "() => document.getElementById('log').textContent").stdout)
	assert.Equal(t, `"abC"`, value, "Shift с буквой вводит заглавную, Control — не вводит ничего")
	assert.Equal(t, `"a;b;Enter;C+S+K;S+C;"`, log)

	r := e.run("press", "Control+Нечто")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, `неизвестная клавиша "Нечто"`)
}

func TestUploadAndDrag(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	snap := e.openForm(t)

	file := filepath.Join(t.TempDir(), "отчёт.txt")
	require.NoError(t, os.WriteFile(file, []byte("данные"), 0o600))
	e.mustRun("upload", uidOf(t, snap, "button", "Файл"), file)
	assert.Equal(t, `"отчёт.txt"`, e.out())

	r := e.run("upload", uidOf(t, snap, "button", "Файл"), filepath.Join(t.TempDir(), "нет.txt"))
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "нет.txt")

	e.mustRun("drag", uidOf(t, snap, "button", "Перетащи"), uidOf(t, snap, "button", "Сюда"))
	assert.Equal(t, `"dropped:груз"`, e.out())
}

func TestScreenshot_Element(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	snap := e.openForm(t)

	path := filepath.Join(t.TempDir(), "button.png")
	e.mustRun("screenshot", "--uid", uidOf(t, snap, "button", "Отправить"), "-o", path)

	assert.Less(t, pngHeight(t, path), 100)
}
