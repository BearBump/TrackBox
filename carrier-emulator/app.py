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
    return {"status": "ok"}


def maybe_delay_and_fail(delay_ms: int, fail_rate: float):
    if delay_ms > 0:
        time.sleep(delay_ms / 1000.0)
    if fail_rate > 0 and random.random() < fail_rate:
        raise HTTPException(status_code=503, detail="emulator: random failure")


def format_track24_dt(dt: datetime) -> str:
    # Track24 пример: "02.07.2014 19:16:00"
    return dt.astimezone(timezone.utc).strftime("%d.%m.%Y %H:%M:%S")


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


