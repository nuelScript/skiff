# The inbox loop every test agent shares: take an event, answer it with the framework, finish it.
import os, sys, time, requests

BASE = os.environ["SKIFF_AI_URL"]
AUTH = {"Authorization": "Bearer " + os.environ["SKIFF_AI_TOKEN"]}
MODEL_URL = BASE + "/fake/v1"
TOKEN = os.environ["SKIFF_AI_TOKEN"]


def serve(answer):
    print("agent up", flush=True)
    while True:
        try:
            r = requests.get(BASE + "/_skiff/events?wait=30", headers=AUTH, timeout=60)
        except Exception as e:
            print("inbox unreachable:", e, flush=True)
            time.sleep(2)
            continue
        if r.status_code != 200:
            continue
        event = r.json()
        try:
            out = answer(event["body"])
        except Exception as e:
            print("failed", event["id"], repr(e), flush=True)
            continue
        print("answered", event["id"], out, flush=True)
        requests.post(f"{BASE}/_skiff/events/{event['id']}/done", headers=AUTH, timeout=30)
