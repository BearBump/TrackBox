param(
  [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
)

$ErrorActionPreference = "Stop"

Set-Location $RepoRoot

Write-Host "[generate] repo: $RepoRoot"

function Assert-Cmd($name) {
  $cmd = Get-Command $name -ErrorAction SilentlyContinue
  if (-not $cmd) {
    throw "command not found: $name. Install it and ensure it's in PATH."
  }
}

Assert-Cmd "protoc"

Write-Host "[generate] protoc: $(protoc --version)"

# protobuf/gRPC
Write-Host "[generate] go/grpc..."
protoc -I ./api `
  -I ./api/google/api `
  --go_out=./internal/pb --go_opt=paths=source_relative `
  --go-grpc_out=./internal/pb --go-grpc_opt=paths=source_relative `
  ./api/trackings_api/trackings.proto ./api/models/tracking_model.proto

# grpc-gateway
Write-Host "[generate] grpc-gateway..."
protoc -I ./api `
  -I ./api/google/api `
  --grpc-gateway_out=./internal/pb `
  --grpc-gateway_opt paths=source_relative `
  --grpc-gateway_opt logtostderr=true `
  ./api/trackings_api/trackings.proto

# openapi v2 (swagger)
Write-Host "[generate] openapi (swagger)..."
protoc -I ./api `
  -I ./api/google/api `
  --openapiv2_out=./internal/pb/swagger `
  --openapiv2_opt logtostderr=true `
  ./api/trackings_api/trackings.proto

# patch swagger for better Swagger UI UX (no body for /refresh, numeric ids for get-by-ids)
Write-Host "[generate] patch swagger..."
powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot "patch-swagger.ps1")

Write-Host "[generate] done"


