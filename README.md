# TrackBox (Go часть): `track-api` + `track-worker` + Postgres + Kafka + Redis

Этот репозиторий реализует Go-часть проекта **TrackBox** по ТЗ:
- **`track-api`** — gRPC + HTTP (grpc-gateway) API, Swagger UI, consumer Kafka `tracking.updated`, запись в Postgres + Redis кэш текущего статуса.
- **`track-worker`** — воркер/поллер: выбирает due-треки из Postgres (`next_check_at <= now()`), соблюдает rate limit через Redis, публикует обновления в Kafka `tracking.updated`.

Для демонстрации используется `carrier-emulator` (Python), который эмулирует “перевозчиков” **CDEK** и **POST_RU**:
- эндпоинт: `GET /v1/tracking/{carrier}/{track_number}`
- внутри есть stateful‑прогрессия статуса (не каждый запрос меняет статус)
- лимиты per‑carrier (может отдавать `429`)

## Быстрый старт (Docker)

1) Поднять инфраструктуру + сервисы:

```bash
docker compose up -d --build
```

2) Проверить:
- HTTP gateway: `http://localhost:8080`
- Swagger UI: `http://localhost:8080/docs/`
- Swagger JSON: `http://localhost:8080/swagger.json`
- Kafka UI: `http://localhost:8081`
- Carrier emulator: `http://localhost:9000`

Примеры запросов к эмулятору:

```bash
curl "http://localhost:9000/v1/tracking/CDEK/1234567890?apiKey=demo-key"
curl "http://localhost:9000/v1/tracking/POST_RU/RA123456789RU?apiKey=demo-key"
```

## Быстрый старт (локально, без Docker для Go)

Инфраструктура (Postgres/Kafka/Redis) — через docker-compose:

```bash
docker compose up -d postgres redis kafka kafka-ui
```

Запуск `track-api`:

```powershell
$env:configPath="C:\Users\Mi\Desktop\4_course\TrackBox\config.trackbox.yaml"
$env:swaggerPath="C:\Users\Mi\Desktop\4_course\TrackBox\internal\pb\swagger\trackings_api\trackings.swagger.json"
go run .\cmd\track-api
```

Запуск `track-worker`:

```powershell
$env:configPath="C:\Users\Mi\Desktop\4_course\TrackBox\config.trackbox.yaml"
go run .\cmd\track-worker
```

## Demo-generator (Python)

Генерирует трек-номера (CDEK/POST_RU), seed'ит `carrier-emulator` сценариями (`/v1/admin/seed-v1`) и массово добавляет треки в `track-api`.

```bash
python -m pip install -r demo-generator/requirements.txt
python demo-generator/main.py --api-base http://localhost:8080 --emulator-base http://localhost:9000 --count 10000 --batch 200 --rps 20 --carriers CDEK,POST_RU
```

## API (минимум по ТЗ)

### Создать треки (массово)
`POST /trackings`

Пример:

```bash
curl -X POST http://localhost:8080/trackings \
  -H "Content-Type: application/json" \
  -d "{\"items\":[{\"carrierCode\":\"CDEK\",\"trackNumber\":\"A1\"},{\"carrierCode\":\"POST_RU\",\"trackNumber\":\"B2\"}]}"
```

### Получить по id
`POST /trackings/get-by-ids`

```bash
curl -X POST http://localhost:8080/trackings/get-by-ids \
  -H "Content-Type: application/json" \
  -d "{\"ids\":[1,2,3]}"
```

### История событий
`GET /trackings/{trackingId}/events?limit=&offset=`

```bash
curl "http://localhost:8080/trackings/1/events?limit=50&offset=0"
```

### Ускорить обновление
`POST /trackings/{trackingId}/refresh`

```bash
curl -X POST "http://localhost:8080/trackings/1/refresh"
```

## Kafka

### Топик `tracking.updated`
Producer: `track-worker`  
Consumer: `track-api`

Формат сообщения: JSON (`internal/broker/messages/TrackingUpdated`):
- `tracking_id`
- `checked_at`
- `status`, `status_raw`, `status_at`
- `next_check_at`
- `events[]` (опционально)
- `error` (опционально)

## Postgres

Таблицы создаются автоматически при старте (`internal/storage/pgtracking/schema.go`):
- `trackings`
- `tracking_events`

## Тесты и покрытие

```bash
go test -cover ./...
```

Текущее суммарное покрытие (по `go tool cover -func cover.out`) ≥ 50%.


