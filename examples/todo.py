"""A small command-line todo list.

Tasks are stored in todo.json next to this file.

    python todo.py add "write the bottom of the stack first"
    python todo.py list
    python todo.py done 1
    python todo.py rm 1
"""

import json
import sys
from pathlib import Path

FILE = Path(__file__).with_name("todo.json")


def load():
    if not FILE.exists():
        return []
    return json.loads(FILE.read_text(encoding="utf-8"))


def save(items):
    FILE.write_text(json.dumps(items, indent=2) + "\n", encoding="utf-8")


def cmd_add(text):
    items = load()
    items.append({"text": text, "done": False})
    save(items)
    print(f"added {len(items)}. {text}")


def cmd_list():
    items = load()
    if not items:
        print("nothing to do")
        return
    for i, item in enumerate(items, start=1):
        mark = "x" if item["done"] else " "
        print(f"{i}. [{mark}] {item['text']}")


def cmd_done(number):
    items = load()
    item = items[number - 1]
    item["done"] = True
    save(items)
    print(f"done {number}. {item['text']}")


def cmd_rm(number):
    items = load()
    item = items.pop(number - 1)
    save(items)
    print(f"removed {number}. {item['text']}")


def main(argv):
    if len(argv) < 2:
        print(__doc__.strip())
        return 0
    command, *rest = argv[1:]
    try:
        if command == "add":
            cmd_add(" ".join(rest))
        elif command == "list":
            cmd_list()
        elif command == "done":
            cmd_done(int(rest[0]))
        elif command == "rm":
            cmd_rm(int(rest[0]))
        else:
            print(f"unknown command: {command}")
            return 1
    except (IndexError, ValueError):
        print("usage: todo.py add TEXT | list | done N | rm N")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
