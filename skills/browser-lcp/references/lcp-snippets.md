# Сниппеты для разбора LCP

Передаются в `chromectl eval "<функция>"`. Результат печатается JSON.

## LCP-элемент

```js
async () => new Promise(resolve => {
  new PerformanceObserver(list => {
    const last = list.getEntries().at(-1);
    resolve({
      element: last.element?.tagName,
      id: last.element?.id,
      className: last.element?.className,
      url: last.url,
      startTime: last.startTime,
      renderTime: last.renderTime,
      loadTime: last.loadTime,
      size: last.size,
    });
  }).observe({type: 'largest-contentful-paint', buffered: true});
})
```

## Тайминг LCP-ресурса

Задержка и длительность загрузки LCP-ресурса относительно TTFB.

```js
async () => {
  const lcp = await new Promise(resolve => new PerformanceObserver(l => resolve(l.getEntries().at(-1)))
    .observe({type: 'largest-contentful-paint', buffered: true}));
  const nav = performance.getEntriesByType('navigation')[0];
  const res = lcp.url ? performance.getEntriesByName(lcp.url)[0] : null;
  const ttfb = nav.responseStart;
  const loadStart = res ? res.startTime : ttfb;
  const loadEnd = res ? res.responseEnd : ttfb;
  return {
    lcp: lcp.startTime,
    ttfb,
    resourceLoadDelay: Math.max(0, loadStart - ttfb),
    resourceLoadDuration: Math.max(0, loadEnd - loadStart),
    elementRenderDelay: Math.max(0, lcp.startTime - loadEnd),
    url: lcp.url,
  };
}
```

## Типовые ошибки

```js
() => {
  const issues = [];
  document.querySelectorAll('img[loading="lazy"]').forEach(img => {
    if (img.getBoundingClientRect().top < innerHeight) {
      issues.push({issue: 'lazy-картинка в первом экране', element: img.outerHTML.slice(0, 200),
        fix: 'убрать loading="lazy": картинка видна сразу и может быть LCP'});
    }
  });
  document.querySelectorAll('img:not([fetchpriority])').forEach(img => {
    const r = img.getBoundingClientRect();
    if (r.top < innerHeight && r.width * r.height > 50000) {
      issues.push({issue: 'крупная картинка первого экрана без fetchpriority', element: img.outerHTML.slice(0, 200),
        fix: 'добавить fetchpriority="high"'});
    }
  });
  document.querySelectorAll('head script:not([async]):not([defer]):not([type="module"])').forEach(s => {
    if (s.src) {
      issues.push({issue: 'блокирующий скрипт в head', element: s.outerHTML.slice(0, 200),
        fix: 'добавить async или defer либо перенести в конец body'});
    }
  });
  return {issueCount: issues.length, issues};
}
```
