#!/usr/bin/env bash

# Prerequisites: bash, curl, go, jq, ngrok

set -e
set -o pipefail
#set -x

function clean_up()
{
  kill -9 $GRPC_SERVER_PID $NGROK_PID
}

trap clean_up EXIT

echo '# Build and run Extend app locally'

go build -buildvcs=false -o grpc-server
./grpc-server & GRPC_SERVER_PID=$!

(for _ in {1..12}; do bash -c "timeout 1 echo > /dev/tcp/127.0.0.1/8080" 2>/dev/null && exit 0 || sleep 5s; done; exit 1)

if [ $? -ne 0 ]; then
  echo "Failed to run Extend app locally"
  exit 1
fi

echo '# Run ngrok'

( ngrok tcp 6565 > ngrok.log 2>&1 ) & NGROK_PID=$!

for _ in {1..12}; do
  sleep 5
  NGROK_RESPONSE=$(curl -s --location 'localhost:4040/api/tunnels')
  NGROK_URL=$(echo "$NGROK_RESPONSE" | jq -r '.tunnels[] | select(.config.addr = "localhost:6565") | .public_url')
  if [ -n "$NGROK_URL" ]; then
      break
  fi
done

if [ -z "$NGROK_URL" ]; then
  echo "Failed to run ngrok"
  exit 1
fi

echo '# Testing Extend app using demo CLI'

(cd demo/cli && GRPC_SERVER_URL="${NGROK_URL#*://}" go run main.go)
