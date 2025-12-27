param(
  [string]$SwaggerPath = ""
)

$ErrorActionPreference = "Stop"

if ($SwaggerPath -eq "") {
  $SwaggerPath = (Resolve-Path (Join-Path $PSScriptRoot "..\\internal\\pb\\swagger\\trackings_api\\trackings.swagger.json")).Path
}

if (-not (Test-Path $SwaggerPath)) {
  throw "swagger file not found: $SwaggerPath"
}

$json = Get-Content -Raw -Path $SwaggerPath | ConvertFrom-Json

# 1) Swagger UI: POST /trackings/{trackingId}/refresh should NOT require a body.
$refreshPathKey = "/trackings/{trackingId}/refresh"
$refreshPathProp = $json.paths.PSObject.Properties | Where-Object { $_.Name -eq $refreshPathKey } | Select-Object -First 1
if ($null -ne $refreshPathProp) {
  $refreshPost = $refreshPathProp.Value.post
  if ($null -ne $refreshPost -and $null -ne $refreshPost.parameters) {
    $refreshPost.parameters = @($refreshPost.parameters | Where-Object { $_.'in' -ne "body" })
  }
}

# 2) Swagger UI: allow numeric IDs in request body for GetByIds (protobuf uint64 is shown as string by default).
$req = $json.definitions.v1GetTrackingsByIdsRequest
if ($null -ne $req -and $null -ne $req.properties -and $null -ne $req.properties.ids -and $null -ne $req.properties.ids.items) {
  $req.properties.ids.items.type = "integer"
  $req.properties.ids.items.format = "int64"
}

$json | ConvertTo-Json -Depth 100 | Set-Content -Path $SwaggerPath -Encoding UTF8

Write-Host "[patch-swagger] patched: $SwaggerPath"


