#!/bin/sh
# Setup git hooks for dgate development
# Run this once after cloning the repository

set -e

REPO_ROOT=$(git rev-parse --show-toplevel)
HOOKS_DIR="$REPO_ROOT/.githooks"

echo "🔧 Setting up git hooks for dgate..."

# Configure git to use our hooks directory
git config core.hooksPath "$HOOKS_DIR"

echo "✅ Git hooks configured!"
echo ""
echo "The following hooks are now active:"
echo "  - pre-commit: Runs rustfmt and clippy before each commit"
echo ""
echo "To disable hooks temporarily, use: git commit --no-verify"
echo "To remove hooks, run: git config --unset core.hooksPath"
