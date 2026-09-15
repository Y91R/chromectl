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

# Окружение читается ровно в одном месте: иначе конфигурация расползается
# по коду и её нельзя ни проверить, ни задокументировать.
if found=$(grep -rnE 'os\.(Getenv|LookupEnv)' --include='*.go' internal cmd 2>/dev/null \
    | grep -v '^internal/config/' | grep -v '_test.go'); then
    report 'чтение окружения вне internal/config — добавьте поле в config.Config' "$found"
fi

# context.TODO означает «здесь не подумали»: в рабочем коде такого быть не должно.
if found=$(grep -rn 'context.TODO()' --include='*.go' internal cmd 2>/dev/null | grep -v '_test.go'); then
    report 'context.TODO() — передайте настоящий контекст' "$found"
fi

# SQL живёт только в репозиториях: запрос из транспорта или домена ломает слои
# молча, компилятор его не заметит.
# Ищем и обычные строки, и raw-литералы в бэктиках, в том числе многострочные:
# запрос в бэктиках — самый частый способ протащить SQL мимо слоя.
sql_start='(select|insert[[:space:]]+into|update[[:space:]]+.*[[:space:]]set|delete[[:space:]]+from)[[:space:]]'
if found=$( { grep -rniE "[\"\`][[:space:]]*${sql_start}" --include='*.go' internal cmd 2>/dev/null; \
              grep -rnE "^[[:space:]]*(SELECT|INSERT INTO|UPDATE .* SET|DELETE FROM)[[:space:]]" --include='*.go' internal cmd 2>/dev/null; } \
    | grep -v '^internal/repository/' | grep -v '_test.go' | sort -u); then
    report 'SQL вне internal/repository — вынесите запрос в репозиторий или internal/db/queries' "$found"
fi

# panic допустим только при сборке приложения, где падать нормально.
if found=$(grep -rn 'panic(' --include='*.go' internal cmd 2>/dev/null \
    | grep -v '^internal/app/' | grep -v '_test.go'); then
    report 'panic вне internal/app — верните ошибку' "$found"
fi

if [ "$fail" -eq 0 ]; then
    echo "✓ гардрейлы пройдены"
fi
exit "$fail"
