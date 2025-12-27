# TrackBox: `track-api` + `track-worker` + Postgres + Kafka + Redis (+ 2 вспомогательных Python-приложения)

Этот репозиторий реализует проект **TrackBox** по ТЗ: хранение трек‑номеров, обновление статусов в фоне, история событий и демонстрация пайплайна через Kafka.

## Что внутри

- **`cmd/track-api`**: HTTP API (grpc-gateway) + Swagger, читает Kafka `tracking.updated`, пишет в Postgres и кэширует текущий статус в Redis.
- **`cmd/track-worker`**: фоновые проверки треков. Берёт из Postgres только те, у кого `next_check_at <= now()`, соблюдает rate-limit per carrier (Redis), ходит во внешний “carrier API”, публикует результат в Kafka `tracking.updated`.
- **`carrier-emulator/` (Python)**: фейковый “внешний сервис перевозчиков” для демо. Умеет прогрессировать статус со временем и отдавать `429` при превышении лимита.
- **`demo-generator/` (Python)**: генерация демо‑данных: создаёт трек‑номера, seed’ит эмулятор сценариями и массово добавляет треки в `track-api` пачками.

## Почему в проекте есть 2 Python-приложения

Они нужны именно для **демонстрации**, чтобы показать “как будто мы ходим в реальные CDEK/Почта РФ/агрегатор” и чтобы быстро наполнять систему большим числом треков.

- **`carrier-emulator`** — имитирует внешний API перевозчиков. Это позволяет демонстрировать воркер и ограничения (rate limits), не завися от реальных сервисов.
- **`demo-generator`** — имитирует клиента/интеграцию, которая массово добавляет треки и “подготавливает” эмулятор, чтобы статусы выглядели правдоподобно.

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

### Demo-fast режим (частые проверки воркером)

По умолчанию воркер ставит `next_check_at` **в минутах** (как ближе к прод‑логике, чтобы не “ддосить” перевозчиков).
Для демонстрации прогресса статуса “каждые несколько секунд” есть готовый конфиг:
- `config.trackbox.docker.demo.yaml` (для Docker)
- `config.trackbox.demo.yaml` (для локального запуска Go)

Чтобы включить в Docker, поменяй `configPath` на demo и пересоздай контейнеры:

```bash
docker compose down
docker compose up -d --build
```

И в `docker-compose.yaml` временно выставь:
`configPath=/app/config.demo.yaml` для `track-api` и `track-worker`.

Примеры запросов к эмулятору:

```bash
curl "http://localhost:9000/v1/tracking/CDEK/1234567890?apiKey=demo-key"
curl "http://localhost:9000/v1/tracking/POST_RU/RA123456789RU?apiKey=demo-key"
```

## Демо-сценарий (коротко)

1) Запусти всё через Docker: `docker compose up -d --build`
2) Нагенерь треки и добавь их в систему:

```bash
python -m pip install -r demo-generator/requirements.txt
python demo-generator/main.py --api-base http://localhost:8080 --emulator-base http://localhost:9000 --count 500 --batch 100 --rps 10 --carriers CDEK,POST_RU
```

3) Открой Swagger и посмотри:
- `POST /trackings/get-by-ids` — “текущее состояние” треков (попробуй ids `[1,2,3]`)
- `GET /trackings/{trackingId}/events` — история событий по треку
- `POST /trackings/{trackingId}/refresh` — “ускоритель”: делает трек срочным (ставит ближайший `next_check_at`)

4) Открой Kafka UI и посмотри топик `tracking.updated` — это сообщения, которые воркер публикует, а `track-api` читает и сохраняет.

## Быстрый старт (локально, без Docker для Go)

Инфраструктура (Postgres/Kafka/Redis + carrier-emulator) — через docker-compose:

```bash
docker compose up -d postgres redis kafka kafka-ui carrier-emulator
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

## Windows .bat (удобный запуск Python)

Есть готовые батники:
- `run-carrier-emulator.bat` — поднять `carrier-emulator` локально (создаёт `.venv`, ставит зависимости, запускает uvicorn на 9000)
- `run-demo-generator.bat` — запустить `demo-generator`
  - если запустить **без параметров** (двойной клик) — спросит `COUNT/BATCH/RPS/...`
  - если запустить **с параметрами** — просто прокинет их внутрь

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

Суммарное покрытие можно посмотреть так:

```bash
go test -coverprofile cover.out ./...
go tool cover -func cover.out
```


