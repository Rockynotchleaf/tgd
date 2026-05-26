#!/usr/bin/env bash
# install.sh — build tgd + tgd-hook and wire up the Claude Code hook
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_DIR="${HOME}/.local/bin"
SETTINGS="${HOME}/.claude/settings.json"

echo "→ Building tgd and tgd-hook..."
cd "$REPO_DIR"
go build -o "${INSTALL_DIR}/tgd"      ./cmd/tgd
go build -o "${INSTALL_DIR}/tgd-hook" ./cmd/tgd-hook

echo "→ Installed:"
echo "    ${INSTALL_DIR}/tgd"
echo "    ${INSTALL_DIR}/tgd-hook"

# ── Patch ~/.claude/settings.json ────────────────────────────────────────────
if [ ! -f "$SETTINGS" ]; then
    echo "→ Creating ${SETTINGS}"
    echo '{}' > "$SETTINGS"
fi

# Check if the hook is already configured
if grep -q "tgd-hook" "$SETTINGS" 2>/dev/null; then
    echo "→ Hook already present in ${SETTINGS} — skipping"
else
    echo "→ Patching ${SETTINGS} with PostToolUse hook..."

    # Use Python (available on most systems) to safely merge JSON
    python3 - "$SETTINGS" "${INSTALL_DIR}/tgd-hook" <<'PYEOF'
import json
import sys

settings_path = sys.argv[1]
hook_bin = sys.argv[2]

with open(settings_path) as f:
    settings = json.load(f)

new_hook = {
    "matcher": "Write|Edit|MultiEdit",
    "hooks": [
        {
            "type": "command",
            "command": hook_bin,
            "timeout": 5
        }
    ]
}

hooks = settings.setdefault("hooks", {})
post = hooks.setdefault("PostToolUse", [])

# Avoid duplicate
for existing in post:
    if existing.get("matcher") == new_hook["matcher"]:
        print("Hook already present (matcher match) — skipping")
        sys.exit(0)

post.append(new_hook)

with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)
    f.write("\n")

print(f"Hook added to {settings_path}")
PYEOF
fi

echo ""
echo "✓ Done. Start tgd manually with:  tgd"
echo "  Or it will auto-launch when Claude next edits a file."
echo ""
echo "  Make sure ${INSTALL_DIR} is in your PATH."
