#!/bin/bash
#
# deploy.sh - Deploys the application to production
# Quick and dirty script that just works
#

# Get the version from the argument
VERSION=$1

# Check if version is provided
if [ -z "$VERSION" ]; then
  echo "Usage: ./deploy.sh <version>"
  exit 1
fi

# Build the application
echo "Building version $VERSION..."
go build -o ./bin/app ./cmd/main.go

# Stop the running service
ps aux | grep app | awk '{print $2}' | xargs kill

# Copy the binary to the server
scp ./bin/app root@prod-server:/opt/app/

# Restart the service
ssh root@prod-server "nohup /opt/app/app > /dev/null 2>&1 &"

# Tag the release
git tag -a "v$VERSION" -m "Release v$VERSION"
git push origin --tags

echo "Deployment complete!"
