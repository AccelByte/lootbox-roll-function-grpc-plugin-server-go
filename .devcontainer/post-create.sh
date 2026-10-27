#!/bin/bash
set -e

echo "🚀 Setting up development environment..."

# Install AGS CLI. CARGO_HOME keeps the cargo-dist installer from dropping
# the binary in ~/.cargo/bin. The devcontainer shell is bash, so the PATH
# line has to live in ~/.bashrc or later terminals cannot find ags.
echo "📥 Installing AGS CLI..."

export CARGO_HOME="$HOME/.ags-cli"
mkdir -p "$CARGO_HOME"
if ! command -v ags >/dev/null 2>&1 && [ ! -x "$CARGO_HOME/bin/ags" ]; then
    set -o pipefail
    curl --proto '=https' --tlsv1.2 -LsSf \
        https://github.com/AccelByte/accelbyte-ags-cli/releases/latest/download/accelbyte-ags-cli-installer.sh \
        | sh
    set +o pipefail
    echo "✅ AGS CLI installed"
else
    echo "✅ AGS CLI already installed"
fi

path_line='export PATH="$HOME/.ags-cli/bin:$PATH"'
touch "$HOME/.bashrc"
grep -qxF "$path_line" "$HOME/.bashrc" || printf '\n%s\n' "$path_line" >> "$HOME/.bashrc"

if [ -x "$CARGO_HOME/bin/ags" ]; then
    sudo ln -sfn "$CARGO_HOME/bin/ags" /usr/local/bin/ags
fi

# Install Go dependencies
echo "📦 Installing Go dependencies..."
go mod download
go mod tidy

# Make scripts executable
echo "🔧 Setting up scripts..."
chmod +x proto.sh

# Generate protobuf files
echo "✏️ Generating protocol buffer files..."
if command -v protoc &> /dev/null; then
    if [ -d "pkg/proto" ]; then
        ./proto.sh || echo "⚠️  Protocol buffer generation skipped"
    else
        echo "⚠️  Proto directory not found, skipping generation"
    fi
else
    echo "⚠️  protoc not found"
fi

# Configure git for safe directory
if [ -d ".git" ]; then
    echo "🔧 Setting up git..."
    git config --global --add safe.directory /workspace
fi

echo "✅ Development environment setup complete!"
echo ""
echo "🎯 Quick start commands:"
echo "  • Run Go service: set -a && source .env && set +a && go run main.go"
echo "  • Generate protobuf: ./proto.sh"
echo ""
echo "🛟 Ports:"
echo "  • gRPC Server: 6565"
echo "  • Prometheus Metrics: 8080"

