#!/usr/bin/env bash
set -e

MSG_FILE="$1"
FIRST_LINE=$(head -n 1 "$MSG_FILE")
PATTERN="^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-zA-Z0-9_\-]+\))?: .+"

if [[ ! "$FIRST_LINE" =~ $PATTERN ]]; then
    echo "ERROR: Commit message does not follow Conventional Commits format!"
    echo "Your message: $FIRST_LINE"
    echo "Format: <type>(<scope>): <subject>"
    echo "Example: feat(task): add task creation endpoint"
    exit 1
fi
