# Build and push multi-arch Docker images
param(
    [string]$Registry = "meisterlala/compressor"
)

# Enable experimental features for buildx
docker buildx create --use --name multiarch 2>$null || docker buildx use multiarch

# Build and push latest tag for linux/amd64 and linux/arm64
$fullTag = "$Registry`:latest"
Write-Host "Building and pushing $fullTag for linux/amd64 and linux/arm64..."

docker buildx build --platform linux/amd64, linux/arm64 `
    --tag $fullTag `
    --push .

Write-Host "Build and push complete!"

Write-Host "Build and push complete!"