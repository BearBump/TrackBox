#!/bin/bash

cd "$(dirname "$0")/.." || exit

# Генерация gRPC кода
protoc -I ./api \
  -I ./api/google/api \
  --go_out=./internal/pb --go_opt=paths=source_relative \
  --go-grpc_out=./internal/pb --go-grpc_opt=paths=source_relative \
  ./api/trackings_api/trackings.proto ./api/models/tracking_model.proto

# Генерация gRPC-Gateway
protoc -I ./api \
  -I ./api/google/api \
  --grpc-gateway_out=./internal/pb \
  --grpc-gateway_opt paths=source_relative \
  --grpc-gateway_opt logtostderr=true \
  ./api/trackings_api/trackings.proto

# Генерация OpenAPI
protoc -I ./api \
  -I ./api/google/api \
  --openapiv2_out=./internal/pb/swagger \
  --openapiv2_opt logtostderr=true \
  ./api/trackings_api/trackings.proto

# Патчим swagger.json для удобства Swagger UI (без body для /refresh, numeric ids для get-by-ids)
if command -v pwsh >/dev/null 2>&1; then
  pwsh -NoProfile -ExecutionPolicy Bypass -File ./scripts/patch-swagger.ps1 >/dev/null
fi