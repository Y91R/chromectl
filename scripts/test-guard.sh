#!/usr/bin/env bash
# Тест — точка спецификации: проверяем, что спецификация вообще написана.
# ADR: docs/adr/0008-testy-kak-specifikaciya.md
#
# Что именно проверяется, «зелёный или нет» проверяет `make test`:
#   1. каждый код ответа из docs/api/openapi.yaml имеет кейс в тестах транспорта;
#   2. каждая операция домена (internal/service/usecase/*) имеет тест.
set -euo pipefail

SPEC=docs/api/openapi.yaml
TRANSPORT_TESTS=internal/httptransport
USECASE_DIR=internal/service/usecase

fail=0

report() {
    echo "✗ $1"
    shift
    printf '%s\n' "$@" | sed 's/^/    /'
    fail=1
}

# Пары «операция — код ответа» из спеки. Отступы вычисляются от самой операции,
# а не зашиты числом: переформатированная спека (prettier, yq) не должна молча
# превращать гардрейл в no-op.
spec_pairs() {
    awk '
        function indent(line,   n) { n = match(line, /[^ ]/); return n == 0 ? -1 : n - 1 }
        /^[[:space:]]*operationId:[[:space:]]*[A-Za-z_]/ {
            op = $2; op_indent = indent($0); in_responses = 0; resp_indent = -1; next
        }
        op == "" { next }
        {
            cur = indent($0)
            if (cur <= op_indent && $0 ~ /[^[:space:]]/) {
                in_responses = ($1 == "responses:" && cur == op_indent)
                resp_indent = -1
                next
            }
            if (!in_responses) next
            if (resp_indent < 0) resp_indent = cur
            if (cur != resp_indent) next
            key = $1
            gsub(/[^0-9]/, "", key)
            if (length(key) == 3) print op, key
        }
    ' "$SPEC"
}

if [ -f "$SPEC" ]; then
    pairs=$(spec_pairs)

    # Гардрейл, который молча ничего не проверил, хуже отсутствующего: если из
    # спеки не разобралась ни одна операция или разобрались не все — это отказ,
    # а не «всё хорошо».
    declared=$(grep -oE 'operationId:[[:space:]]*[A-Za-z_][A-Za-z0-9_]*' "$SPEC" | wc -l | tr -d ' ')
    parsed=$(printf '%s\n' "$pairs" | awk 'NF {print $1}' | sort -u | wc -l | tr -d ' ')
    if [ "$declared" -gt 0 ] && [ "$parsed" -ne "$declared" ]; then
        echo "✗ гардрейл не разобрал $SPEC: операций в спеке $declared, распознано $parsed"
        echo "    проверьте формат файла — иначе проверка покрытия молча ничего не значит"
        fail=1
    fi

    missing=()
    while read -r op code; do
        [ -n "$op" ] || continue
        upper="$(printf '%s' "${op:0:1}" | tr '[:lower:]' '[:upper:]')${op:1}"
        if ! grep -rqE "[${op:0:1}${upper:0:1}]${op:1}_?${code}" --include='*_test.go' "$TRANSPORT_TESTS" 2>/dev/null; then
            missing+=("$op → $code")
        fi
    done < <(printf '%s\n' "$pairs")

    if [ "${#missing[@]}" -gt 0 ]; then
        report "код ответа из спеки не покрыт тестом ($TRANSPORT_TESTS): тест пишется до хендлера, а ожидание берётся из спеки" "${missing[@]}"
    fi
fi

if [ -d "$USECASE_DIR" ]; then
    untested=()
    for dir in "$USECASE_DIR"/*/; do
        [ -d "$dir" ] || continue
        compgen -G "$dir*_test.go" >/dev/null || untested+=("${dir%/}")
    done

    if [ "${#untested[@]}" -gt 0 ]; then
        report 'операция домена без теста — операция без спецификации' "${untested[@]}"
    fi
fi

if [ "$fail" -eq 0 ]; then
    echo "✓ спецификация тестами на месте"
fi
exit "$fail"
