#!/usr/bin/env bash
# Правила, которые не выражаются через импорты и потому не ловятся линтером.
set -euo pipefail

fail=0

report() {
    echo "✗ $1"
    shift
    printf '%s\n' "$@" | sed 's/^/    /'
    fail=1
}

# context.TODO означает «здесь не подумали»: каждый CDP-вызов обязан получать
# настоящий контекст с таймаутом, иначе команда может повиснуть на мёртвом порту.
if found=$(grep -rn 'context.TODO()' --include='*.go' internal cmd 2>/dev/null | grep -v '_test.go'); then
    report 'context.TODO() — передайте настоящий контекст' "$found"
fi

# context.Background() в internal отрывает вызов от таймаута команды: он живёт,
# даже когда команда уже решила завершиться. Место ему — main и тесты.
if found=$(grep -rn 'context.Background()' --include='*.go' internal 2>/dev/null | grep -v '_test.go'); then
    report 'context.Background() в internal — передайте контекст команды' "$found"
fi

# Паника в команде даёт стектрейс вместо сообщения и оставляет state
# недописанным; ошибка возвращается наверх и превращается в код выхода.
if found=$(grep -rn 'panic(' --include='*.go' internal 2>/dev/null | grep -v '_test.go'); then
    report 'panic в internal — верните ошибку' "$found"
fi

if [ "$fail" -eq 0 ]; then
    echo "✓ гардрейлы пройдены"
fi
exit "$fail"
