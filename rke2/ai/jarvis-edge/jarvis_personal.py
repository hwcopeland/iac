"""JARVIS personal-assistant data layer: macOS Calendar + Reminders +
overnight cluster alerts → a spoken-friendly daily briefing.

All sources are best-effort and degrade independently (Calendar needs a
one-time macOS TCC Automation grant; until then it's skipped, not fatal).
The briefing is cached so "what's my daily summary" / "Jarvis, wake up"
is an instant read, not a live recompute.

Local/uncommitted like the rest of JARVIS.
"""
from __future__ import annotations

import datetime as _dt
import email.utils as _eut
import json
import os
import subprocess
import xml.etree.ElementTree as _ET
from typing import List, Optional

_CACHE = os.path.expanduser("~/.openjarvis/briefing.json")
_CONFIG_PATH = os.path.expanduser("~/.openjarvis/config.toml")
_HTTP_TIMEOUT = 8.0
_UA = "JARVIS-personal/0.1 (+local)"


def _config() -> dict:
    """Read ~/.openjarvis/config.toml. Returns {} on any failure."""
    try:
        import tomllib
        with open(_CONFIG_PATH, "rb") as f:
            return tomllib.load(f)
    except (OSError, ValueError, ModuleNotFoundError):
        return {}


def _personal_cfg() -> dict:
    return _config().get("personal", {}) or {}

# Chronic RKE2 scrape-artifact alerts — never real failures (mirrors the
# daemon's _AlertWatcher mute set).
_MUTE = {
    "Watchdog", "KubeProxyDown", "InfoInhibitor",
    "etcdInsufficientMembers", "etcdMembersDown", "etcdNoLeader",
}


def _osa(script: str, timeout: float = 25.0) -> tuple[bool, str]:
    """AppleScript stub for the cluster pod (Linux — no macOS Calendar/
    Reminders access). Calendar / reminders functions all funnel through
    this, so returning failure makes them degrade gracefully into
    'unauthorized'. Future: replace with CalDAV bridge to iCloud."""
    return False, "AppleScript unavailable in cluster pod"


# ── iCloud CalDAV bridge (replaces macOS AppleScript path on Linux) ────────
# Connects to https://caldav.icloud.com using an Apple ID + app-specific
# password from env (sourced from a Bitwarden item via External Secret —
# see docs/spec/jarvis-icloud-caldav.md). When env vars are absent the
# functions return "unauthorized" so the briefing degrades cleanly.

_CALDAV_URL = os.environ.get("ICLOUD_CALDAV_URL", "https://caldav.icloud.com")
_caldav_client = None  # type: ignore[var-annotated]
_caldav_principal = None  # type: ignore[var-annotated]


def _caldav() -> tuple[object, object] | None:
    """Lazy-init the iCloud CalDAV client. Returns (client, principal) or
    None if creds aren't configured."""
    global _caldav_client, _caldav_principal
    if _caldav_client is not None:
        return _caldav_client, _caldav_principal
    user = os.environ.get("ICLOUD_APPLE_ID")
    pw = os.environ.get("ICLOUD_APP_PASSWORD")
    if not user or not pw:
        return None
    try:
        import caldav as _cd  # noqa: WPS433
        _caldav_client = _cd.DAVClient(url=_CALDAV_URL, username=user, password=pw)
        _caldav_principal = _caldav_client.principal()
    except Exception as exc:
        # Don't cache failure as success — leave client=None for retry
        _caldav_client = None
        _caldav_principal = None
        return None
    return _caldav_client, _caldav_principal


def _today_window() -> tuple[_dt.datetime, _dt.datetime]:
    now = _dt.datetime.now()
    d0 = now.replace(hour=0, minute=0, second=0, microsecond=0)
    d1 = d0 + _dt.timedelta(days=1)
    return d0, d1


def _vtodo_summary(comp) -> str:
    try:
        v = comp.icalendar_component if hasattr(comp, "icalendar_component") else comp
        s = v.get("SUMMARY") or v.get("summary")
        return str(s) if s else ""
    except Exception:
        return ""


def _vtodo_due_today(comp) -> bool:
    try:
        v = comp.icalendar_component if hasattr(comp, "icalendar_component") else comp
        due = v.get("DUE") or v.get("due")
        if due is None:
            return False
        d = due.dt if hasattr(due, "dt") else due
        if isinstance(d, _dt.datetime):
            d0, d1 = _today_window()
            return d0 <= d.replace(tzinfo=None) < d1
        # date-only
        return d == _dt.date.today()
    except Exception:
        return False


# ── Reminders (CalDAV VTODO collections on iCloud) ────────────────────────
def reminders_open(limit: int = 12) -> List[str]:
    res = _caldav()
    if res is None:
        return []
    _, principal = res
    out: List[str] = []
    try:
        for cal in principal.calendars():
            try:
                if "VTODO" not in (cal.get_supported_components() or []):
                    continue
            except Exception:
                continue
            try:
                todos = cal.todos(include_completed=False)
            except Exception:
                continue
            for t in todos:
                name = _vtodo_summary(t)
                if name:
                    out.append(name)
                if len(out) >= limit:
                    return out
    except Exception:
        return out
    return out[:limit]


def reminders_due_today() -> List[str]:
    res = _caldav()
    if res is None:
        return []
    _, principal = res
    out: List[str] = []
    try:
        for cal in principal.calendars():
            try:
                if "VTODO" not in (cal.get_supported_components() or []):
                    continue
            except Exception:
                continue
            try:
                todos = cal.todos(include_completed=False)
            except Exception:
                continue
            for t in todos:
                if _vtodo_due_today(t):
                    name = _vtodo_summary(t)
                    if name:
                        out.append(name)
    except Exception:
        return out
    return out


# ── Calendar (CalDAV VEVENT collections on iCloud) ────────────────────────
def calendar_today() -> dict:
    res = _caldav()
    if res is None:
        return {"status": "unauthorized"}
    _, principal = res
    d0, d1 = _today_window()
    events: list[dict] = []
    try:
        for cal in principal.calendars():
            try:
                comps = cal.get_supported_components() or []
                if "VEVENT" not in comps:
                    continue
            except Exception:
                continue
            try:
                results = cal.search(start=d0, end=d1, event=True, expand=True)
            except Exception:
                # older caldav lib API
                try:
                    results = cal.date_search(start=d0, end=d1)
                except Exception:
                    continue
            for ev in results:
                try:
                    v = ev.icalendar_component if hasattr(ev, "icalendar_component") else ev
                    title = str(v.get("SUMMARY") or v.get("summary") or "Untitled")
                    start = v.get("DTSTART") or v.get("dtstart")
                    when = ""
                    if start is not None:
                        sd = start.dt if hasattr(start, "dt") else start
                        if isinstance(sd, _dt.datetime):
                            when = sd.strftime("%-I:%M %p").lstrip("0")
                        else:
                            when = "all day"
                    events.append({"title": title.strip(), "when": when})
                except Exception:
                    continue
    except Exception as exc:
        return {"status": "error", "detail": str(exc)[:120]}
    events.sort(key=lambda e: e.get("when") or "")
    return {"status": "ok", "events": events}


# ── Overnight critical cluster alerts (read-only) ────────────────────────
def cluster_alerts() -> List[str]:
    try:
        pod = subprocess.run(
            ["kubectl", "-n", "monitor", "get", "pod", "-l",
             "app.kubernetes.io/name=alertmanager",
             "-o", "jsonpath={.items[0].metadata.name}"],
            capture_output=True, text=True, timeout=15).stdout.strip()
        if not pod:
            return []
        raw = subprocess.run(
            ["kubectl", "-n", "monitor", "exec", pod, "-c", "alertmanager",
             "--", "wget", "-qO-",
             "http://localhost:9093/api/v2/alerts?active=true&silenced=false"
             "&inhibited=false"],
            capture_output=True, text=True, timeout=30).stdout
        names = sorted({
            a.get("labels", {}).get("alertname", "?")
            for a in json.loads(raw or "[]")
            if a.get("labels", {}).get("severity") == "critical"
            and a.get("labels", {}).get("alertname") not in _MUTE
            and a.get("status", {}).get("state") == "active"
        })
        return names
    except Exception:  # noqa: BLE001
        return []


# ── Weather (wttr.in, IP-geolocated, no key) ─────────────────────────────
def _http_get(url: str, timeout: float = _HTTP_TIMEOUT) -> Optional[str]:
    """Fetch a URL via curl. Python.org's Python 3.12 doesn't trust the
    macOS keychain roots, so urllib SSL fails on wttr.in / news.google.com;
    curl uses the system trust store and just works.
    """
    try:
        r = subprocess.run(
            ["curl", "-sSL", "--max-time", str(int(timeout)),
             "-A", _UA, url],
            capture_output=True, text=True, timeout=timeout + 2,
        )
        if r.returncode != 0:
            return None
        return r.stdout
    except (subprocess.SubprocessError, OSError):
        return None


def weather(location: str = "") -> dict:
    """Today's weather summary. ``location`` defaults to the
    ``[personal] zip`` (or ``location``) value in
    ``~/.openjarvis/config.toml``; falls back to ZIP 37130 (Murfreesboro/
    Smyrna TN) if neither is set. Empty string → wttr.in IP-geolocates.

    Returns {status, location, summary, high_f, low_f, conditions}.
    """
    if not location:
        cfg = _personal_cfg()
        location = str(cfg.get("zip") or cfg.get("location") or "37130")
    # wttr.in matches bare digit strings against multiple country ZIP/postal
    # schemes (e.g. 37130 → Langeais, France). Pin digit-only inputs to US
    # unless they already carry a country suffix.
    if location.isdigit():
        location = f"{location},US"
    url = f"https://wttr.in/{location}?format=j1"
    raw = _http_get(url)
    if not raw:
        return {"status": "error", "detail": "fetch failed"}
    try:
        d = json.loads(raw)
        area = (d.get("nearest_area") or [{}])[0]
        place = ", ".join(
            x[0].get("value", "")
            for x in (area.get("areaName") or [],
                       area.get("region") or [])
            if x
        ).strip(", ")
        cur = (d.get("current_condition") or [{}])[0]
        today = (d.get("weather") or [{}])[0]
        hi = today.get("maxtempF")
        lo = today.get("mintempF")
        cond = (cur.get("weatherDesc") or [{}])[0].get("value", "").strip()
        temp = cur.get("temp_F")
        feels = cur.get("FeelsLikeF")
        humid = cur.get("humidity")
        summary = (
            f"{cond}, {temp}°F (feels {feels}°F), "
            f"high {hi}, low {lo}, {humid}% humidity"
        )
        return {"status": "ok", "location": place, "summary": summary,
                "high_f": hi, "low_f": lo, "conditions": cond,
                "temp_f": temp, "feels_f": feels, "humidity": humid}
    except (ValueError, KeyError, IndexError) as exc:
        return {"status": "error", "detail": str(exc)[:120]}


# ── News (wire-service / public-broadcaster RSS, stdlib only) ─────────────
# Quality over volume. These are non-aggregator feeds with editorial
# standards and minimal celebrity/sports/clickbait pollution:
#   • BBC World        — UK public broadcaster, broad international
#   • NPR World        — US public radio, hard news
#   • Guardian World   — UK quality daily, deep international coverage
# AP and Reuters discontinued public RSS, so they're not here.
_NEWS_WORLD_SOURCES = [
    ("BBC",      "http://feeds.bbci.co.uk/news/world/rss.xml"),
    ("NPR",      "https://feeds.npr.org/1004/rss.xml"),
    ("Guardian", "https://www.theguardian.com/world/rss"),
]


def _parse_rss(url: str, source: str = "") -> List[dict]:
    raw = _http_get(url)
    if not raw:
        return []
    try:
        root = _ET.fromstring(raw)
    except _ET.ParseError:
        return []
    items: List[dict] = []
    for it in root.iterfind(".//item"):
        title = (it.findtext("title") or "").strip()
        pub = (it.findtext("pubDate") or "").strip()
        if not title:
            continue
        ts: Optional[_dt.datetime] = None
        if pub:
            try:
                ts = _eut.parsedate_to_datetime(pub)
            except (TypeError, ValueError):
                ts = None
        items.append({"title": title, "ts": ts, "source": source})
    return items


def _dedupe_key(title: str) -> str:
    # Conservative same-story detector: lowercase, strip punctuation, take
    # the first ~40 chars. Different outlets phrase headlines differently
    # but usually share the leading subject (a name, place, or event).
    s = "".join(c for c in title.lower() if c.isalnum() or c.isspace())
    return " ".join(s.split())[:40]


def _aggregate_world(hours: Optional[int] = None) -> List[dict]:
    """Pull all configured world feeds, sort by recency, dedupe by title
    prefix. ``hours`` filters out anything older than that cutoff.
    """
    cutoff = None
    if hours is not None:
        cutoff = _dt.datetime.now(_dt.timezone.utc) - _dt.timedelta(hours=hours)
    merged: List[dict] = []
    for src, url in _NEWS_WORLD_SOURCES:
        merged.extend(_parse_rss(url, source=src))
    # Items with no timestamp sort last; valid ts → most recent first.
    merged.sort(key=lambda x: x["ts"] or _dt.datetime.min.replace(
        tzinfo=_dt.timezone.utc), reverse=True)
    seen, out = set(), []
    for it in merged:
        if cutoff is not None and (it["ts"] is None or it["ts"] < cutoff):
            continue
        k = _dedupe_key(it["title"])
        if k in seen:
            continue
        seen.add(k)
        out.append(it)
    return out


def news_top(limit: int = 5) -> List[str]:
    """Most-recent world headlines across BBC + NPR + Guardian."""
    return [it["title"] for it in _aggregate_world(hours=None)[:limit]]


def news_overnight(hours: int = 12, limit: int = 5) -> List[str]:
    """World headlines published in the last ``hours`` hours
    (default 12 = overnight). Same sources as :func:`news_top`.
    """
    return [it["title"] for it in _aggregate_world(hours=hours)[:limit]]


# ── Time-aware greeting + rain forecast narrative ────────────────────────
def _salutation(hour: int) -> str:
    if 4 <= hour < 12:
        return "Good morning"
    if 12 <= hour < 17:
        return "Good afternoon"
    if 17 <= hour < 22:
        return "Good evening"
    return "Hello"  # 22:00–03:59 — late night / early hours


_ONES_WORDS = (
    "zero", "one", "two", "three", "four", "five", "six", "seven",
    "eight", "nine", "ten", "eleven", "twelve", "thirteen", "fourteen",
    "fifteen", "sixteen", "seventeen", "eighteen", "nineteen",
)
_TENS_WORDS = ("", "", "twenty", "thirty", "forty", "fifty")


def _num_words(n: int) -> str:
    """Convert 0–59 to English words ('fifty-three')."""
    if n < 20:
        return _ONES_WORDS[n]
    t, o = divmod(n, 10)
    return _TENS_WORDS[t] + (f"-{_ONES_WORDS[o]}" if o else "")


def _speak_time(now: _dt.datetime) -> str:
    """TTS-friendly current time. Chatterbox reads '6:53' as the integer
    six-thousand-fifty-three, so we spell the components: 'It's six
    fifty-three PM.' Midnight/noon are special-cased."""
    h, m = now.hour, now.minute
    if h == 0 and m == 0:
        return "It's midnight."
    if h == 12 and m == 0:
        return "It's noon."
    h12 = h % 12 or 12
    suffix = "PM" if h >= 12 else "AM"
    if m == 0:
        return f"It's {_num_words(h12)} {suffix}."
    if m < 10:
        return f"It's {_num_words(h12)} oh {_num_words(m)} {suffix}."
    return f"It's {_num_words(h12)} {_num_words(m)} {suffix}."


_WET_TOKENS = ("rain", "shower", "storm", "thunder", "drizzle", "sleet")


def _is_wet(slot: dict) -> bool:
    desc = (slot.get("weatherDesc") or [{}])[0].get("value", "").lower()
    if any(t in desc for t in _WET_TOKENS):
        return True
    try:
        return int(slot.get("chanceofrain", "0")) >= 40
    except (TypeError, ValueError):
        return False


def _intensity(slot: dict) -> str:
    desc = (slot.get("weatherDesc") or [{}])[0].get("value", "").lower()
    if any(t in desc for t in ("heavy", "thunderstorm", "thundery")):
        return "heavy"
    if "light" in desc or "drizzle" in desc:
        return "light"
    return "moderate"


def _slot_clock(time_int: int) -> str:
    """Convert wttr.in's 'HH00' integer (0, 300, …, 2100) to spoken
    12-hour clock, e.g. 1500 → '3 PM', 900 → '9 AM'."""
    h24 = time_int // 100
    if h24 == 0:
        return "midnight"
    if h24 == 12:
        return "noon"
    suffix = "AM" if h24 < 12 else "PM"
    h12 = h24 % 12 or 12
    return f"{h12} {suffix}"


def _rain_narrative(j1: dict, now: _dt.datetime) -> str:
    """Scan today's hourly forecast and produce one short sentence about
    the next rain transition (start or stop). Empty string if no rain in
    today's remaining forecast and none currently."""
    try:
        hourly = j1["weather"][0]["hourly"]
    except (KeyError, IndexError, TypeError):
        return ""
    # Future-only slots: each slot covers a 3-hour window starting at its
    # time. Keep slots whose end (slot_time + 3h) is in the future.
    cur_h = now.hour
    future = [s for s in hourly if (int(s.get("time", 0)) // 100) + 3 > cur_h]
    if not future:
        return ""
    now_slot = future[0]
    currently_wet = _is_wet(now_slot)
    if currently_wet:
        # Find first dry slot after now → "should clear around HH".
        for s in future[1:]:
            if not _is_wet(s):
                return (f"Currently {_intensity(now_slot)} rain, expected "
                        f"to halt around {_slot_clock(int(s['time']))}.")
        return f"Expect {_intensity(now_slot)} rain through the rest of the day."
    # Dry now — find first wet slot.
    for s in future[1:]:
        if _is_wet(s):
            return (f"Expect {_intensity(s)} rain around "
                    f"{_slot_clock(int(s['time']))}.")
    return ""


def greeting(refresh: bool = False) -> str:
    """Time-aware spoken greeting: salutation + time + weather + rain
    narrative. Composed live every call (cheap — one wttr.in fetch). The
    weather portion is intentionally short so this works on demand without
    feeling like a full briefing."""
    now = _dt.datetime.now()
    parts = [f"{_salutation(now.hour)}, sir.", _speak_time(now)]
    wx_loc = _personal_cfg().get("zip") or _personal_cfg().get("location") or "37130"
    url = f"https://wttr.in/{wx_loc},US?format=j1" if str(wx_loc).isdigit() \
        else f"https://wttr.in/{wx_loc}?format=j1"
    raw = _http_get(url)
    if raw:
        try:
            j1 = json.loads(raw)
            cur = (j1.get("current_condition") or [{}])[0]
            today = (j1.get("weather") or [{}])[0]
            area = (j1.get("nearest_area") or [{}])[0]
            place = (area.get("areaName") or [{}])[0].get("value", "")
            cond = (cur.get("weatherDesc") or [{}])[0].get("value", "").strip()
            temp = cur.get("temp_F")
            hi = today.get("maxtempF")
            parts.append(
                f"It's currently {temp} degrees in {place}, "
                f"{cond.lower()}, with a high of {hi}."
            )
            rn = _rain_narrative(j1, now)
            if rn:
                parts.append(rn)
        except (ValueError, KeyError, IndexError):
            pass
    return " ".join(parts)


# ── Compose + cache ──────────────────────────────────────────────────────
def compose_briefing() -> str:
    now = _dt.datetime.now()
    parts: List[str] = [f"Good morning, sir. It's {now:%A, %B %-d}."]

    # Calendar + Reminders are AppleScript-only on this fork; the cluster
    # pod stub returns "unauthorized" for both. Stay quiet about them in
    # the briefing — adding them back is a follow-up via CalDAV/iCloud.
    cal = calendar_today()
    if cal.get("status") == "ok":
        ev = cal["events"]
        if ev:
            head = "; ".join(f"{e['title']}" for e in ev[:5])
            parts.append(f"You have {len(ev)} event"
                         f"{'s' if len(ev) != 1 else ''} today: {head}.")
        else:
            parts.append("Your calendar is clear today.")

    due = reminders_due_today()
    if due:
        parts.append(f"{len(due)} reminder"
                     f"{'s' if len(due) != 1 else ''} due today: "
                     f"{', '.join(due[:5])}.")

    alerts = cluster_alerts()
    if alerts:
        parts.append(f"Overnight, {len(alerts)} critical cluster alert"
                     f"{'s' if len(alerts) != 1 else ''}: "
                     f"{', '.join(alerts)}.")
    else:
        parts.append("The cluster was quiet overnight.")

    wx = weather()
    if wx.get("status") == "ok":
        loc = wx.get("location") or "your area"
        parts.append(
            f"In {loc} it's {wx['conditions'].lower()}, "
            f"{wx['temp_f']}°, high {wx['high_f']}, low {wx['low_f']}."
        )

    relevant = _interest_filtered_news(hours=12, max_items=2)
    if relevant:
        if len(relevant) == 1:
            parts.append(f"Worth your attention, sir: {relevant[0]}.")
        else:
            parts.append(
                f"Worth your attention, sir: {relevant[0]}. "
                f"Also: {'; '.join(relevant[1:])}."
            )
    # If zero matches → silently skip the news line (no generic headlines —
    # owner explicitly said dumping random world news kills the briefing).

    return " ".join(parts)


def _load_owner_interests() -> List[str]:
    """Read /state/users/<owner>/profile.md and pull keywords from a line
    starting with `Interests:` (case-insensitive). Returns lowercased,
    de-duplicated keyword list. Empty list = no filtering, news skipped."""
    import re
    # Find first owner-role profile under /state/users/
    base = "/state/users"
    if not os.path.isdir(base):
        return []
    keywords: List[str] = []
    for name in sorted(os.listdir(base)):
        p = os.path.join(base, name, "profile.md")
        if not os.path.isfile(p):
            continue
        try:
            txt = open(p).read()
        except OSError:
            continue
        m = re.search(r"(?im)^\s*(?:interests|topics|care_about|i_care_about)\s*[:=]\s*(.+)$",
                      txt)
        if m:
            for k in re.split(r"[,;]", m.group(1)):
                k = k.strip().lower()
                if k and k not in keywords:
                    keywords.append(k)
    return keywords


def _interest_filtered_news(hours: int = 12, max_items: int = 2) -> List[str]:
    """Pull overnight headlines, filter by keyword match against the
    owner's profile.md Interests line. Return up to `max_items`. If no
    interests configured → empty list (briefing skips news entirely)."""
    interests = _load_owner_interests()
    if not interests:
        return []
    candidates = news_overnight(hours=hours, limit=30)
    matched: List[str] = []
    for headline in candidates:
        low = headline.lower()
        if any(kw in low for kw in interests):
            matched.append(headline)
            if len(matched) >= max_items:
                break
    return matched


def write_cache(text: str) -> None:
    os.makedirs(os.path.dirname(_CACHE), exist_ok=True)
    with open(_CACHE, "w") as f:
        json.dump({"text": text,
                   "generated": _dt.datetime.now().isoformat(timespec="seconds")},
                  f)


def read_cache() -> dict:
    try:
        with open(_CACHE) as f:
            return json.load(f)
    except (OSError, ValueError):
        return {}


def seconds_until(hour: int = 5, minute: int = 0) -> float:
    now = _dt.datetime.now()
    nxt = now.replace(hour=hour, minute=minute, second=0, microsecond=0)
    if nxt <= now:
        nxt += _dt.timedelta(days=1)
    return (nxt - now).total_seconds()


def rebuild_and_cache() -> str:
    text = compose_briefing()
    write_cache(text)
    return text


if __name__ == "__main__":
    print(rebuild_and_cache())
