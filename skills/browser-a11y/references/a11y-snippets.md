# Сниппеты для проверки доступности

Передаются в `chromectl eval "<функция>"`; элемент из снапшота —
`--arg <uid>`. Результат печатается JSON.

## Поля без подписи

Поля ввода без `label[for]`, `aria-label`, `aria-labelledby` и без оборачивающего
`<label>`.

```js
() =>
  Array.from(document.querySelectorAll('input, select, textarea'))
    .filter(i => {
      const hasId = i.id && document.querySelector(`label[for="${i.id}"]`);
      const hasAria = i.getAttribute('aria-label') || i.getAttribute('aria-labelledby');
      return !hasId && !hasAria && !i.closest('label');
    })
    .map(i => ({tag: i.tagName, id: i.id, name: i.name, placeholder: i.placeholder}))
```

## Размер области нажатия

```js
(el) => {
  const rect = el.getBoundingClientRect();
  return {width: rect.width, height: rect.height};
}
```

## Контраст

Приближённое отношение контраста цвета текста и фона. Не учитывает прозрачность,
градиенты и фоновые картинки — для строгого аудита используй Lighthouse или axe-core.

```js
(el) => {
  const rgb = s => {
    const m = s.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/);
    return m ? [+m[1], +m[2], +m[3]] : [255, 255, 255];
  };
  const lum = ([r, g, b]) => [r, g, b]
    .map(v => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); })
    .reduce((acc, v, i) => acc + v * [0.2126, 0.7152, 0.0722][i], 0);
  const style = getComputedStyle(el);
  const l1 = lum(rgb(style.color));
  const l2 = lum(rgb(style.backgroundColor));
  return {
    color: style.color,
    background: style.backgroundColor,
    contrastRatio: ((Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05)).toFixed(2),
  };
}
```

## Настройки документа

```js
() => ({
  lang: document.documentElement.lang || 'НЕТ — скринридеру нужен язык для произношения',
  title: document.title || 'НЕТ — нужен для контекста',
  viewport: document.querySelector('meta[name="viewport"]')?.content || 'НЕТ — проверь и user-scalable=no',
  reducedMotion: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'включено' : 'выключено',
})
```
