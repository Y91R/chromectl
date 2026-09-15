#!/usr/bin/env bash
# Блокирует правку того, что должно меняться только через источник:
# генерат (gen/, front/src/gen/) и уже закоммиченные миграции (forward-only).
# Через Bash те же файлы правятся так же легко, как через Edit/Write, поэтому
# разбирается и команда — но именно её цели записи, а не любое упоминание пути.
set -uo pipefail

if ! command -v python3 >/dev/null 2>&1; then
    # Молча пропускать нельзя: гардрейл, который тихо отключился, хуже
    # отсутствующего — правка генерата прошла бы незамеченной.
    echo "protect-gen: нужен python3, чтобы разобрать вход хука; правка заблокирована" >&2
    exit 2
fi

# Вход хука читается в переменную: stdin занят самим скриптом.
payload=$(cat)

PROTECT_GEN_PAYLOAD="$payload" python3 - <<'PY'
import json
import os
import re
import shlex
import subprocess
import sys

GEN_MESSAGE = """Файлы в gen/ и front/src/gen/ генерируются, править их бессмысленно — изменения
потеряются при следующем `make generate`. Правьте источник:
  HTTP и TS-типы фронта → docs/api/openapi.yaml
  запросы               → internal/db/queries/*.sql и internal/db/migrations/
  моки                  → интерфейсы в internal/service/ports и usecase/*/contracts.go
  токены дизайна        → DESIGN.md"""

MIGRATION_MESSAGE = """Эта миграция уже под контролем версий и, скорее всего, применена.
Forward-only: создайте новую пару файлов через `make migrate-create name=...`."""


def deny(message):
    print(message, file=sys.stderr)
    raise SystemExit(2)


def is_generated(path):
    return re.search(r"(^|/)gen/", path) is not None


def is_migration(path):
    return "db/migrations/" in path


def is_committed(path):
    return subprocess.run(
        ["git", "ls-files", "--error-unmatch", path],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    ).returncode == 0


def write_targets(command):
    """Пути, в которые команда пишет. Упоминание пути в тексте (heredoc,
    сообщение коммита, аргумент grep) целью записи не является."""
    targets = []

    for segment in re.split(r"[\n;]|&&|\|\||\|", command):
        try:
            tokens = shlex.split(segment)
        except ValueError:
            tokens = segment.split()
        if not tokens:
            continue

        # Перенаправление: цель — ровно следующий токен.
        for i, token in enumerate(tokens[:-1]):
            if token in (">", ">>") or re.fullmatch(r"\d?>>?", token):
                targets.append(tokens[i + 1])
            elif token.startswith(">") and len(token) > 1 and not token.startswith(">>"):
                targets.append(token.lstrip(">"))
            elif token.startswith(">>") and len(token) > 2:
                targets.append(token[2:])

        name = os.path.basename(tokens[0])
        args = tokens[1:]
        paths = [a for a in args if not a.startswith("-") and "/" in a and not a.startswith("s/")]

        if name == "sed" and any(a.startswith("-i") or a == "--in-place" for a in args):
            targets.extend(paths)
        elif name in ("mv", "cp", "install") and len(args) >= 2:
            targets.append(args[-1])
        elif name in ("rm", "truncate", "shred", "unlink"):
            targets.extend(paths)
        elif name == "tee":
            targets.extend(paths)
        elif name == "dd":
            targets.extend(a[len("of="):] for a in args if a.startswith("of="))
        elif name in ("perl", "python", "python3") and any(a.startswith("-i") for a in args):
            targets.extend(paths)

    return [t for t in targets if t and " " not in t]


try:
    data = json.loads(os.environ.get("PROTECT_GEN_PAYLOAD") or "{}")
except json.JSONDecodeError as err:
    # Неразобранный вход — это не «всё хорошо»: пропустить его значит отключить
    # защиту молча, поэтому отказ громкий.
    deny(f"protect-gen: не удалось разобрать вход хука ({err}); правка заблокирована")
tool = data.get("tool_name", "")
tool_input = data.get("tool_input", {})

candidates = []
path = tool_input.get("file_path", "")
if path:
    candidates.append(path)

command = tool_input.get("command", "")
if tool == "Bash" and command:
    candidates.extend(write_targets(command))

for target in candidates:
    if is_generated(target):
        deny(GEN_MESSAGE)

for target in candidates:
    # Применённая миграция неизменна: правка уже накаченного файла разъезжается
    # с состоянием прода. Новая пара .up/.down — единственный корректный путь.
    if is_migration(target) and is_committed(target):
        deny(MIGRATION_MESSAGE)
PY
