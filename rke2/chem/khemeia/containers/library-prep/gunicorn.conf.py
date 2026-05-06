import json
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer


class _HealthHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({"status": "ok"}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


def on_starting(server):
    """Start a lightweight health server in the arbiter process.

    Runs on port 8001 so liveness/readiness probes never compete with
    gunicorn workers that may be locked in long-running RDKit computations.
    """
    t = threading.Thread(
        target=lambda: HTTPServer(("0.0.0.0", 8001), _HealthHandler).serve_forever(),
        daemon=True,
    )
    t.start()
