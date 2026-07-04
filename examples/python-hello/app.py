from http.server import BaseHTTPRequestHandler, HTTPServer
import os

port = int(os.environ.get("PORT", "8000"))


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.end_headers()
        self.wfile.write(
            b"<!doctype html><title>skiff</title><h1>hello from skiff (python, no Dockerfile)</h1>"
        )


print(f"python-hello listening on {port}", flush=True)
HTTPServer(("0.0.0.0", port), Handler).serve_forever()
