from __future__ import annotations

import argparse
import random
import string
import time
from typing import Iterable

import requests


def gen_post_ru() -> str:
    # Упрощённый UPU S10: 2 буквы + 9 цифр + RU
    prefix = random.choice(string.ascii_uppercase) + random.choice(string.ascii_uppercase)
    digits = "".join(random.choice(string.digits) for _ in range(9))
    return f"{prefix}{digits}RU"


def gen_cdek() -> str:
    # Упрощённо: 10 цифр
    return "".join(random.choice(string.digits) for _ in range(10))


def batched(it: Iterable[dict], n: int):
    buf = []
    for x in it:
        buf.append(x)
        if len(buf) >= n:
            yield buf
            buf = []
    if buf:
        yield buf


def main():
    p = argparse.ArgumentParser("demo-generator")
    p.add_argument("--api-base", default="http://localhost:8080")
    p.add_argument("--emulator-base", default="http://localhost:9000")
    p.add_argument("--count", type=int, default=1000)
    p.add_argument("--batch", type=int, default=200)
    p.add_argument("--rps", type=float, default=20.0)
    p.add_argument("--carriers", default="CDEK,POST_RU")
    p.add_argument("--seed-emulator", action="store_true", default=True)
    args = p.parse_args()

    carriers = [c.strip() for c in args.carriers.split(",") if c.strip()]
    if not carriers:
        raise SystemExit("no carriers")

    sess = requests.Session()

    def gen_item():
        carrier = random.choice(carriers)
        if carrier == "CDEK":
            tn = gen_cdek()
        elif carrier == "POST_RU":
            tn = gen_post_ru()
        else:
            tn = "T" + "".join(random.choice(string.ascii_uppercase + string.digits) for _ in range(10))
        return {"carrierCode": carrier, "trackNumber": tn}

    items = [gen_item() for _ in range(args.count)]

    # Seed emulator scenarios for v1 endpoint (optional but полезно, чтобы прогрессия была предсказуемой)
    if args.seed_emulator:
        seed_items = []
        for it in items:
            carrier = it["carrierCode"]
            tn = it["trackNumber"]
            if carrier == "CDEK":
                steps = [
                    {"status": "IN_TRANSIT", "statusRaw": "CDEK accepted", "location": "Moscow", "message": "Accepted"},
                    {"status": "IN_TRANSIT", "statusRaw": "CDEK in transit", "location": "Sorting", "message": "In transit"},
                    {"status": "DELIVERED", "statusRaw": "CDEK delivered", "location": "Destination", "message": "Delivered"},
                ]
            else:
                steps = [
                    {"status": "IN_TRANSIT", "statusRaw": "POST_RU accepted", "location": "Москва", "message": "Принято"},
                    {"status": "IN_TRANSIT", "statusRaw": "POST_RU processing", "location": "СЦ", "message": "Обработка"},
                    {"status": "DELIVERED", "statusRaw": "POST_RU delivered", "location": "Отделение", "message": "Вручено"},
                ]

            seed_items.append(
                {
                    "carrier": carrier,
                    "trackNumber": tn,
                    "stepSeconds": 10,
                    "progressProb": 0.20,
                    "steps": steps,
                }
            )

        r = sess.post(f"{args.emulator_base}/v1/admin/seed-v1", json={"items": seed_items}, timeout=30)
        r.raise_for_status()

    # Create in track-api
    delay = 1.0 / max(1e-6, args.rps)
    created = 0
    for batch in batched(items, args.batch):
        t0 = time.time()
        r = sess.post(f"{args.api_base}/trackings", json={"items": batch}, timeout=30)
        r.raise_for_status()
        created += len(batch)

        took = time.time() - t0
        # простая пауза, чтобы держать rps примерно
        sleep_for = max(0.0, delay * len(batch) - took)
        if sleep_for > 0:
            time.sleep(sleep_for)

        print(f"created {created}/{args.count}")


if __name__ == "__main__":
    main()


