from __future__ import annotations

import random
import time
from dataclasses import dataclass
from datetime import datetime, timezone, timedelta
from typing import Any

from fastapi import FastAPI, HTTPException, Query
from pydantic import BaseModel, Field


app = FastAPI(title="TrackBox Carrier Emulator", version="1.0.0")


def utc_now() -> datetime:
    return datetime.now(timezone.utc)


@dataclass
class Scenario:
    carrier: str
    code: str
    created_at: datetime
    steps: list[dict[str, Any]]
    step_seconds: int = 30

    def current_step_index(self, now: datetime) -> int:
        elapsed = max(0, int((now - self.created_at).total_seconds()))
        if self.step_seconds <= 0:
            return 0
        idx = elapsed // self.step_seconds
        return min(idx, max(0, len(self.steps) - 1))


# In-memory "DB" сценариев: ключ = f"{carrier}:{code}"
SCENARIOS: dict[str, Scenario] = {}


# Состояние треков для /v1/tracking/{carrier}/{track_number}
TRACK_STATE: dict[str, dict[str, Any]] = {}

# Простые лимиты "как будто у перевозчиков разные" (для демонстрации).
# В реальности мы лимитим и на Go (через Redis), но 429 от эмулятора делает демо нагляднее.
CARRIER_LIMITS_PER_MIN = {
    "CDEK": 60,
    "POST_RU": 20,
}

# Счётчики запросов по минуте: key = f"{carrier}:{YYYYMMDDHHMM}"
REQ_COUNTERS: dict[str, int] = {}


class SeedStep(BaseModel):
    operationAttribute: str = Field(..., description="Текст статуса как у Track24")
    operationType: str = Field(..., description="Тип операции как у Track24")
    operationPlacePostalCode: str | None = None
    operationPlaceName: str | None = None
    source: str | None = None


class SeedItem(BaseModel):
    carrier: str
    code: str
    stepSeconds: int = 30
    steps: list[SeedStep]


class SeedRequest(BaseModel):
    items: list[SeedItem]


@app.post("/v1/admin/seed")
def admin_seed(req: SeedRequest):
    now = utc_now()
    for it in req.items:
        key = f"{it.carrier}:{it.code}"
        SCENARIOS[key] = Scenario(
            carrier=it.carrier,
            code=it.code,
            created_at=now,
            steps=[s.model_dump() for s in it.steps],
            step_seconds=it.stepSeconds,
        )
    return {"status": "ok", "count": len(req.items)}


@app.post("/v1/admin/reset")
def admin_reset():
    SCENARIOS.clear()
    TRACK_STATE.clear()
    REQ_COUNTERS.clear()
    return {"status": "ok"}


class SeedV1Step(BaseModel):
    status: str
    statusRaw: str
    location: str | None = None
    message: str | None = None


class SeedV1Item(BaseModel):
    carrier: str
    trackNumber: str
    stepSeconds: int = 30
    progressProb: float = Field(0.25, ge=0.0, le=1.0)
    steps: list[SeedV1Step]


class SeedV1Request(BaseModel):
    items: list[SeedV1Item]


@app.post("/v1/admin/seed-v1")
def admin_seed_v1(req: SeedV1Request):
    now = utc_now()
    for it in req.items:
        key = f"{it.carrier}:{it.trackNumber}"
        TRACK_STATE[key] = {
            "carrier": it.carrier,
            "track_number": it.trackNumber,
            "created_at": now,
            "last_advance_at": now,
            "step_seconds": it.stepSeconds,
            "progress_prob": it.progressProb,
            "step_index": 0,
            "steps": [s.model_dump() for s in it.steps],
            "events": [],
        }
    return {"status": "ok", "count": len(req.items)}


def maybe_delay_and_fail(delay_ms: int, fail_rate: float):
    if delay_ms > 0:
        time.sleep(delay_ms / 1000.0)
    if fail_rate > 0 and random.random() < fail_rate:
        raise HTTPException(status_code=503, detail="emulator: random failure")


def format_track24_dt(dt: datetime) -> str:
    # Track24 пример: "02.07.2014 19:16:00"
    return dt.astimezone(timezone.utc).strftime("%d.%m.%Y %H:%M:%S")


def _minute_key(carrier: str, now: datetime) -> str:
    return f"{carrier}:{now.strftime('%Y%m%d%H%M')}"


def _enforce_carrier_limit(carrier: str, now: datetime):
    limit = CARRIER_LIMITS_PER_MIN.get(carrier)
    if limit is None:
        return
    k = _minute_key(carrier, now)
    REQ_COUNTERS[k] = REQ_COUNTERS.get(k, 0) + 1
    if REQ_COUNTERS[k] > limit:
        raise HTTPException(status_code=429, detail=f"rate limit exceeded for {carrier} ({limit}/min)")


def _default_steps_for(carrier: str) -> list[dict[str, Any]]:
    if carrier == "CDEK":
        return [
            {"status": "IN_TRANSIT", "statusRaw": "CDEK: accepted", "location": "Moscow", "message": "Accepted"},
            {"status": "IN_TRANSIT", "statusRaw": "CDEK: in transit", "location": "Sorting center", "message": "In transit"},
            {"status": "DELIVERED", "statusRaw": "CDEK: delivered", "location": "Destination", "message": "Delivered"},
        ]
    if carrier == "POST_RU":
        return [
            {"status": "IN_TRANSIT", "statusRaw": "POST_RU: accepted", "location": "Москва", "message": "Принято"},
            {"status": "IN_TRANSIT", "statusRaw": "POST_RU: processing", "location": "СЦ", "message": "Обработка"},
            {"status": "DELIVERED", "statusRaw": "POST_RU: delivered", "location": "Отделение", "message": "Вручено"},
        ]
    return [
        {"status": "IN_TRANSIT", "statusRaw": "Accepted", "location": "Emulator", "message": "Accepted"},
        {"status": "DELIVERED", "statusRaw": "Delivered", "location": "Emulator", "message": "Delivered"},
    ]


@app.get("/v1/tracking/{carrier}/{track_number}")
def v1_tracking(
    carrier: str,
    track_number: str,
    apiKey: str | None = Query(None),
    delayMs: int = Query(0, ge=0, le=10_000),
    failRate: float = Query(0.0, ge=0.0, le=1.0),
):
    """
    Основной "перевозчик/агрегатор" эндпоинт под TrackBox:
    - stateful прогрессия статусов
    - разные лимиты per carrier
    """
    _ = apiKey
    maybe_delay_and_fail(delayMs, failRate)

    now = utc_now()
    _enforce_carrier_limit(carrier, now)

    key = f"{carrier}:{track_number}"
    st = TRACK_STATE.get(key)
    if st is None:
        # auto-create default scenario
        st = {
            "carrier": carrier,
            "track_number": track_number,
            "created_at": now,
            "last_advance_at": now,
            "step_seconds": 20,
            "progress_prob": 0.25,
            "step_index": 0,
            "steps": _default_steps_for(carrier),
            "events": [],
        }
        TRACK_STATE[key] = st

    steps = st["steps"]
    idx = st["step_index"]

    # прогрессия: по таймеру или вероятности
    can_advance = idx < len(steps) - 1
    if can_advance:
        last_adv: datetime = st["last_advance_at"]
        step_seconds = int(st.get("step_seconds") or 0)
        prob = float(st.get("progress_prob") or 0.0)
        time_ready = step_seconds > 0 and (now - last_adv).total_seconds() >= step_seconds
        if time_ready or (prob > 0 and random.random() < prob):
            idx += 1
            st["step_index"] = idx
            st["last_advance_at"] = now

            step = steps[idx]
            st["events"].append(
                {
                    "status": step["status"],
                    "status_raw": step["statusRaw"],
                    "event_time": now.isoformat(),
                    "location": step.get("location"),
                    "message": step.get("message"),
                    "payload": {"carrier": carrier},
                }
            )

    cur = steps[idx]
    # Убедимся, что хотя бы одно событие есть (для истории)
    if not st["events"]:
        st["events"].append(
            {
                "status": cur["status"],
                "status_raw": cur["statusRaw"],
                "event_time": now.isoformat(),
                "location": cur.get("location"),
                "message": cur.get("message"),
                "payload": {"carrier": carrier},
            }
        )

    return {
        "carrier": carrier,
        "track_number": track_number,
        "status": cur["status"],
        "status_raw": cur["statusRaw"],
        "status_at": now.isoformat(),
        "events": st["events"],
    }


@app.get("/tracking.json.php")
def track24_tracking(
    apiKey: str = Query(...),
    domain: str = Query(...),
    code: str = Query(...),
    pretty: bool = Query(False),
    delayMs: int = Query(0, ge=0, le=10_000),
    failRate: float = Query(0.0, ge=0.0, le=1.0),
):
    """
    Эмуляция Track24 JSON endpoint.
    Документация Track24: https://track24.ru/?page=api#apiDocumentation
    """
    _ = apiKey
    _ = domain

    maybe_delay_and_fail(delayMs, failRate)

    # Для простоты используем carrier "AUTO" в ключе — можно заранее seed'ить AUTO:CODE
    key = f"AUTO:{code}"
    now = utc_now()
    sc = SCENARIOS.get(key)

    if sc is None:
        # Если сценарий не задан — сгенерим простой default
        sc = Scenario(
            carrier="AUTO",
            code=code,
            created_at=now - timedelta(minutes=10),
            steps=[
                {
                    "operationAttribute": "Принято в отделении связи",
                    "operationType": "Прием",
                    "operationPlacePostalCode": "000000",
                    "operationPlaceName": "Emulator City",
                    "source": "emulator",
                },
                {
                    "operationAttribute": "В пути",
                    "operationType": "Перевозка",
                    "operationPlacePostalCode": "000000",
                    "operationPlaceName": "Emulator City",
                    "source": "emulator",
                },
            ],
            step_seconds=60,
        )
        SCENARIOS[key] = sc

    idx = sc.current_step_index(now)

    events = []
    for i in range(idx + 1):
        step = sc.steps[i]
        ev_time = sc.created_at + timedelta(seconds=sc.step_seconds * i)
        events.append(
            {
                "id": str(1000 + i),
                "operationDateTime": format_track24_dt(ev_time),
                "operationAttribute": step.get("operationAttribute"),
                "operationPlacePostalCode": step.get("operationPlacePostalCode"),
                "operationPlaceName": step.get("operationPlaceName"),
                "operationType": step.get("operationType"),
                "itemWeight": "",
                "source": step.get("source") or "emulator",
            }
        )

    resp = {"status": "ok", "data": {"events": events}}
    return resp


@app.get("/gdeposylka/api/v4/track")
def gdeposylka_like(
    token: str = Query(...),
    track: str = Query(...),
    delayMs: int = Query(0, ge=0, le=10_000),
    failRate: float = Query(0.0, ge=0.0, le=1.0),
):
    """
    Упрощённая эмуляция "агрегатора" в стиле gdeposylka.
    Мы не завязываемся на реальный платный API, но сохраняем идею: token + track -> события.
    """
    _ = token
    maybe_delay_and_fail(delayMs, failRate)

    key = f"AUTO:{track}"
    now = utc_now()
    sc = SCENARIOS.get(key)
    if sc is None:
        raise HTTPException(status_code=404, detail="track not found (seed it via /v1/admin/seed)")

    idx = sc.current_step_index(now)
    checkpoints = []
    for i in range(idx + 1):
        step = sc.steps[i]
        ev_time = sc.created_at + timedelta(seconds=sc.step_seconds * i)
        checkpoints.append(
            {
                "time": ev_time.isoformat(),
                "status": step.get("operationType"),
                "message": step.get("operationAttribute"),
                "location": step.get("operationPlaceName"),
                "postal_code": step.get("operationPlacePostalCode"),
                "source": step.get("source") or "emulator",
            }
        )

    return {"status": "ok", "track": track, "checkpoints": checkpoints}


