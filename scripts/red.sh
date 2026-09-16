#!/usr/bin/env bash
# Шаг red цикла TDD: тест обязан упасть, и упасть от ассерта.
# ADR: docs/adr/0008-testy-kak-specifikaciya.md
#
# Usage: scripts/red.sh <regexp имени теста> [пакеты]
set -uo pipefail

run="${1:-}"
pkg="${2:-./...}"
[ -n "$pkg" ] || pkg="./..."

if [ -z "$run" ]; then
    echo "укажите имя теста: scripts/red.sh TestVersion_JSON [./cmd/...]" >&2
    exit 1
fi

out=$(go test "$pkg" -count=1 -run "$run" 2>&1)
status=$?

printf '%s\n' "$out"

if [ "$status" -eq 0 ]; then
    echo "✗ '$run' зелёный до реализации: тест не задаёт нового поведения (или не запустился — проверьте имя)"
    exit 1
fi

# Красный от компилятора ничего не проверяет: тест ещё не дошёл до ассерта.
if printf '%s' "$out" | grep -qE 'build failed|setup failed|undefined:|cannot find package|no test files|no tests to run'; then
    echo "✗ падение не от ассерта: сначала заглушка сигнатуры, чтобы код собирался, и только потом красный"
    exit 1
fi

echo "✓ красный получен — сверьте причину провала с предсказанной, затем пишите реализацию"
